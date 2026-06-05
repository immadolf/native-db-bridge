package backend

import (
	"context"
	"time"
)

// ExecResult holds the outcome of a write/exec operation.
// Used by both SQLBackend.Exec and MongoBackend.Write.
type ExecResult struct {
	AffectedCount int64         `json:"affected_count"`
	Elapsed       time.Duration `json:"elapsed"`
}

// SQLBackend abstracts SQL database execution (MySQL, PostgreSQL, SQLite).
type SQLBackend interface {
	// Ping verifies connectivity to the given datasource.
	Ping(ctx context.Context, datasource string) error
	// Query executes a read-only SQL statement and returns rows up to limit.
	Query(ctx context.Context, datasource string, sql string, limit int) (SQLResult, error)
	// Exec executes a write SQL statement and returns affected row count.
	Exec(ctx context.Context, datasource string, sql string) (ExecResult, error)
	// PreviewTable returns up to limit rows from the given schema.table.
	PreviewTable(ctx context.Context, datasource, schema, table string, limit int) (SQLResult, error)
}

// RedisBackend abstracts Redis command execution.
type RedisBackend interface {
	// Ping verifies connectivity to the given datasource.
	Ping(ctx context.Context, datasource string) error
	// Command executes a single Redis command and returns the raw result.
	Command(ctx context.Context, datasource, command string, args []string) (RedisResult, error)
	// ScanKeys iterates keys matching the pattern using SCAN with cursor.
	ScanKeys(ctx context.Context, datasource, match, cursor string, count int) (RedisScanResult, error)
	// KeyDescribe returns metadata about a single Redis key.
	KeyDescribe(ctx context.Context, datasource, key string) (RedisKeyDescription, error)
}

// MongoBackend abstracts MongoDB operations.
type MongoBackend interface {
	// Ping verifies connectivity to the given datasource.
	Ping(ctx context.Context, datasource string) error
	// Find executes find or aggregate operations and returns matching documents.
	Find(ctx context.Context, req MongoFindRequest) (MongoResult, error)
	// Write executes insert, update, or delete operations.
	Write(ctx context.Context, req MongoWriteRequest) (ExecResult, error)
	// ListDatabases returns all database names for the datasource.
	ListDatabases(ctx context.Context, datasource string) ([]string, error)
	// ListCollections returns collection names matching the optional pattern.
	ListCollections(ctx context.Context, datasource, pattern string) ([]MongoCollection, error)
	// DescribeCollection returns metadata and sample schema for a collection.
	DescribeCollection(ctx context.Context, datasource, collection string) (MongoCollectionDescription, error)
}
