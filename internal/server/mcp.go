package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"native-db-bridge-mcp/internal/tools"
)

// JSON-RPC 2.0 constants.
const (
	jsonrpcVersion     = "2.0"
	mcpProtocolVersion = "2025-03-26"

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

// MCPTool describes one tool in the standard MCP tools/list response.
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// mcpCallToolParams is the standard params object for tools/call.
type mcpCallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// mcpTextContent is the standard text content item returned by tools/call.
type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
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

// buildMCPTools creates the standard MCP tool metadata for tools/list.
func buildMCPTools() []MCPTool {
	typesByName := map[string]reflect.Type{
		"datasource_list":           inputTypeOf[tools.DatasourceListInput](),
		"datasource_healthcheck":    inputTypeOf[tools.DatasourceHealthcheckInput](),
		"sql_schema_list":           inputTypeOf[tools.SQLSchemaListInput](),
		"sql_object_type_list":      inputTypeOf[tools.SQLObjectTypeListInput](),
		"sql_object_list":           inputTypeOf[tools.SQLObjectListInput](),
		"sql_object_describe":       inputTypeOf[tools.SQLObjectDescribeInput](),
		"sql_table_preview":         inputTypeOf[tools.SQLTablePreviewInput](),
		"redis_key_scan":            inputTypeOf[tools.RedisKeyScanInput](),
		"redis_key_describe":        inputTypeOf[tools.RedisKeyDescribeInput](),
		"mongo_database_list":       inputTypeOf[tools.MongoDatabaseListInput](),
		"mongo_collection_list":     inputTypeOf[tools.MongoCollectionListInput](),
		"mongo_collection_describe": inputTypeOf[tools.MongoCollectionDescribeInput](),
		"sql_query":                 inputTypeOf[tools.SQLQueryInput](),
		"sql_prepare_change":        inputTypeOf[tools.SQLPrepareChangeInput](),
		"redis_command":             inputTypeOf[tools.RedisCommandInput](),
		"redis_prepare_change":      inputTypeOf[tools.RedisPrepareChangeInput](),
		"mongo_find":                inputTypeOf[tools.MongoFindInput](),
		"mongo_prepare_change":      inputTypeOf[tools.MongoPrepareChangeInput](),
		"execute_confirmation":      inputTypeOf[tools.ExecuteConfirmationInput](),
		"operation_list":            inputTypeOf[tools.OperationListInput](),
		"cancel_operation":          inputTypeOf[tools.CancelOperationInput](),
		"audit_recent":              inputTypeOf[tools.AuditRecentInput](),
		"confirmation_get":          inputTypeOf[tools.ConfirmationGetInput](),
		"cancel_confirmation":       inputTypeOf[tools.CancelConfirmationInput](),
	}

	descriptions := map[string]string{
		"datasource_list":           "List configured SQL, Redis, and MongoDB datasources.",
		"datasource_healthcheck":    "Check a datasource configuration or live connectivity.",
		"sql_schema_list":           "List schemas or databases for a SQL datasource.",
		"sql_object_type_list":      "List SQL object types supported by the datasource.",
		"sql_object_list":           "List SQL tables, views, or other schema objects.",
		"sql_object_describe":       "Describe a SQL object including columns, indexes, and definition.",
		"sql_table_preview":         "Preview rows from a SQL table with a bounded limit.",
		"redis_key_scan":            "Scan Redis keys with cursor pagination.",
		"redis_key_describe":        "Describe one Redis key including type, TTL, length, and existence.",
		"mongo_database_list":       "List MongoDB databases for a datasource.",
		"mongo_collection_list":     "List MongoDB collections for a datasource.",
		"mongo_collection_describe": "Describe a MongoDB collection including indexes and sample schema.",
		"sql_query":                 "Run a read-only SQL query with limit and timeout controls.",
		"sql_prepare_change":        "Prepare a SQL write operation and create a confirmation record.",
		"redis_command":             "Run an allowed read-only Redis command.",
		"redis_prepare_change":      "Prepare a Redis write command and create a confirmation record.",
		"mongo_find":                "Run an allowed read-only MongoDB find, count, distinct, or aggregate operation.",
		"mongo_prepare_change":      "Prepare a MongoDB write operation and create a confirmation record.",
		"execute_confirmation":      "Execute a previously prepared write confirmation.",
		"operation_list":            "List recent or running operations.",
		"cancel_operation":          "Request cancellation of a running operation.",
		"audit_recent":              "List recent audit events.",
		"confirmation_get":          "Get the current state of a write confirmation.",
		"cancel_confirmation":       "Cancel a pending write confirmation.",
	}

	result := make([]MCPTool, 0, len(typesByName))
	for _, name := range tools.ToolNames() {
		result = append(result, MCPTool{
			Name:        name,
			Description: descriptions[name],
			InputSchema: inputSchemaFor(typesByName[name]),
		})
	}
	return result
}

func inputTypeOf[T any]() reflect.Type {
	var zero T
	return reflect.TypeOf(zero)
}

func inputSchemaFor(t reflect.Type) map[string]interface{} {
	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           map[string]interface{}{},
		"additionalProperties": false,
	}
	if t.Kind() != reflect.Struct {
		return schema
	}

	properties := schema["properties"].(map[string]interface{})
	required := make([]string, 0)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, optional := jsonFieldName(field)
		if name == "" || name == "-" {
			continue
		}
		properties[name] = jsonSchemaForType(field.Type)
		if !optional {
			required = append(required, name)
		}
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name, false
	}
	parts := strings.Split(tag, ",")
	optional := false
	for _, part := range parts[1:] {
		if part == "omitempty" {
			optional = true
			break
		}
	}
	return parts[0], optional
}

