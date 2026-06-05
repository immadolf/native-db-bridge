package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"native-db-bridge-mcp/internal/tools"
)

// JSON-RPC 2.0 constants.
const (
	jsonrpcVersion = "2.0"

	// Standard JSON-RPC error codes.
	errCodeParseError     = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternalError  = -32603
)

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the error object in a JSON-RPC 2.0 response.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// toolHandler is a function that dispatches raw JSON params to a typed handler.
type toolHandler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// buildDispatcher creates the method-name-to-handler mapping for all 24 MCP tools.
func buildDispatcher(h *tools.Handlers) map[string]toolHandler {
	return map[string]toolHandler{
		// Metadata tools
		"datasource_list": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.DatasourceListInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.DatasourceList(ctx, input), nil
		},
		"datasource_healthcheck": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.DatasourceHealthcheckInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.DatasourceHealthcheck(ctx, input), nil
		},
		"sql_schema_list": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.SQLSchemaListInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.SQLSchemaList(ctx, input), nil
		},
		"sql_object_type_list": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.SQLObjectTypeListInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.SQLObjectTypeList(ctx, input), nil
		},
		"sql_object_list": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.SQLObjectListInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.SQLObjectList(ctx, input), nil
		},
		"sql_object_describe": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.SQLObjectDescribeInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.SQLObjectDescribe(ctx, input), nil
		},
		"sql_table_preview": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.SQLTablePreviewInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.SQLTablePreview(ctx, input), nil
		},
		"redis_key_scan": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.RedisKeyScanInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.RedisKeyScan(ctx, input), nil
		},
		"redis_key_describe": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.RedisKeyDescribeInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.RedisKeyDescribe(ctx, input), nil
		},
		"mongo_database_list": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.MongoDatabaseListInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.MongoDatabaseList(ctx, input), nil
		},
		"mongo_collection_list": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.MongoCollectionListInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.MongoCollectionList(ctx, input), nil
		},
		"mongo_collection_describe": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.MongoCollectionDescribeInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.MongoCollectionDescribe(ctx, input), nil
		},

		// Execution tools
		"sql_query": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.SQLQueryInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.SQLQuery(ctx, input), nil
		},
		"sql_prepare_change": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.SQLPrepareChangeInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.SQLPrepareChange(ctx, input), nil
		},
		"redis_command": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.RedisCommandInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.RedisCommand(ctx, input), nil
		},
		"redis_prepare_change": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.RedisPrepareChangeInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.RedisPrepareChange(ctx, input), nil
		},
		"mongo_find": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.MongoFindInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.MongoFind(ctx, input), nil
		},
		"mongo_prepare_change": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.MongoPrepareChangeInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.MongoPrepareChange(ctx, input), nil
		},
		"execute_confirmation": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.ExecuteConfirmationInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.ExecuteConfirmation(ctx, input), nil
		},

		// Control / Audit tools
		"operation_list": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.OperationListInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.OperationList(ctx, input), nil
		},
		"cancel_operation": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.CancelOperationInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.CancelOperation(ctx, input), nil
		},
		"audit_recent": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.AuditRecentInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.AuditRecent(ctx, input), nil
		},
		"confirmation_get": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.ConfirmationGetInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.ConfirmationGet(ctx, input), nil
		},
		"cancel_confirmation": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var input tools.CancelConfirmationInput
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, fmt.Errorf("invalid params: %w", err)
			}
			return h.CancelConfirmation(ctx, input), nil
		},
	}
}

// handleMCP handles a single JSON-RPC 2.0 request against the dispatcher.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONRPCError(w, nil, errCodeInvalidRequest, "only POST is allowed", nil)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONRPCError(w, nil, errCodeParseError, "parse error", err.Error())
		return
	}

	if req.JSONRPC != jsonrpcVersion {
		writeJSONRPCError(w, req.ID, errCodeInvalidRequest, "jsonrpc must be \"2.0\"", nil)
		return
	}

	handler, ok := s.dispatcher[req.Method]
	if !ok {
		writeJSONRPCError(w, req.ID, errCodeMethodNotFound,
			fmt.Sprintf("unknown method %q", req.Method), nil)
		return
	}

	result, err := handler(r.Context(), req.Params)
	if err != nil {
		writeJSONRPCError(w, req.ID, errCodeInvalidParams, err.Error(), nil)
		return
	}

	writeJSONRPCResult(w, req.ID, result)
}

// writeJSONRPCResult writes a successful JSON-RPC 2.0 response.
func writeJSONRPCResult(w http.ResponseWriter, id json.RawMessage, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := JSONRPCResponse{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Result:  result,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// writeJSONRPCError writes an error JSON-RPC 2.0 response.
func writeJSONRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	// Use 200 OK for JSON-RPC protocol errors per the spec,
	// but use 400 for parse errors to help debugging.
	if code == errCodeParseError {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	resp := JSONRPCResponse{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}
