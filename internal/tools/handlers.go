// Package tools defines MCP tool input/output schemas and handler methods
// for the native-db-bridge MCP server.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"native-db-bridge-mcp/internal/audit"
	"native-db-bridge-mcp/internal/backend"
	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/nbderrors"
	"native-db-bridge-mcp/internal/ops"
)

// SQLMetaBackend extends backend.SQLBackend with schema introspection methods
// needed by the metadata tools.
type SQLMetaBackend interface {
	backend.SQLBackend
	SchemaList(ctx context.Context, datasource string) ([]string, error)
	ObjectTypeList(ctx context.Context, datasource string) ([]string, error)
	ObjectList(ctx context.Context, datasource, schema, objectType, namePattern string) ([]SQLObjectInfo, error)
	DescribeObject(ctx context.Context, datasource, schema, objectName, objectType string) (SQLDescribeResult, error)
}

// SQLDescribeResult holds the full description of a SQL object.
type SQLDescribeResult struct {
	Object     SQLObjectInfo
	Columns    []SQLColumnInfo
	Indexes    []SQLIndexInfo
	Definition string
}

// Deps holds all dependencies injected into Handlers.
type Deps struct {
	Config config.Config
	Audit  *audit.Store
	SQL    SQLMetaBackend
	Redis  backend.RedisBackend
	Mongo  backend.MongoBackend
	Ops    *ops.Tracker
}

// Handlers implements all MCP tool handler methods.
type Handlers struct {
	deps Deps
}

// NewHandlers returns a Handlers with the given dependencies.
func NewHandlers(d Deps) *Handlers {
	return &Handlers{deps: d}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// findDatasource returns the type and true if the datasource exists in config.
func (h *Handlers) findDatasource(name string) (string, interface{}, bool) {
	for _, ds := range h.deps.Config.Datasources.SQL {
		if ds.Name == name {
			return "sql", ds, true
		}
	}
	for _, ds := range h.deps.Config.Datasources.Redis {
		if ds.Name == name {
			return "redis", ds, true
		}
	}
	for _, ds := range h.deps.Config.Datasources.Mongo {
		if ds.Name == name {
			return "mongo", ds, true
		}
	}
	return "", nil, false
}

// withOperation creates a new context with timeout and registers the operation
// with the ops tracker. Returns the new context and operation ID.
func (h *Handlers) withOperation(ctx context.Context, kind, datasource, confirmationID string) (context.Context, string) {
	timeout := h.deps.Config.Server.QueryTimeout.Duration
	ctx, cancel := context.WithTimeout(ctx, timeout)
	opID := "op_" + uuid.New().String()
	h.deps.Ops.Register(opID, cancel)

	_ = h.deps.Audit.InsertOperation(audit.Operation{
		ID:             opID,
		Kind:           kind,
		Datasource:     datasource,
		Status:         "running",
		ConfirmationID: confirmationID,
		StartedAt:      time.Now(),
	})

	return ctx, opID
}

// recordAuditEvent inserts an audit event into the store.
func (h *Handlers) recordAuditEvent(eventType, datasource, opID, confID, summary, status string, elapsedMs int64, rowCount int, err error) {
	evt := audit.AuditEvent{
		ID:             "evt_" + uuid.New().String(),
		EventType:      eventType,
		Datasource:     datasource,
		OperationID:    opID,
		ConfirmationID: confID,
		Summary:        summary,
		Status:         status,
		ElapsedMs:      elapsedMs,
		RowCount:       rowCount,
		CreatedAt:      time.Now(),
	}
	if err != nil {
		evt.ErrorCode = "DRIVER_ERROR"
	}
	_ = h.deps.Audit.InsertAuditEvent(evt)
}

// executePayload dispatches the frozen confirmation payload to the appropriate backend.
func (h *Handlers) executePayload(ctx context.Context, conf *audit.Confirmation) (backend.ExecResult, error) {
	switch conf.Kind {
	case "sql_dml", "sql_ddl":
		var payload struct {
			SQL string `json:"sql"`
		}
		if err := json.Unmarshal([]byte(conf.PayloadJSON), &payload); err != nil {
			return backend.ExecResult{}, fmt.Errorf("failed to decode payload: %w", err)
		}
		return h.deps.SQL.Exec(ctx, conf.Datasource, payload.SQL)

	case "redis_write":
		var payload struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if err := json.Unmarshal([]byte(conf.PayloadJSON), &payload); err != nil {
			return backend.ExecResult{}, fmt.Errorf("failed to decode payload: %w", err)
		}
		result, err := h.deps.Redis.Command(ctx, conf.Datasource, payload.Command, payload.Args)
		if err != nil {
			return backend.ExecResult{}, err
		}
		return backend.ExecResult{AffectedCount: 1, Elapsed: result.Elapsed}, nil

	case "mongo_write":
		var payload struct {
			Operation  string                   `json:"operation"`
			Collection string                   `json:"collection"`
			Filter     map[string]interface{}   `json:"filter,omitempty"`
			Document   map[string]interface{}   `json:"document,omitempty"`
			Documents  []map[string]interface{} `json:"documents,omitempty"`
		}
		if err := json.Unmarshal([]byte(conf.PayloadJSON), &payload); err != nil {
			return backend.ExecResult{}, fmt.Errorf("failed to decode payload: %w", err)
		}
		return h.deps.Mongo.Write(ctx, backend.MongoWriteRequest{
			Datasource: conf.Datasource,
			Collection: payload.Collection,
			Operation:  payload.Operation,
			Filter:     payload.Filter,
			Document:   payload.Document,
			Documents:  payload.Documents,
		})

	default:
		return backend.ExecResult{}, fmt.Errorf("unknown confirmation kind: %s", conf.Kind)
	}
}

// parseTimeout parses a timeout string, defaulting to the configured query timeout.
func (h *Handlers) parseTimeout(s string) time.Duration {
	if s == "" {
		return h.deps.Config.Server.QueryTimeout.Duration
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return h.deps.Config.Server.QueryTimeout.Duration
	}
	if d > h.deps.Config.Server.QueryTimeout.Duration {
		return h.deps.Config.Server.QueryTimeout.Duration
	}
	return d
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// makeError creates a BaseOutput with a structured error.
func makeError(code nbderrors.Code, message string) BaseOutput {
	nbdErr := nbderrors.New(code, message)
	return BaseOutput{
		OK: false,
		Error: &ErrorOutput{
			Code:     string(nbdErr.Code),
			Category: string(nbdErr.Category),
			Message:  nbdErr.Message,
			Retryable: nbdErr.Retryable,
		},
	}
}

// withOpID sets the operation ID on a BaseOutput error.
func (b BaseOutput) withOpID(opID string) BaseOutput {
	b.OperationID = opID
	if b.Error != nil {
		b.Error.OperationID = opID
	}
	return b
}