func jsonSchemaForType(t reflect.Type) map[string]interface{} {
	switch t.Kind() {
	case reflect.String:
		return map[string]interface{}{"type": "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]interface{}{"type": "integer"}
	case reflect.Bool:
		return map[string]interface{}{"type": "boolean"}
	case reflect.Slice, reflect.Array:
		return map[string]interface{}{
			"type":  "array",
			"items": jsonSchemaForType(t.Elem()),
		}
	case reflect.Map:
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		}
	case reflect.Interface:
		return map[string]interface{}{}
	default:
		return map[string]interface{}{"type": "object"}
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

	if len(req.ID) == 0 && strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	result, err := s.dispatchJSONRPC(r.Context(), req)
	if err != nil {
		code := errCodeInvalidParams
		if strings.HasPrefix(err.Error(), "unknown method") || strings.HasPrefix(err.Error(), "unknown tool") {
			code = errCodeMethodNotFound
		}
		writeJSONRPCError(w, req.ID, code, err.Error(), nil)
		return
	}

	writeJSONRPCResult(w, req.ID, result)
}

func (s *Server) dispatchJSONRPC(ctx context.Context, req JSONRPCRequest) (interface{}, error) {
	switch req.Method {
	case "initialize":
		return map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "native-db-bridge",
				"version": "0.1.0",
			},
		}, nil
	case "notifications/initialized":
		return nil, nil
	case "tools/list":
		return map[string]interface{}{
			"tools": buildMCPTools(),
		}, nil
	case "tools/call":
		return s.dispatchMCPToolCall(ctx, req.Params)
	default:
		handler, ok := s.dispatcher[req.Method]
		if !ok {
			return nil, fmt.Errorf("unknown method %q", req.Method)
		}
		return handler(ctx, req.Params)
	}
}

func (s *Server) dispatchMCPToolCall(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var call mcpCallToolParams
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	handler, ok := s.dispatcher[call.Name]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", call.Name)
	}
	args := call.Arguments
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	result, err := handler(ctx, args)
	if err != nil {
		return nil, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode tool result: %w", err)
	}
	return map[string]interface{}{
		"content": []mcpTextContent{
			{Type: "text", Text: string(resultJSON)},
		},
		"isError": false,
	}, nil
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
