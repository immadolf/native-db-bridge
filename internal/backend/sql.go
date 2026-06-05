package backend

import "time"

// ColumnInfo describes a single column in a SQL result set.
type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SQLResult holds the outcome of a read-only SQL query.
type SQLResult struct {
	Columns  []ColumnInfo              `json:"columns"`
	Rows     []map[string]interface{}  `json:"rows"`
	RowCount int                       `json:"row_count"`
	Elapsed  time.Duration             `json:"elapsed"`
}
