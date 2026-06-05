package backend

import "time"

// MongoFindRequest holds parameters for find or aggregate operations.
type MongoFindRequest struct {
	Datasource string                 `json:"datasource"`
	Collection string                 `json:"collection"`
	Operation  string                 `json:"operation"` // "find" or "aggregate"
	Filter     map[string]interface{} `json:"filter,omitempty"`
	Pipeline   []interface{}          `json:"pipeline,omitempty"`
	Projection map[string]interface{} `json:"projection,omitempty"`
	Sort       map[string]interface{} `json:"sort,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
	Timeout    time.Duration          `json:"timeout,omitempty"`
}

// MongoWriteRequest holds parameters for insert, update, or delete operations.
type MongoWriteRequest struct {
	Datasource string                 `json:"datasource"`
	Collection string                 `json:"collection"`
	Operation  string                 `json:"operation"` // "insertOne", "insertMany", "updateOne", "updateMany", "deleteOne", "deleteMany"
	Filter     map[string]interface{} `json:"filter,omitempty"`
	Document   map[string]interface{} `json:"document,omitempty"`
	Documents  []map[string]interface{} `json:"documents,omitempty"`
	Timeout    time.Duration          `json:"timeout,omitempty"`
}

// MongoResult holds the outcome of a MongoDB find operation.
type MongoResult struct {
	Documents []map[string]interface{} `json:"documents"`
	Count     int                      `json:"count"`
	Elapsed   time.Duration            `json:"elapsed"`
}

// MongoCollection describes a single MongoDB collection.
type MongoCollection struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// IndexInfo describes a MongoDB index.
type IndexInfo struct {
	Name   string                 `json:"name"`
	Key    map[string]interface{} `json:"key"`
	Unique bool                   `json:"unique,omitempty"`
}

// MongoCollectionDescription holds metadata about a MongoDB collection.
type MongoCollectionDescription struct {
	Collection      string                   `json:"collection"`
	EstimatedCount  int64                    `json:"estimated_count"`
	Indexes         []IndexInfo              `json:"indexes"`
	SampleSchema    map[string]interface{}   `json:"sample_schema,omitempty"`
}
