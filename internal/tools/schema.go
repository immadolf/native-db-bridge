// Package tools defines MCP tool input/output schemas and handler methods
// for the native-db-bridge MCP server.
package tools

// ToolNames returns the complete list of all 24 MCP tool names.
func ToolNames() []string {
	return []string{
		// Metadata tools
		"datasource_list",
		"datasource_healthcheck",
		"sql_schema_list",
		"sql_object_type_list",
		"sql_object_list",
		"sql_object_describe",
		"sql_table_preview",
		"redis_key_scan",
		"redis_key_describe",
		"mongo_database_list",
		"mongo_collection_list",
		"mongo_collection_describe",
		// Execution tools
		"sql_query",
		"sql_prepare_change",
		"redis_command",
		"redis_prepare_change",
		"mongo_find",
		"mongo_prepare_change",
		"execute_confirmation",
		// Control / Audit tools
		"operation_list",
		"cancel_operation",
		"audit_recent",
		"confirmation_get",
		"cancel_confirmation",
	}
}

// ---------------------------------------------------------------------------
// Input schemas
// ---------------------------------------------------------------------------

// DatasourceListInput is the input for the datasource_list tool.
type DatasourceListInput struct {
	Type        string `json:"type,omitempty"`        // "sql", "redis", "mongo", or empty for all
	Environment string `json:"environment,omitempty"` // filter by environment, empty for all
}

// DatasourceHealthcheckInput is the input for the datasource_healthcheck tool.
type DatasourceHealthcheckInput struct {
	Datasource string `json:"datasource"`
	Mode       string `json:"mode"` // "config" or "connect"
}

// SQLSchemaListInput is the input for the sql_schema_list tool.
type SQLSchemaListInput struct {
	Datasource string `json:"datasource"`
}

// SQLObjectTypeListInput is the input for the sql_object_type_list tool.
type SQLObjectTypeListInput struct {
	Datasource string `json:"datasource"`
}

// SQLObjectListInput is the input for the sql_object_list tool.
type SQLObjectListInput struct {
	Datasource   string `json:"datasource"`
	Schema       string `json:"schema"`
	ObjectType   string `json:"object_type"`
	NamePattern  string `json:"name_pattern,omitempty"`
}

// SQLObjectDescribeInput is the input for the sql_object_describe tool.
type SQLObjectDescribeInput struct {
	Datasource string `json:"datasource"`
	Schema     string `json:"schema"`
	ObjectName string `json:"object_name"`
	ObjectType string `json:"object_type"`
}

// SQLTablePreviewInput is the input for the sql_table_preview tool.
type SQLTablePreviewInput struct {
	Datasource string `json:"datasource"`
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Limit      int    `json:"limit,omitempty"`
}

// RedisKeyScanInput is the input for the redis_key_scan tool.
type RedisKeyScanInput struct {
	Datasource string `json:"datasource"`
	Match      string `json:"match,omitempty"`
	Count      int    `json:"count,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
}

// RedisKeyDescribeInput is the input for the redis_key_describe tool.
type RedisKeyDescribeInput struct {
	Datasource string `json:"datasource"`
	Key        string `json:"key"`
}

// MongoDatabaseListInput is the input for the mongo_database_list tool.
type MongoDatabaseListInput struct {
	Datasource string `json:"datasource"`
}

// MongoCollectionListInput is the input for the mongo_collection_list tool.
type MongoCollectionListInput struct {
	Datasource  string `json:"datasource"`
	NamePattern string `json:"name_pattern,omitempty"`
}

// MongoCollectionDescribeInput is the input for the mongo_collection_describe tool.
type MongoCollectionDescribeInput struct {
	Datasource string `json:"datasource"`
	Collection string `json:"collection"`
}

// SQLQueryInput is the input for the sql_query tool.
type SQLQueryInput struct {
	Datasource string `json:"datasource"`
	SQL        string `json:"sql"`
	Limit      int    `json:"limit,omitempty"`
	Timeout    string `json:"timeout,omitempty"`
}

// SQLPrepareChangeInput is the input for the sql_prepare_change tool.
type SQLPrepareChangeInput struct {
	Datasource string `json:"datasource"`
	SQL        string `json:"sql"`
	Timeout    string `json:"timeout,omitempty"`
}

// RedisCommandInput is the input for the redis_command tool.
type RedisCommandInput struct {
	Datasource string   `json:"datasource"`
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	Timeout    string   `json:"timeout,omitempty"`
}

// RedisPrepareChangeInput is the input for the redis_prepare_change tool.
type RedisPrepareChangeInput struct {
	Datasource string   `json:"datasource"`
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	Timeout    string   `json:"timeout,omitempty"`
}

// MongoFindInput is the input for the mongo_find tool.
type MongoFindInput struct {
	Datasource string                 `json:"datasource"`
	Collection string                 `json:"collection"`
	Operation  string                 `json:"operation"` // "find", "findOne", "countDocuments", "distinct", "aggregate"
	Filter     map[string]interface{} `json:"filter,omitempty"`
	Pipeline   []interface{}          `json:"pipeline,omitempty"`
	Projection map[string]interface{} `json:"projection,omitempty"`
	Sort       map[string]interface{} `json:"sort,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
	Timeout    string                 `json:"timeout,omitempty"`
}

