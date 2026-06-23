package tools

import (
	"context"
	"fmt"

	"native-db-bridge-mcp/internal/backend"
	"native-db-bridge-mcp/internal/nbderrors"
)

// ---------------------------------------------------------------------------
// Metadata tools
// ---------------------------------------------------------------------------

// DatasourceList returns datasources filtered by type and environment.
func (h *Handlers) DatasourceList(ctx context.Context, input DatasourceListInput) DatasourceListOutput {
	_, opID := h.withOperation(ctx, "datasource_list", "", "")
	defer h.finishOperation(opID, nil)

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
	ctx, opID := h.withOperation(ctx, "datasource_healthcheck", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	dsType, _, found := h.findDatasource(input.Datasource)
	if !found {
		opErr = fmt.Errorf("datasource %q not found", input.Datasource)
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
			BaseOutput: BaseOutput{OK: true},
			Datasource: input.Datasource,
			Mode:       "config",
			Status:     "healthy",
			Details:    map[string]interface{}{"type": dsType},
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
		opErr = err
		return DatasourceHealthcheckOutput{
			BaseOutput: makeError(nbderrors.CodeConnectionFailed,
				fmt.Sprintf("connection failed: %v", err)),
			Datasource: input.Datasource,
			Mode:       "connect",
			Status:     "unhealthy",
		}
	}

	return DatasourceHealthcheckOutput{
		BaseOutput: BaseOutput{OK: true},
		Datasource: input.Datasource,
		Mode:       "connect",
		Status:     "healthy",
	}
}

// SQLSchemaList returns available schemas for a SQL datasource.
func (h *Handlers) SQLSchemaList(ctx context.Context, input SQLSchemaListInput) SQLSchemaListOutput {
	ctx, opID := h.withOperation(ctx, "sql_schema_list", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	schemas, err := h.deps.SQL.SchemaList(ctx, input.Datasource)
	if err != nil {
		opErr = err
		return SQLSchemaListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return SQLSchemaListOutput{BaseOutput: BaseOutput{OK: true}, Schemas: schemas}
}

// SQLObjectTypeList returns available object types for a SQL datasource.
func (h *Handlers) SQLObjectTypeList(ctx context.Context, input SQLObjectTypeListInput) SQLObjectTypeListOutput {
	ctx, opID := h.withOperation(ctx, "sql_object_type_list", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	types, err := h.deps.SQL.ObjectTypeList(ctx, input.Datasource)
	if err != nil {
		opErr = err
		return SQLObjectTypeListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return SQLObjectTypeListOutput{BaseOutput: BaseOutput{OK: true}, ObjectTypes: types}
}

// SQLObjectList returns objects matching the criteria.
func (h *Handlers) SQLObjectList(ctx context.Context, input SQLObjectListInput) SQLObjectListOutput {
	ctx, opID := h.withOperation(ctx, "sql_object_list", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	objects, err := h.deps.SQL.ObjectList(ctx, input.Datasource, input.Schema, input.ObjectType, input.NamePattern)
	if err != nil {
		opErr = err
		return SQLObjectListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return SQLObjectListOutput{BaseOutput: BaseOutput{OK: true}, Objects: objects}
}

// SQLObjectDescribe returns detailed metadata for a SQL object.
func (h *Handlers) SQLObjectDescribe(ctx context.Context, input SQLObjectDescribeInput) SQLObjectDescribeOutput {
	ctx, opID := h.withOperation(ctx, "sql_object_describe", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	result, err := h.deps.SQL.DescribeObject(ctx, input.Datasource, input.Schema, input.ObjectName, input.ObjectType)
	if err != nil {
		opErr = err
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
	ctx, opID := h.withOperation(ctx, "sql_table_preview", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	limit := input.Limit
	if limit <= 0 {
		limit = h.deps.Config.Server.MaxResultRows
	}
	if limit > h.deps.Config.Server.MaxResultRows {
		limit = h.deps.Config.Server.MaxResultRows
	}

	result, err := h.deps.SQL.PreviewTable(ctx, input.Datasource, input.Schema, input.Table, limit)
	if err != nil {
		opErr = err
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

// SQLColumnSearch searches MySQL column metadata without scanning business rows.
func (h *Handlers) SQLColumnSearch(ctx context.Context, input SQLColumnSearchInput) SQLColumnSearchOutput {
	ctx, opID := h.withOperation(ctx, "sql_column_search", input.Datasource, "")
	var opErr error
	defer func() { h.finishSQLOperation(opID, opErr) }()

	if input.Schema == "" {
		opErr = fmt.Errorf("schema is required")
		return SQLColumnSearchOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, "schema is required")}
	}

	results, err := h.deps.SQL.ColumnSearch(ctx, backend.SQLColumnSearchRequest{
		Datasource:    input.Datasource,
		Schema:        input.Schema,
		TablePattern:  input.TablePattern,
		ColumnPattern: input.ColumnPattern,
		DataTypes:     input.DataTypes,
		Limit:         input.Limit,
	})
	if err != nil {
		opErr = err
		return SQLColumnSearchOutput{BaseOutput: makeErrorFromNative(nbderrors.ClassifySQLErrorMessage(err.Error()))}
	}
	return SQLColumnSearchOutput{BaseOutput: BaseOutput{OK: true}, Columns: results}
}

// SQLTextColumnPlan builds a metadata-only text-column scan plan.
func (h *Handlers) SQLTextColumnPlan(ctx context.Context, input SQLTextColumnPlanInput) SQLTextColumnPlanOutput {
	ctx, opID := h.withOperation(ctx, "sql_text_column_plan", input.Datasource, "")
	var opErr error
	defer func() { h.finishSQLOperation(opID, opErr) }()

	if input.Schema == "" {
		opErr = fmt.Errorf("schema is required")
		return SQLTextColumnPlanOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, "schema is required")}
	}

	result, err := h.deps.SQL.TextColumnPlan(ctx, backend.SQLTextColumnPlanRequest{
		Datasource:    input.Datasource,
		Schema:        input.Schema,
		TablePattern:  input.TablePattern,
		ColumnPattern: input.ColumnPattern,
		Keywords:      input.Keywords,
		MaxTables:     input.MaxTables,
		MaxColumns:    input.MaxColumns,
	})
	if err != nil {
		opErr = err
		return SQLTextColumnPlanOutput{BaseOutput: makeErrorFromNative(nbderrors.ClassifySQLErrorMessage(err.Error()))}
	}
	return SQLTextColumnPlanOutput{
		BaseOutput: BaseOutput{OK: true},
		Candidates: result.Candidates,
		Batches:    result.Batches,
		Warnings:   result.Warnings,
	}
}

// RedisKeyScan scans for keys matching a pattern, capped at redis_scan_count_max.
func (h *Handlers) RedisKeyScan(ctx context.Context, input RedisKeyScanInput) RedisKeyScanOutput {
	ctx, opID := h.withOperation(ctx, "redis_key_scan", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

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
		opErr = err
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
	ctx, opID := h.withOperation(ctx, "redis_key_describe", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	desc, err := h.deps.Redis.KeyDescribe(ctx, input.Datasource, input.Key)
	if err != nil {
		opErr = err
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
	ctx, opID := h.withOperation(ctx, "mongo_database_list", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	dbs, err := h.deps.Mongo.ListDatabases(ctx, input.Datasource)
	if err != nil {
		opErr = err
		return MongoDatabaseListOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error())}
	}
	return MongoDatabaseListOutput{BaseOutput: BaseOutput{OK: true}, Databases: dbs}
}

// MongoCollectionList returns collections for a MongoDB datasource.
func (h *Handlers) MongoCollectionList(ctx context.Context, input MongoCollectionListInput) MongoCollectionListOutput {
	ctx, opID := h.withOperation(ctx, "mongo_collection_list", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	colls, err := h.deps.Mongo.ListCollections(ctx, input.Datasource, input.NamePattern)
	if err != nil {
		opErr = err
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
	ctx, opID := h.withOperation(ctx, "mongo_collection_describe", input.Datasource, "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	desc, err := h.deps.Mongo.DescribeCollection(ctx, input.Datasource, input.Collection)
	if err != nil {
		opErr = err
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
