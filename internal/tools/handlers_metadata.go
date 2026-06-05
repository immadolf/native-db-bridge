package tools

import (
	"context"
	"fmt"

	"native-db-bridge-mcp/internal/nbderrors"
)

// ---------------------------------------------------------------------------
// Metadata tools
// ---------------------------------------------------------------------------

// DatasourceList returns datasources filtered by type and environment.
func (h *Handlers) DatasourceList(_ context.Context, input DatasourceListInput) DatasourceListOutput {
	var results []DatasourceInfo

	for _, ds := range h.deps.Config.Datasources.SQL {
		if input.Type != "" && input.Type != "sql" {
			continue
		}
		if input.Environment != "" && ds.Environment != input.Environment {
			continue
		}
		results = append(results, DatasourceInfo{
			Name:            ds.Name,
			Type:            "sql",
			Environment:     ds.Environment,
			Writable:        true,
			DefaultDatabase: ds.DefaultDatabase,
		})
	}

	for _, ds := range h.deps.Config.Datasources.Redis {
		if input.Type != "" && input.Type != "redis" {
			continue
		}
		if input.Environment != "" && ds.Environment != input.Environment {
			continue
		}
		results = append(results, DatasourceInfo{
			Name:        ds.Name,
			Type:        "redis",
			Environment: ds.Environment,
			Writable:    true,
		})
	}

	for _, ds := range h.deps.Config.Datasources.Mongo {
		if input.Type != "" && input.Type != "mongo" {
			continue
		}
		if input.Environment != "" && ds.Environment != input.Environment {
			continue
		}
		results = append(results, DatasourceInfo{
			Name:            ds.Name,
			Type:            "mongo",
			Environment:     ds.Environment,
			Writable:        true,
			DefaultDatabase: ds.DefaultDatabase,
		})
	}

	return DatasourceListOutput{
		BaseOutput:  BaseOutput{OK: true},
		Datasources: results,
	}
}

// DatasourceHealthcheck checks datasource configuration or connectivity.
func (h *Handlers) DatasourceHealthcheck(ctx context.Context, input DatasourceHealthcheckInput) DatasourceHealthcheckOutput {
	dsType, _, found := h.findDatasource(input.Datasource)
	if !found {
		return DatasourceHealthcheckOutput{
			BaseOutput: makeError(nbderrors.CodeConfigDatasourceNotFound,
				fmt.Sprintf("datasource %q not found", input.Datasource)),
			Datasource: input.Datasource,
			Mode:       input.Mode,
			Status:     "unhealthy",
		}
	}

	if input.Mode == "config" {
		return DatasourceHealthcheckOutput{
			BaseOutput:  BaseOutput{OK: true},
			Datasource:  input.Datasource,
			Mode:        "config",
			Status:      "healthy",
			Details:     map[string]interface{}{"type": dsType},
		}
	}

	// connect mode: ping the backend
	var err error
	switch dsType {
	case "sql":
		err = h.deps.SQL.Ping(ctx, input.Datasource)
	case "redis":
		err = h.deps.Redis.Ping(ctx, input.Datasource)
	case "mongo":
		err = h.deps.Mongo.Ping(ctx, input.Datasource)
	}

	if err != nil {
		return DatasourceHealthcheckOutput{
			BaseOutput: makeError(nbderrors.CodeConnectionFailed,
				fmt.Sprintf("connection failed: %v", err)),
			Datasource: input.Datasource,
			Mode:       "connect",
			Status:     "unhealthy",
		}
	}

	return DatasourceHealthcheckOutput{
		BaseOutput:  BaseOutput{OK: true},
		Datasource:  input.Datasource,
		Mode:        "connect",
		Status:      "healthy",
	}
}