// MongoPrepareChangeInput is the input for the mongo_prepare_change tool.
type MongoPrepareChangeInput struct {
	Datasource string                   `json:"datasource"`
	Collection string                   `json:"collection"`
	Operation  string                   `json:"operation"` // "insertOne", "insertMany", "updateOne", "updateMany", "deleteOne", "deleteMany", "dropCollection"
	Filter     map[string]interface{}   `json:"filter,omitempty"`
	Document   map[string]interface{}   `json:"document,omitempty"`
	Documents  []map[string]interface{} `json:"documents,omitempty"`
	Timeout    string                   `json:"timeout,omitempty"`
}

// ExecuteConfirmationInput is the input for the execute_confirmation tool.
type ExecuteConfirmationInput struct {
	ConfirmationID string `json:"confirmation_id"`
}

// OperationListInput is the input for the operation_list tool.
type OperationListInput struct {
	Status string `json:"status,omitempty"` // "running", "cancel_requested", "finished", or empty
	Limit  int    `json:"limit,omitempty"`
}

// CancelOperationInput is the input for the cancel_operation tool.
type CancelOperationInput struct {
	OperationID string `json:"operation_id"`
}

// AuditRecentInput is the input for the audit_recent tool.
type AuditRecentInput struct {
	Datasource string `json:"datasource,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	Status     string `json:"status,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// ConfirmationGetInput is the input for the confirmation_get tool.
type ConfirmationGetInput struct {
	ConfirmationID string `json:"confirmation_id"`
}

// CancelConfirmationInput is the input for the cancel_confirmation tool.
type CancelConfirmationInput struct {
	ConfirmationID string `json:"confirmation_id"`
}

// ---------------------------------------------------------------------------
// Output schemas
// ---------------------------------------------------------------------------

// BaseOutput is the common envelope for all tool responses.
type BaseOutput struct {
	OK          bool   `json:"ok"`
	OperationID string `json:"operation_id,omitempty"`
	Error       *ErrorOutput `json:"error,omitempty"`
}

// ErrorOutput is the structured error returned in tool responses.
type ErrorOutput struct {
	Code        string                 `json:"code"`
	Category    string                 `json:"category"`
	Message     string                 `json:"message"`
	Datasource  string                 `json:"datasource,omitempty"`
	OperationID string                 `json:"operation_id,omitempty"`
	Retryable   bool                   `json:"retryable"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// DatasourceInfo describes a single datasource returned by datasource_list.
type DatasourceInfo struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Environment     string `json:"environment"`
	Writable        bool   `json:"writable"`
	DefaultDatabase string `json:"default_database,omitempty"`
}

// DatasourceListOutput is the output for the datasource_list tool.
type DatasourceListOutput struct {
	BaseOutput
	Datasources []DatasourceInfo `json:"datasources"`
}

// DatasourceHealthcheckOutput is the output for the datasource_healthcheck tool.
type DatasourceHealthcheckOutput struct {
	BaseOutput
	Datasource string                 `json:"datasource"`
	Mode       string                 `json:"mode"`
	Status     string                 `json:"status"` // "healthy" or "unhealthy"
	Details    map[string]interface{} `json:"details,omitempty"`
}

// SQLSchemaListOutput is the output for the sql_schema_list tool.
type SQLSchemaListOutput struct {
	BaseOutput
	Schemas []string `json:"schemas"`
}

// SQLObjectTypeListOutput is the output for the sql_object_type_list tool.
type SQLObjectTypeListOutput struct {
	BaseOutput
	ObjectTypes []string `json:"object_types"`
}

// SQLObjectInfo describes a single SQL object (table, view, etc.).
type SQLObjectInfo struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

// SQLObjectListOutput is the output for the sql_object_list tool.
type SQLObjectListOutput struct {
	BaseOutput
	Objects []SQLObjectInfo `json:"objects"`
}

// SQLColumnInfo describes a column including nullability.
type SQLColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// SQLIndexInfo describes an index on a SQL object.
type SQLIndexInfo struct {
	Name     string   `json:"name"`
	Columns  []string `json:"columns"`
	Unique   bool     `json:"unique"`
}

// SQLObjectDescribeOutput is the output for the sql_object_describe tool.
type SQLObjectDescribeOutput struct {
	BaseOutput
	Object     SQLObjectInfo   `json:"object"`
	Columns    []SQLColumnInfo `json:"columns"`
	Indexes    []SQLIndexInfo  `json:"indexes,omitempty"`
	Definition string          `json:"definition,omitempty"`
}

// SQLTablePreviewOutput is the output for the sql_table_preview tool.
type SQLTablePreviewOutput struct {
	BaseOutput
	Columns  []ColumnInfo            `json:"columns"`
	Rows     []map[string]interface{} `json:"rows"`
	RowCount int                      `json:"row_count"`
	Elapsed  int64                    `json:"elapsed_ms"`
}

// ColumnInfo describes a column in a SQL result set (matches backend.ColumnInfo).
type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// RedisKeyScanOutput is the output for the redis_key_scan tool.
type RedisKeyScanOutput struct {
	BaseOutput
	Keys       []string `json:"keys"`
	NextCursor string   `json:"next_cursor"`
	Truncated  bool     `json:"truncated"`
}

// RedisKeyDescribeOutput is the output for the redis_key_describe tool.
type RedisKeyDescribeOutput struct {
	BaseOutput
	Key    string `json:"key"`
	Type   string `json:"type"`
	TTL    int64  `json:"ttl_ms"`
	Length int64  `json:"length"`
	Exists bool   `json:"exists"`
}

// MongoDatabaseListOutput is the output for the mongo_database_list tool.
type MongoDatabaseListOutput struct {
	BaseOutput
	Databases []string `json:"databases"`
}

// MongoCollectionInfo describes a single MongoDB collection.
type MongoCollectionInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// MongoCollectionListOutput is the output for the mongo_collection_list tool.
type MongoCollectionListOutput struct {
	BaseOutput
	Collections []MongoCollectionInfo `json:"collections"`
}

// MongoIndexInfo describes a MongoDB index.
type MongoIndexInfo struct {
	Name   string                 `json:"name"`
	Keys   map[string]interface{} `json:"keys"`
	Unique bool                   `json:"unique,omitempty"`
}

// MongoCollectionDescribeOutput is the output for the mongo_collection_describe tool.
type MongoCollectionDescribeOutput struct {
	BaseOutput
	Collection     string                   `json:"collection"`
	EstimatedCount int64                    `json:"estimated_count"`
	Indexes        []MongoIndexInfo         `json:"indexes"`
	SampleSchema   map[string]interface{}   `json:"sample_schema,omitempty"`
}

// SQLQueryOutput is the output for the sql_query tool.
type SQLQueryOutput struct {
	BaseOutput
	Columns  []ColumnInfo            `json:"columns"`
	Rows     []map[string]interface{} `json:"rows"`
	RowCount int                      `json:"row_count"`
	Elapsed  int64                    `json:"elapsed_ms"`
}

// SQLPrepareChangeOutput is the output for the sql_prepare_change tool.
type SQLPrepareChangeOutput struct {
	BaseOutput
	ConfirmationID string                 `json:"confirmation_id"`
	Kind           string                 `json:"kind"`
	Datasource     string                 `json:"datasource"`
	RiskLevel      string                 `json:"risk_level"`
	Impact         map[string]interface{} `json:"impact"`
	Summary        string                 `json:"summary"`
	ExpiresAt      string                 `json:"expires_at"`
}

// RedisCommandOutput is the output for the redis_command tool.
type RedisCommandOutput struct {
	BaseOutput
	Result  interface{} `json:"result"`
	Elapsed int64       `json:"elapsed_ms"`
}

// RedisPrepareChangeOutput is the output for the redis_prepare_change tool.
type RedisPrepareChangeOutput struct {
	BaseOutput
	ConfirmationID string                 `json:"confirmation_id"`
	Kind           string                 `json:"kind"`
	Datasource     string                 `json:"datasource"`
	RiskLevel      string                 `json:"risk_level"`
	Impact         map[string]interface{} `json:"impact"`
	Summary        string                 `json:"summary"`
	ExpiresAt      string                 `json:"expires_at"`
}

// MongoFindOutput is the output for the mongo_find tool.
type MongoFindOutput struct {
	BaseOutput
	Documents []map[string]interface{} `json:"documents"`
	Count     int                      `json:"count"`
	Elapsed   int64                    `json:"elapsed_ms"`
}

// MongoPrepareChangeOutput is the output for the mongo_prepare_change tool.
type MongoPrepareChangeOutput struct {
	BaseOutput
	ConfirmationID string                 `json:"confirmation_id"`
	Kind           string                 `json:"kind"`
	Datasource     string                 `json:"datasource"`
	RiskLevel      string                 `json:"risk_level"`
	Impact         map[string]interface{} `json:"impact"`
	Summary        string                 `json:"summary"`
	ExpiresAt      string                 `json:"expires_at"`
}

// ExecuteConfirmationOutput is the output for the execute_confirmation tool.
type ExecuteConfirmationOutput struct {
	BaseOutput
	ConfirmationID string `json:"confirmation_id"`
	Status         string `json:"status"`
	AffectedCount  int64  `json:"affected_count"`
	ResultSummary  string `json:"result_summary"`
	Elapsed        int64  `json:"elapsed_ms"`
}

// OperationInfo describes a single operation in the operations list.
type OperationInfo struct {
	OperationID        string  `json:"operation_id"`
	Kind               string  `json:"kind"`
	Datasource         string  `json:"datasource"`
	Status             string  `json:"status"`
	StartedAt          string  `json:"started_at"`
	FinishedAt         *string `json:"finished_at,omitempty"`
	CancelRequestedAt  *string `json:"cancel_requested_at,omitempty"`
	ConfirmationID     *string `json:"confirmation_id,omitempty"`
	ErrorCode          *string `json:"error_code,omitempty"`
	ErrorSummary       *string `json:"error_summary,omitempty"`
}

// OperationListOutput is the output for the operation_list tool.
type OperationListOutput struct {
	BaseOutput
	Operations []OperationInfo `json:"operations"`
}

// CancelOperationOutput is the output for the cancel_operation tool.
type CancelOperationOutput struct {
	BaseOutput
	Status string `json:"status"`
}

// AuditEventInfo describes a single audit event.
type AuditEventInfo struct {
	ID             string  `json:"id"`
	EventType      string  `json:"event_type"`
	Datasource     string  `json:"datasource"`
	Status         string  `json:"status"`
	Summary        string  `json:"summary"`
	CreatedAt      string  `json:"created_at"`
	OperationID    *string `json:"operation_id,omitempty"`
	ConfirmationID *string `json:"confirmation_id,omitempty"`
	ElapsedMs      int64   `json:"elapsed_ms,omitempty"`
	RowCount       int     `json:"row_count,omitempty"`
	ErrorCode      *string `json:"error_code,omitempty"`
}

// AuditRecentOutput is the output for the audit_recent tool.
type AuditRecentOutput struct {
	BaseOutput
	Events []AuditEventInfo `json:"events"`
}

// ConfirmationInfo describes the current state of a confirmation.
type ConfirmationInfo struct {
	ConfirmationID string `json:"confirmation_id"`
	Kind           string `json:"kind"`
	Datasource     string `json:"datasource"`
	Status         string `json:"status"`
	RiskLevel      string `json:"risk_level"`
	Summary        string `json:"summary"`
	ExpiresAt      string `json:"expires_at"`
}

// ConfirmationGetOutput is the output for the confirmation_get tool.
type ConfirmationGetOutput struct {
	BaseOutput
	Confirmation ConfirmationInfo `json:"confirmation"`
}

// CancelConfirmationOutput is the output for the cancel_confirmation tool.
type CancelConfirmationOutput struct {
	BaseOutput
	ConfirmationID string `json:"confirmation_id"`
	Status         string `json:"status"`
}
