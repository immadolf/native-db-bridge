package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"native-db-bridge-mcp/internal/audit"
	"native-db-bridge-mcp/internal/backend"
	"native-db-bridge-mcp/internal/nbderrors"
	"native-db-bridge-mcp/internal/policy"
)

// ---------------------------------------------------------------------------
// Execution tools
// ---------------------------------------------------------------------------

// SQLQuery executes a read-only SQL query.
func (h *Handlers) SQLQuery(ctx context.Context, input SQLQueryInput) SQLQueryOutput {
	if !policy.IsSQLReadAllowed(input.SQL) {
		return SQLQueryOutput{
			BaseOutput: makeError(nbderrors.CodePolicyReadonlyToolRejectedWrite,
				"sql_query only accepts read-only statements (SELECT, SHOW, DESCRIBE, EXPLAIN)"),
		}
	}

	limit := input.Limit
	if limit <= 0 {
		limit = h.deps.Config.Server.MaxResultRows
	}
	if limit > h.deps.Config.Server.MaxResultRows {
		limit = h.deps.Config.Server.MaxResultRows
	}

	ctx, opID := h.withOperation(ctx, "sql_query", input.Datasource, "")

	result, err := h.deps.SQL.Query(ctx, input.Datasource, input.SQL, limit)
	if err != nil {
		h.recordAuditEvent("sql_query", input.Datasource, opID, "", input.SQL, "error", 0, 0, err)
		h.finishOperation(opID, err)
		return SQLQueryOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error()).withOpID(opID)}
	}

	cols := make([]ColumnInfo, len(result.Columns))
	for i, c := range result.Columns {
		cols[i] = ColumnInfo{Name: c.Name, Type: c.Type}
	}

	h.recordAuditEvent("sql_query", input.Datasource, opID, "", input.SQL, "success",
		result.Elapsed.Milliseconds(), result.RowCount, nil)
	h.finishOperation(opID, nil)

	return SQLQueryOutput{
		BaseOutput: BaseOutput{OK: true, OperationID: opID},
		Columns:    cols,
		Rows:       result.Rows,
		RowCount:   result.RowCount,
		Elapsed:    result.Elapsed.Milliseconds(),
	}
}

// SQLPrepareChange creates a confirmation for a SQL write operation.
func (h *Handlers) SQLPrepareChange(ctx context.Context, input SQLPrepareChangeInput) SQLPrepareChangeOutput {
	kind, allowed := policy.IsSQLWriteAllowed(input.SQL)
	if !allowed {
		return SQLPrepareChangeOutput{
			BaseOutput: makeError(nbderrors.CodePolicyReadonlyToolRejectedWrite,
				"sql_prepare_change only accepts write statements (INSERT, UPDATE, DELETE, DDL)"),
		}
	}

	ctx, opID := h.withOperation(ctx, "sql_prepare_change", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	confirmationKind := "sql_dml"
	if kind == "ddl" {
		confirmationKind = "sql_ddl"
	}

	riskLevel := "medium"
	if kind == "ddl" {
		riskLevel = "high"
	}

	confID := "conf_" + uuid.New().String()
	impact := map[string]interface{}{"mode": "estimated"}
	summary := truncate(input.SQL, 200)

	now := time.Now()
	expiresAt := now.Add(h.deps.Config.Policy.ConfirmationTTL.Duration)

	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"kind":       confirmationKind,
		"datasource": input.Datasource,
		"sql":        input.SQL,
	})

	conf := audit.Confirmation{
		ID:          confID,
		Kind:        confirmationKind,
		Datasource:  input.Datasource,
		PayloadJSON: string(payloadJSON),
		PayloadHash: fmt.Sprintf("%x", sha256.Sum256(payloadJSON)),
		Summary:     summary,
		RiskLevel:   riskLevel,
		ImpactJSON:  `{"mode":"estimated"}`,
		Status:      "pending",
		ExpiresAt:   expiresAt,
	}

	if err := h.deps.Audit.CreateConfirmation(conf); err != nil {
		opErr = err
		return SQLPrepareChangeOutput{
			BaseOutput: makeError(nbderrors.CodeInternalError, fmt.Sprintf("failed to create confirmation: %v", err)),
		}
	}

	h.recordAuditEvent("sql_prepare_change", input.Datasource, opID, confID, input.SQL, "pending", 0, 0, nil)

	return SQLPrepareChangeOutput{
		BaseOutput:     BaseOutput{OK: true},
		ConfirmationID: confID,
		Kind:           confirmationKind,
		Datasource:     input.Datasource,
		RiskLevel:      riskLevel,
		Impact:         impact,
		Summary:        summary,
		ExpiresAt:      expiresAt.Format(time.RFC3339),
	}
}

