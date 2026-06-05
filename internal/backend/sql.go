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
