package backend

import "time"

// ColumnInfo describes a single column in a SQL result set.
type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SQLResult holds the outcome of a read-only SQL query.
type SQLResult struct {
	Columns  []ColumnInfo             `json:"columns"`
	Rows     []map[string]interface{} `json:"rows"`
	RowCount int                      `json:"row_count"`
	Elapsed  time.Duration            `json:"elapsed"`
}

// SQLObjectInfo describes a single SQL object (table, view, etc.).
type SQLObjectInfo struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

// SQLColumnInfo describes a column including nullability.
type SQLColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// SQLIndexInfo describes an index on a SQL object.
type SQLIndexInfo struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

// SQLDescribeResult holds the full description of a SQL object.
type SQLDescribeResult struct {
	Object     SQLObjectInfo   `json:"object"`
	Columns    []SQLColumnInfo `json:"columns"`
	Indexes    []SQLIndexInfo  `json:"indexes,omitempty"`
	Definition string          `json:"definition,omitempty"`
}

// SQLColumnSearchRequest describes a metadata-only search over
// information_schema.COLUMNS.
type SQLColumnSearchRequest struct {
	Datasource    string
	Schema        string
	TablePattern  string
	ColumnPattern string
	DataTypes     []string
	Limit         int
}

// SQLColumnSearchResult describes one matching MySQL column.
type SQLColumnSearchResult struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Column     string `json:"column"`
	DataType   string `json:"data_type"`
	ColumnType string `json:"column_type"`
	Nullable   bool   `json:"nullable"`
	ColumnKey  string `json:"column_key,omitempty"`
	Comment    string `json:"comment,omitempty"`
}

// SQLTextColumnPlanRequest describes a metadata-only text-column scan plan.
type SQLTextColumnPlanRequest struct {
	Datasource    string
	Schema        string
	TablePattern  string
	ColumnPattern string
	Keywords      []string
	MaxTables     int
	MaxColumns    int
}

// SQLTextColumnCandidate describes one text-like column that can be scanned.
type SQLTextColumnCandidate struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Column     string `json:"column"`
	DataType   string `json:"data_type"`
	ColumnType string `json:"column_type"`
}

// SQLTextScanTarget identifies a single table column for text scanning.
type SQLTextScanTarget struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

// SQLTextScanBatch groups text scan targets and keywords for agent execution.
type SQLTextScanBatch struct {
	Targets  []SQLTextScanTarget `json:"targets"`
	Keywords []string            `json:"keywords"`
}

// SQLTextColumnPlanResult contains candidate text columns and suggested batches.
type SQLTextColumnPlanResult struct {
	Candidates []SQLTextColumnCandidate `json:"candidates"`
	Batches    []SQLTextScanBatch       `json:"batches"`
	Warnings   []string                 `json:"warnings,omitempty"`
}

// SQLTextScanRequest describes a controlled count-only text scan.
type SQLTextScanRequest struct {
	Datasource         string
	Schema             string
	Targets            []SQLTextScanTarget
	Keywords           []string
	Mode               string
	MaxColumnsPerQuery int
	Timeout            time.Duration
}

// SQLTextScanMatch describes one target-keyword count result.
type SQLTextScanMatch struct {
	Table     string        `json:"table"`
	Column    string        `json:"column"`
	Keyword   string        `json:"keyword"`
	Count     int64         `json:"count"`
	Elapsed   time.Duration `json:"elapsed"`
	TimedOut  bool          `json:"timed_out,omitempty"`
	Error     string        `json:"error,omitempty"`
	ErrorCode string        `json:"error_code,omitempty"`
}

// SQLTextScanResult contains all count-only text scan matches.
type SQLTextScanResult struct {
	Matches []SQLTextScanMatch `json:"matches"`
}