// RedisCommand executes a read-only Redis command.
func (h *Handlers) RedisCommand(ctx context.Context, input RedisCommandInput) RedisCommandOutput {
	if policy.IsRedisAlwaysRejected(input.Command) {
		return RedisCommandOutput{
			BaseOutput: makeError(nbderrors.CodePolicyRedisSelectRejected,
				fmt.Sprintf("command %q is always rejected", input.Command)),
		}
	}

	if !policy.IsRedisReadAllowed(input.Command) {
		return RedisCommandOutput{
			BaseOutput: makeError(nbderrors.CodePolicyReadonlyToolRejectedWrite,
				fmt.Sprintf("command %q is not a read command; use redis_prepare_change for writes", input.Command)),
		}
	}

	ctx, opID := h.withOperation(ctx, "redis_command", input.Datasource, "")

	result, err := h.deps.Redis.Command(ctx, input.Datasource, input.Command, input.Args)
	if err != nil {
		h.recordAuditEvent("redis_command", input.Datasource, opID, "",
			fmt.Sprintf("%s %v", input.Command, input.Args), "error", 0, 0, err)
		h.finishOperation(opID, err)
		return RedisCommandOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error()).withOpID(opID)}
	}

	h.recordAuditEvent("redis_command", input.Datasource, opID, "",
		fmt.Sprintf("%s %v", input.Command, input.Args), "success",
		result.Elapsed.Milliseconds(), 0, nil)
	h.finishOperation(opID, nil)

	return RedisCommandOutput{
		BaseOutput: BaseOutput{OK: true, OperationID: opID},
		Result:     result.Result,
		Elapsed:    result.Elapsed.Milliseconds(),
	}
}

// RedisPrepareChange creates a confirmation for a Redis write command.
func (h *Handlers) RedisPrepareChange(ctx context.Context, input RedisPrepareChangeInput) RedisPrepareChangeOutput {
	if policy.IsRedisAlwaysRejected(input.Command) {
		return RedisPrepareChangeOutput{
			BaseOutput: makeError(nbderrors.CodePolicyRedisSelectRejected,
				fmt.Sprintf("command %q is always rejected", input.Command)),
		}
	}

	if !policy.IsRedisWriteCommand(input.Command) {
		return RedisPrepareChangeOutput{
			BaseOutput: makeError(nbderrors.CodePolicyReadonlyToolRejectedWrite,
				fmt.Sprintf("command %q is not a recognized write command", input.Command)),
		}
	}

	ctx, opID := h.withOperation(ctx, "redis_prepare_change", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	confID := "conf_" + uuid.New().String()
	riskLevel := "medium"
	if input.Command == "FLUSHDB" || input.Command == "FLUSHALL" {
		riskLevel = "critical"
	} else if input.Command == "DEL" || input.Command == "HDEL" || input.Command == "SREM" || input.Command == "ZREM" {
		riskLevel = "high"
	}

	summary := fmt.Sprintf("%s %v", input.Command, input.Args)
	impact := map[string]interface{}{
		"mode": "best_effort",
		"keys": input.Args,
	}

	now := time.Now()
	expiresAt := now.Add(h.deps.Config.Policy.ConfirmationTTL.Duration)

	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"command": input.Command,
		"args":    input.Args,
	})

	conf := audit.Confirmation{
		ID:          confID,
		Kind:        "redis_write",
		Datasource:  input.Datasource,
		PayloadJSON: string(payloadJSON),
		PayloadHash: fmt.Sprintf("%x", sha256.Sum256(payloadJSON)),
		Summary:     summary,
		RiskLevel:   riskLevel,
		ImpactJSON:  `{"mode":"best_effort"}`,
		Status:      "pending",
		ExpiresAt:   expiresAt,
	}

	if err := h.deps.Audit.CreateConfirmation(conf); err != nil {
		opErr = err
		return RedisPrepareChangeOutput{
			BaseOutput: makeError(nbderrors.CodeInternalError, fmt.Sprintf("failed to create confirmation: %v", err)),
		}
	}

	h.recordAuditEvent("redis_prepare_change", input.Datasource, opID, confID, summary, "pending", 0, 0, nil)

	return RedisPrepareChangeOutput{
		BaseOutput:     BaseOutput{OK: true},
		ConfirmationID: confID,
		Kind:           "redis_write",
		Datasource:     input.Datasource,
		RiskLevel:      riskLevel,
		Impact:         impact,
		Summary:        summary,
		ExpiresAt:      expiresAt.Format(time.RFC3339),
	}
}