// SQLSchemaList returns available schemas for a SQL datasource.
func (h *Handlers) SQLSchemaList(ctx context.Context, input SQLSchemaListInput) SQLSchemaListOutput {
	schemas, err := h.deps.SQL.SchemaList(ctx, input.Datasource)
	if err != nil {
		return SQLSchemaListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return SQLSchemaListOutput{BaseOutput: BaseOutput{OK: true}, Schemas: schemas}
}

// SQLObjectTypeList returns available object types for a SQL datasource.
func (h *Handlers) SQLObjectTypeList(ctx context.Context, input SQLObjectTypeListInput) SQLObjectTypeListOutput {
	types, err := h.deps.SQL.ObjectTypeList(ctx, input.Datasource)
	if err != nil {
		return SQLObjectTypeListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return SQLObjectTypeListOutput{BaseOutput: BaseOutput{OK: true}, ObjectTypes: types}
}

// SQLObjectList returns objects matching the criteria.
func (h *Handlers) SQLObjectList(ctx context.Context, input SQLObjectListInput) SQLObjectListOutput {
	objects, err := h.deps.SQL.ObjectList(ctx, input.Datasource, input.Schema, input.ObjectType, input.NamePattern)
	if err != nil {
		return SQLObjectListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return SQLObjectListOutput{BaseOutput: BaseOutput{OK: true}, Objects: objects}
}

// SQLObjectDescribe returns detailed metadata for a SQL object.
func (h *Handlers) SQLObjectDescribe(ctx context.Context, input SQLObjectDescribeInput) SQLObjectDescribeOutput {
	result, err := h.deps.SQL.DescribeObject(ctx, input.Datasource, input.Schema, input.ObjectName, input.ObjectType)
	if err != nil {
		return SQLObjectDescribeOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return SQLObjectDescribeOutput{
		BaseOutput: BaseOutput{OK: true},
		Object:     result.Object,
		Columns:    result.Columns,
		Indexes:    result.Indexes,
		Definition: result.Definition,
	}
}

// SQLTablePreview returns a preview of rows from a table.
func (h *Handlers) SQLTablePreview(ctx context.Context, input SQLTablePreviewInput) SQLTablePreviewOutput {
	limit := input.Limit
	if limit <= 0 {
		limit = h.deps.Config.Server.MaxResultRows
	}
	if limit > h.deps.Config.Server.MaxResultRows {
		limit = h.deps.Config.Server.MaxResultRows
	}

	result, err := h.deps.SQL.PreviewTable(ctx, input.Datasource, input.Schema, input.Table, limit)
	if err != nil {
		return SQLTablePreviewOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}

	cols := make([]ColumnInfo, len(result.Columns))
	for i, c := range result.Columns {
		cols[i] = ColumnInfo{Name: c.Name, Type: c.Type}
	}

	return SQLTablePreviewOutput{
		BaseOutput: BaseOutput{OK: true},
		Columns:    cols,
		Rows:       result.Rows,
		RowCount:   result.RowCount,
		Elapsed:    result.Elapsed.Milliseconds(),
	}
}

// RedisKeyScan scans for keys matching a pattern, capped at redis_scan_count_max.
func (h *Handlers) RedisKeyScan(ctx context.Context, input RedisKeyScanInput) RedisKeyScanOutput {
	count := input.Count
	if count <= 0 || count > h.deps.Config.Server.RedisScanCountMax {
		count = h.deps.Config.Server.RedisScanCountMax
	}

	cursor := input.Cursor
	if cursor == "" {
		cursor = "0"
	}

	result, err := h.deps.Redis.ScanKeys(ctx, input.Datasource, input.Match, cursor, count)
	if err != nil {
		return RedisKeyScanOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}

	return RedisKeyScanOutput{
		BaseOutput: BaseOutput{OK: true},
		Keys:       result.Keys,
		NextCursor: result.NextCursor,
		Truncated:  result.Truncated,
	}
}

// RedisKeyDescribe returns metadata about a single Redis key.
func (h *Handlers) RedisKeyDescribe(ctx context.Context, input RedisKeyDescribeInput) RedisKeyDescribeOutput {
	desc, err := h.deps.Redis.KeyDescribe(ctx, input.Datasource, input.Key)
	if err != nil {
		return RedisKeyDescribeOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return RedisKeyDescribeOutput{
		BaseOutput: BaseOutput{OK: true},
		Key:        desc.Key,
		Type:       desc.Type,
		TTL:        desc.TTL,
		Length:     desc.Length,
		Exists:     desc.Exists,
	}
}

// MongoDatabaseList returns databases for a MongoDB datasource.
func (h *Handlers) MongoDatabaseList(ctx context.Context, input MongoDatabaseListInput) MongoDatabaseListOutput {
	dbs, err := h.deps.Mongo.ListDatabases(ctx, input.Datasource)
	if err != nil {
		return MongoDatabaseListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return MongoDatabaseListOutput{BaseOutput: BaseOutput{OK: true}, Databases: dbs}
}

// MongoCollectionList returns collections for a MongoDB datasource.
func (h *Handlers) MongoCollectionList(ctx context.Context, input MongoCollectionListInput) MongoCollectionListOutput {
	colls, err := h.deps.Mongo.ListCollections(ctx, input.Datasource, input.NamePattern)
	if err != nil {
		return MongoCollectionListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}

	results := make([]MongoCollectionInfo, len(colls))
	for i, c := range colls {
		results[i] = MongoCollectionInfo{Name: c.Name, Type: c.Type}
	}

	return MongoCollectionListOutput{BaseOutput: BaseOutput{OK: true}, Collections: results}
}

// MongoCollectionDescribe returns metadata for a MongoDB collection.
func (h *Handlers) MongoCollectionDescribe(ctx context.Context, input MongoCollectionDescribeInput) MongoCollectionDescribeOutput {
	desc, err := h.deps.Mongo.DescribeCollection(ctx, input.Datasource, input.Collection)
	if err != nil {
		return MongoCollectionDescribeOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}

	indexes := make([]MongoIndexInfo, len(desc.Indexes))
	for i, idx := range desc.Indexes {
		indexes[i] = MongoIndexInfo{Name: idx.Name, Keys: idx.Key, Unique: idx.Unique}
	}

	return MongoCollectionDescribeOutput{
		BaseOutput:     BaseOutput{OK: true},
		Collection:     desc.Collection,
		EstimatedCount: desc.EstimatedCount,
		Indexes:        indexes,
		SampleSchema:   desc.SampleSchema,
	}
}