// MongoFind executes a read operation on MongoDB.
func (h *Handlers) MongoFind(ctx context.Context, input MongoFindInput) MongoFindOutput {
	// Validate aggregate stages if operation is aggregate
	if input.Operation == "aggregate" {
		for _, stage := range input.Pipeline {
			stageMap, ok := stage.(map[string]interface{})
			if !ok {
				return MongoFindOutput{
					BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, "invalid pipeline stage format"),
				}
			}
			for stageName := range stageMap {
				if !policy.IsMongoAggregateStageAllowed(stageName) {
					return MongoFindOutput{
						BaseOutput: makeError(nbderrors.CodePolicyReadonlyToolRejectedWrite,
							fmt.Sprintf("aggregate stage %q is not allowed", stageName)),
					}
				}
			}
		}
	}

	ctx, opID := h.withOperation(ctx, "mongo_find", input.Datasource, "")

	timeout := h.parseTimeout(input.Timeout)
	req := backend.MongoFindRequest{
		Datasource: input.Datasource,
		Collection: input.Collection,
		Operation:  input.Operation,
		Filter:     input.Filter,
		Pipeline:   input.Pipeline,
		Projection: input.Projection,
		Sort:       input.Sort,
		Limit:      input.Limit,
		Timeout:    timeout,
	}

	result, err := h.deps.Mongo.Find(ctx, req)
	if err != nil {
		h.recordAuditEvent("mongo_find", input.Datasource, opID, "",
			fmt.Sprintf("%s.%s", input.Datasource, input.Collection), "error", 0, 0, err)
		h.finishOperation(opID, err)
		return MongoFindOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error()).withOpID(opID)}
	}

	h.recordAuditEvent("mongo_find", input.Datasource, opID, "",
		fmt.Sprintf("%s.%s", input.Datasource, input.Collection), "success",
		result.Elapsed.Milliseconds(), result.Count, nil)
	h.finishOperation(opID, nil)

	return MongoFindOutput{
		BaseOutput: BaseOutput{OK: true, OperationID: opID},
		Documents:  result.Documents,
		Count:      result.Count,
		Elapsed:    result.Elapsed.Milliseconds(),
	}
}

// MongoPrepareChange creates a confirmation for a MongoDB write operation.
func (h *Handlers) MongoPrepareChange(ctx context.Context, input MongoPrepareChangeInput) MongoPrepareChangeOutput {
	hasFilter := len(input.Filter) > 0
	hasDocument := len(input.Document) > 0
	hasDocuments := len(input.Documents) > 0

	if !policy.ValidateMongoWrite(input.Operation, hasFilter, hasDocument, hasDocuments) {
		return MongoPrepareChangeOutput{
			BaseOutput: makeError(nbderrors.CodePolicyReadonlyToolRejectedWrite,
				fmt.Sprintf("invalid parameter combination for mongo operation %q", input.Operation)),
		}
	}

	ctx, opID := h.withOperation(ctx, "mongo_prepare_change", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	confID := "conf_" + uuid.New().String()
	riskLevel := "medium"
	switch input.Operation {
	case "deleteMany", "dropCollection":
		riskLevel = "high"
	case "insertOne", "insertMany", "updateOne", "updateMany", "deleteOne":
		riskLevel = "medium"
	}

	summary := fmt.Sprintf("%s on %s", input.Operation, input.Collection)

	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"operation":  input.Operation,
		"collection": input.Collection,
		"filter":     input.Filter,
		"document":   input.Document,
		"documents":  input.Documents,
	})

	now := time.Now()
	expiresAt := now.Add(h.deps.Config.Policy.ConfirmationTTL.Duration)

	conf := audit.Confirmation{
		ID:          confID,
		Kind:        "mongo_write",
		Datasource:  input.Datasource,
		PayloadJSON: string(payloadJSON),
		PayloadHash: fmt.Sprintf("%x", sha256.Sum256(payloadJSON)),
		Summary:     summary,
		RiskLevel:   riskLevel,
		ImpactJSON:  `{"mode":"best_effort"}`,
		Status:      "pending",
		ExpiresAt:   expiresAt,
	}

	if err := h.deps.Audit.CreateConfirmation(conf); err != nil {
		opErr = err
		return MongoPrepareChangeOutput{
			BaseOutput: makeError(nbderrors.CodeInternalError, fmt.Sprintf("failed to create confirmation: %v", err)),
		}
	}

	h.recordAuditEvent("mongo_prepare_change", input.Datasource, opID, confID, summary, "pending", 0, 0, nil)

	return MongoPrepareChangeOutput{
		BaseOutput:     BaseOutput{OK: true},
		ConfirmationID: confID,
		Kind:           "mongo_write",
		Datasource:     input.Datasource,
		RiskLevel:      riskLevel,
		Impact:         map[string]interface{}{"mode": "best_effort"},
		Summary:        summary,
		ExpiresAt:      expiresAt.Format(time.RFC3339),
	}
}

// ExecuteConfirmation executes a previously prepared write confirmation.
func (h *Handlers) ExecuteConfirmation(ctx context.Context, input ExecuteConfirmationInput) ExecuteConfirmationOutput {
	conf, err := h.deps.Audit.GetConfirmation(input.ConfirmationID)
	if err != nil {
		return ExecuteConfirmationOutput{
			BaseOutput: makeError(nbderrors.CodeConfirmationNotFound,
				fmt.Sprintf("confirmation %q not found", input.ConfirmationID)),
		}
	}

	ctx, opID := h.withOperation(ctx, "execute_"+conf.Kind, conf.Datasource, input.ConfirmationID)

	// Check expiry
	if time.Now().After(conf.ExpiresAt) {
		if err := h.deps.Audit.MarkConfirmationExpired(input.ConfirmationID); err != nil {
			h.finishOperation(opID, err)
			return ExecuteConfirmationOutput{
				BaseOutput: makeError(nbderrors.CodeConfirmationInvalidState,
					fmt.Sprintf("confirmation %q is not in pending state", input.ConfirmationID)).withOpID(opID),
				ConfirmationID: input.ConfirmationID,
				Status:         conf.Status,
			}
		}
		expiredErr := fmt.Errorf("confirmation %q has expired", input.ConfirmationID)
		h.recordAuditEvent("execute_confirmation", conf.Datasource, opID, input.ConfirmationID,
			conf.Summary, "expired", 0, 0, nil)
		h.finishOperation(opID, expiredErr)
		return ExecuteConfirmationOutput{
			BaseOutput: makeError(nbderrors.CodeConfirmationExpired,
				fmt.Sprintf("confirmation %q has expired", input.ConfirmationID)).withOpID(opID),
			ConfirmationID: input.ConfirmationID,
			Status:         "expired",
		}
	}

	// Mark executing (atomic CAS: pending -> executing)
	if err := h.deps.Audit.MarkConfirmationExecuting(input.ConfirmationID); err != nil {
		h.finishOperation(opID, err)
		return ExecuteConfirmationOutput{
			BaseOutput: makeError(nbderrors.CodeConfirmationInvalidState,
				fmt.Sprintf("confirmation %q is not in pending state", input.ConfirmationID)).withOpID(opID),
			ConfirmationID: input.ConfirmationID,
		}
	}

	// Execute the frozen payload
	execResult, execErr := h.executePayload(ctx, conf)

	if execErr != nil {
		_ = h.deps.Audit.MarkConfirmationFailed(input.ConfirmationID, execErr.Error())
		h.recordAuditEvent("execute_confirmation", conf.Datasource, opID, input.ConfirmationID,
			conf.Summary, "error", 0, 0, execErr)
		h.finishOperation(opID, execErr)
		return ExecuteConfirmationOutput{
			BaseOutput:     makeError(nbderrors.CodeDriverError, execErr.Error()).withOpID(opID),
			ConfirmationID: input.ConfirmationID,
			Status:         "failed",
		}
	}

	if err := h.deps.Audit.MarkConfirmationExecuted(input.ConfirmationID); err != nil {
		h.recordAuditEvent("execute_confirmation", conf.Datasource, opID, input.ConfirmationID,
			conf.Summary, "error", 0, 0, err)
		h.finishOperation(opID, err)
		return ExecuteConfirmationOutput{
			BaseOutput:     makeError(nbderrors.CodeConfirmationInvalidState, err.Error()).withOpID(opID),
			ConfirmationID: input.ConfirmationID,
			Status:         "failed",
		}
	}
	h.recordAuditEvent("execute_confirmation", conf.Datasource, opID, input.ConfirmationID,
		conf.Summary, "success", execResult.Elapsed.Milliseconds(), int(execResult.AffectedCount), nil)
	h.finishOperation(opID, nil)

	return ExecuteConfirmationOutput{
		BaseOutput:     BaseOutput{OK: true, OperationID: opID},
		ConfirmationID: input.ConfirmationID,
		Status:         "executed",
		AffectedCount:  execResult.AffectedCount,
		ResultSummary:  fmt.Sprintf("affected %d rows", execResult.AffectedCount),
		Elapsed:        execResult.Elapsed.Milliseconds(),
	}
}

// CancelConfirmation cancels a pending confirmation.
func (h *Handlers) CancelConfirmation(ctx context.Context, input CancelConfirmationInput) CancelConfirmationOutput {
	conf, err := h.deps.Audit.GetConfirmation(input.ConfirmationID)
	if err != nil {
		return CancelConfirmationOutput{
			BaseOutput: makeError(nbderrors.CodeConfirmationNotFound,
				fmt.Sprintf("confirmation %q not found", input.ConfirmationID)),
		}
	}

	ctx, opID := h.withOperation(ctx, "cancel_confirmation", conf.Datasource, input.ConfirmationID)
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	if conf.Status != "pending" {
		opErr = fmt.Errorf("confirmation %q is in %q state", input.ConfirmationID, conf.Status)
		return CancelConfirmationOutput{
			BaseOutput: makeError(nbderrors.CodeConfirmationInvalidState,
				fmt.Sprintf("confirmation %q is in %q state, only pending can be cancelled",
					input.ConfirmationID, conf.Status)),
			ConfirmationID: input.ConfirmationID,
			Status:         conf.Status,
		}
	}

	if err := h.deps.Audit.MarkConfirmationCancelled(input.ConfirmationID); err != nil {
		opErr = err
		return CancelConfirmationOutput{
			BaseOutput: makeError(nbderrors.CodeConfirmationInvalidState,
				fmt.Sprintf("failed to cancel confirmation: %v", err)),
			ConfirmationID: input.ConfirmationID,
		}
	}

	h.recordAuditEvent("cancel_confirmation", conf.Datasource, opID, input.ConfirmationID,
		conf.Summary, "cancelled", 0, 0, nil)

	return CancelConfirmationOutput{
		BaseOutput:     BaseOutput{OK: true},
		ConfirmationID: input.ConfirmationID,
		Status:         "cancelled",
	}
}

// ---------------------------------------------------------------------------
// Control / Audit tools
// ---------------------------------------------------------------------------

// OperationList returns recent operations, optionally filtered by status.
func (h *Handlers) OperationList(ctx context.Context, input OperationListInput) OperationListOutput {
	_, opID := h.withOperation(ctx, "operation_list", "", "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	operations, err := h.deps.Audit.ListOperations(input.Status, limit)
	if err != nil {
		opErr = err
		return OperationListOutput{BaseOutput: makeError(nbderrors.CodeInternalError, err.Error())}
	}

	results := make([]OperationInfo, len(operations))
	for i, op := range operations {
		results[i] = OperationInfo{
			OperationID: op.ID,
			Kind:        op.Kind,
			Datasource:  op.Datasource,
			Status:      op.Status,
			StartedAt:   op.StartedAt.Format(time.RFC3339),
		}
		if op.FinishedAt != nil {
			s := op.FinishedAt.Format(time.RFC3339)
			results[i].FinishedAt = &s
		}
		if op.CancelRequestedAt != nil {
			s := op.CancelRequestedAt.Format(time.RFC3339)
			results[i].CancelRequestedAt = &s
		}
		if op.ConfirmationID != "" {
			results[i].ConfirmationID = &op.ConfirmationID
		}
		if op.ErrorCode != "" {
			results[i].ErrorCode = &op.ErrorCode
		}
		if op.ErrorSummary != "" {
			results[i].ErrorSummary = &op.ErrorSummary
		}
	}

	return OperationListOutput{
		BaseOutput: BaseOutput{OK: true},
		Operations: results,
	}
}

// CancelOperation cancels an in-flight operation.
func (h *Handlers) CancelOperation(ctx context.Context, input CancelOperationInput) CancelOperationOutput {
	_, opID := h.withOperation(ctx, "cancel_operation", "", "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	found, cancelErr := h.deps.Ops.CancelAfter(input.OperationID, func() error {
		return h.deps.Audit.MarkOperationCancelRequested(input.OperationID)
	})
	if !found {
		opErr = fmt.Errorf("operation %q not found or already completed", input.OperationID)
		return CancelOperationOutput{
			BaseOutput: makeError(nbderrors.CodeOperationNotFound,
				fmt.Sprintf("operation %q not found or already completed", input.OperationID)),
		}
	}
	if cancelErr != nil {
		opErr = cancelErr
		return CancelOperationOutput{
			BaseOutput: makeError(nbderrors.CodeInternalError,
				fmt.Sprintf("failed to mark operation cancel requested: %v", cancelErr)),
			Status: "cancel_requested",
		}
	}
	return CancelOperationOutput{
		BaseOutput: BaseOutput{OK: true, OperationID: input.OperationID},
		Status:     "cancel_requested",
	}
}

// AuditRecent returns recent audit events.
func (h *Handlers) AuditRecent(ctx context.Context, input AuditRecentInput) AuditRecentOutput {
	_, opID := h.withOperation(ctx, "audit_recent", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}

	events, err := h.deps.Audit.ListAuditEvents(input.Datasource, input.EventType, input.Status, limit)
	if err != nil {
		opErr = err
		return AuditRecentOutput{BaseOutput: makeError(nbderrors.CodeInternalError, err.Error())}
	}

	results := make([]AuditEventInfo, len(events))
	for i, evt := range events {
		results[i] = AuditEventInfo{
			ID:         evt.ID,
			EventType:  evt.EventType,
			Datasource: evt.Datasource,
			Status:     evt.Status,
			Summary:    evt.Summary,
			CreatedAt:  evt.CreatedAt.Format(time.RFC3339),
			ElapsedMs:  evt.ElapsedMs,
			RowCount:   evt.RowCount,
		}
		if evt.OperationID != "" {
			results[i].OperationID = &evt.OperationID
		}
		if evt.ConfirmationID != "" {
			results[i].ConfirmationID = &evt.ConfirmationID
		}
		if evt.ErrorCode != "" {
			results[i].ErrorCode = &evt.ErrorCode
		}
	}

	return AuditRecentOutput{
		BaseOutput: BaseOutput{OK: true},
		Events:     results,
	}
}

// ConfirmationGet returns the current state of a confirmation.
func (h *Handlers) ConfirmationGet(ctx context.Context, input ConfirmationGetInput) ConfirmationGetOutput {
	conf, err := h.deps.Audit.GetConfirmation(input.ConfirmationID)
	if err != nil {
		return ConfirmationGetOutput{
			BaseOutput: makeError(nbderrors.CodeConfirmationNotFound,
				fmt.Sprintf("confirmation %q not found", input.ConfirmationID)),
		}
	}

	_, opID := h.withOperation(ctx, "confirmation_get", conf.Datasource, input.ConfirmationID)
	defer h.finishOperation(opID, nil)

	return ConfirmationGetOutput{
		BaseOutput: BaseOutput{OK: true},
		Confirmation: ConfirmationInfo{
			ConfirmationID: conf.ID,
			Kind:           conf.Kind,
			Datasource:     conf.Datasource,
			Status:         conf.Status,
			RiskLevel:      conf.RiskLevel,
			Summary:        conf.Summary,
			ExpiresAt:      conf.ExpiresAt.Format(time.RFC3339),
		},
	}
}
