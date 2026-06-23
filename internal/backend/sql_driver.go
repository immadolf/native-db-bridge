package backend

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/lifecycle"
	"native-db-bridge-mcp/internal/nbderrors"
)

// sqlResource wraps a *sql.DB to satisfy lifecycle.Resource.
type sqlResource struct {
	db *sql.DB
}

func (r *sqlResource) Close() error {
	return r.db.Close()
}

// SQLDriverBackend implements both SQLBackend and the SQLMetaBackend
// interface defined in the tools package. It uses database/sql with
// the go-sql-driver/mysql driver. Connections are managed lazily
// via lifecycle.Manager: no connection is opened until the first
// Acquire call.
type SQLDriverBackend struct {
	cfg     config.Config
	manager *lifecycle.Manager[string]
}

// NewSQLDriverBackend creates a SQL backend that defers connection
// until the first operation. The constructor never opens a database
// connection.
func NewSQLDriverBackend(cfg config.Config) *SQLDriverBackend {
	idleTTL := cfg.ConnectionLifecycle.Defaults.IdleTTL.Duration
	if cfg.ConnectionLifecycle.SQL.IdleTTL.Duration > 0 {
		idleTTL = cfg.ConnectionLifecycle.SQL.IdleTTL.Duration
	}
	if idleTTL == 0 {
		idleTTL = 10 * time.Minute
	}

	factory := func(ctx context.Context, datasource string) (lifecycle.Resource, error) {
		connCfg, dsCfg, err := findSQLConnectionAndDatasource(cfg, datasource)
		if err != nil {
			return nil, err
		}

		dsn := buildDSN(connCfg.DSN, dsCfg.DefaultDatabase)

		db, err := sql.Open(connCfg.Driver, dsn)
		if err != nil {
			return nil, fmt.Errorf("sql open %s: %w", datasource, err)
		}

		if connCfg.Pool.MaxOpenConns > 0 {
			db.SetMaxOpenConns(connCfg.Pool.MaxOpenConns)
		}
		if connCfg.Pool.MaxIdleConns > 0 {
			db.SetMaxIdleConns(connCfg.Pool.MaxIdleConns)
		}
		if connCfg.Pool.ConnMaxLifetime.Duration > 0 {
			db.SetConnMaxLifetime(connCfg.Pool.ConnMaxLifetime.Duration)
		}

		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("sql ping %s: %w", datasource, err)
		}

		return &sqlResource{db: db}, nil
	}

	return &SQLDriverBackend{
		cfg:     cfg,
		manager: lifecycle.NewManager(idleTTL, factory),
	}
}

// Close shuts down the lifecycle manager and all managed connections.
func (b *SQLDriverBackend) Close() error {
	return b.manager.Close()
}

// ActiveConnections returns the number of lazily-created connections
// currently tracked by the lifecycle manager.
func (b *SQLDriverBackend) ActiveConnections() int {
	return b.manager.Len()
}

// Ping verifies connectivity to the given datasource.
func (b *SQLDriverBackend) Ping(ctx context.Context, datasource string) error {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return err
	}
	release()
	return nil
}

// Query executes a read-only SQL statement and returns rows up to limit.
func (b *SQLDriverBackend) Query(ctx context.Context, datasource string, sqlStr string, limit int) (SQLResult, error) {
	start := time.Now()

	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return SQLResult{}, err
	}
	defer release()

	db, err := b.getDB(datasource)
	if err != nil {
		return SQLResult{}, err
	}

	maxRows := b.cfg.Server.MaxResultRows
	if limit <= 0 {
		limit = maxRows
	}
	if limit > maxRows {
		limit = maxRows
	}

	finalSQL, hasLimitParam := applyLimit(sqlStr, limit)

	var rows *sql.Rows
	if hasLimitParam {
		rows, err = db.QueryContext(ctx, finalSQL, limit)
	} else {
		rows, err = db.QueryContext(ctx, finalSQL)
	}
	if err != nil {
		return SQLResult{}, fmt.Errorf("sql query %s: %w", datasource, err)
	}
	defer rows.Close()

	columns, err := rows.ColumnTypes()
	if err != nil {
		return SQLResult{}, fmt.Errorf("sql columns %s: %w", datasource, err)
	}

	colInfos := make([]ColumnInfo, len(columns))
	for i, c := range columns {
		colInfos[i] = ColumnInfo{
			Name: c.Name(),
			Type: c.DatabaseTypeName(),
		}
	}

	var result []map[string]interface{}
	for rows.Next() {
		if len(result) >= limit {
			break
		}
		values := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return SQLResult{}, fmt.Errorf("sql scan %s: %w", datasource, err)
		}
		row := make(map[string]interface{}, len(columns))
		for i, c := range columns {
			row[c.Name()] = normalizeSQLValue(values[i])
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return SQLResult{}, fmt.Errorf("sql rows %s: %w", datasource, err)
	}

	return SQLResult{
		Columns:  colInfos,
		Rows:     result,
		RowCount: len(result),
		Elapsed:  time.Since(start),
	}, nil
}

// Exec executes a write SQL statement and returns affected row count.
func (b *SQLDriverBackend) Exec(ctx context.Context, datasource string, sqlStr string) (ExecResult, error) {
	start := time.Now()

	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return ExecResult{}, err
	}
	defer release()

	db, err := b.getDB(datasource)
	if err != nil {
		return ExecResult{}, err
	}

	res, err := db.ExecContext(ctx, sqlStr)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sql exec %s: %w", datasource, err)
	}

	affected, _ := res.RowsAffected()

	return ExecResult{
		AffectedCount: affected,
		Elapsed:       time.Since(start),
	}, nil
}

// PreviewTable returns up to limit rows from the given schema.table.
func (b *SQLDriverBackend) PreviewTable(ctx context.Context, datasource, schema, table string, limit int) (SQLResult, error) {
	quoted := fmt.Sprintf("`%s`.`%s`", schema, table)
	sqlStr := fmt.Sprintf("SELECT * FROM %s", quoted)
	return b.Query(ctx, datasource, sqlStr, limit)
}

// SchemaList returns available schemas (databases) for a SQL datasource.
func (b *SQLDriverBackend) SchemaList(ctx context.Context, datasource string) ([]string, error) {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return nil, err
	}
	defer release()

	db, err := b.getDB(datasource)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("sql schema list %s: %w", datasource, err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("sql scan schema %s: %w", datasource, err)
		}
		schemas = append(schemas, name)
	}

	return schemas, rows.Err()
}

// ObjectTypeList returns available object types for a SQL datasource.
func (b *SQLDriverBackend) ObjectTypeList(_ context.Context, _ string) ([]string, error) {
	return []string{"table", "view", "procedure", "function"}, nil
}

// ObjectList returns objects matching the criteria.
func (b *SQLDriverBackend) ObjectList(ctx context.Context, datasource, schema, objectType, namePattern string) ([]SQLObjectInfo, error) {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return nil, err
	}
	defer release()

	db, err := b.getDB(datasource)
	if err != nil {
		return nil, err
	}

	var query string
	var args []interface{}

	switch objectType {
	case "table":
		query = "SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'"
		args = append(args, schema)
		if namePattern != "" {
			query += " AND TABLE_NAME LIKE ?"
			args = append(args, namePattern)
		}
	case "view":
		query = "SELECT TABLE_SCHEMA, TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'VIEW'"
		args = append(args, schema)
		if namePattern != "" {
			query += " AND TABLE_NAME LIKE ?"
			args = append(args, namePattern)
		}
	case "procedure":
		query = "SELECT ROUTINE_SCHEMA, ROUTINE_NAME, ROUTINE_TYPE FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = ? AND ROUTINE_TYPE = 'PROCEDURE'"
		args = append(args, schema)
		if namePattern != "" {
			query += " AND ROUTINE_NAME LIKE ?"
			args = append(args, namePattern)
		}
	case "function":
		query = "SELECT ROUTINE_SCHEMA, ROUTINE_NAME, ROUTINE_TYPE FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = ? AND ROUTINE_TYPE = 'FUNCTION'"
		args = append(args, schema)
		if namePattern != "" {
			query += " AND ROUTINE_NAME LIKE ?"
			args = append(args, namePattern)
		}
	default:
		return nil, fmt.Errorf("unsupported object type: %s", objectType)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sql object list %s: %w", datasource, err)
	}
	defer rows.Close()

	var objects []SQLObjectInfo
	for rows.Next() {
		var obj SQLObjectInfo
		var typeName string
		if err := rows.Scan(&obj.Schema, &obj.Name, &typeName); err != nil {
			return nil, fmt.Errorf("sql scan object %s: %w", datasource, err)
		}
		obj.Type = objectType
		objects = append(objects, obj)
	}

	return objects, rows.Err()
}

// DescribeObject returns detailed metadata for a SQL object.
func (b *SQLDriverBackend) DescribeObject(ctx context.Context, datasource, schema, objectName, objectType string) (SQLDescribeResult, error) {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return SQLDescribeResult{}, err
	}
	defer release()

	db, err := b.getDB(datasource)
	if err != nil {
		return SQLDescribeResult{}, err
	}

	result := SQLDescribeResult{
		Object: SQLObjectInfo{
			Schema: schema,
			Name:   objectName,
			Type:   objectType,
		},
	}

	colRows, err := db.QueryContext(ctx,
		"SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION",
		schema, objectName)
	if err != nil {
		return SQLDescribeResult{}, fmt.Errorf("sql describe columns %s: %w", datasource, err)
	}
	defer colRows.Close()

	for colRows.Next() {
		var col SQLColumnInfo
		var nullable string
		if err := colRows.Scan(&col.Name, &col.Type, &nullable); err != nil {
			return SQLDescribeResult{}, fmt.Errorf("sql scan column %s: %w", datasource, err)
		}
		col.Nullable = nullable == "YES"
		result.Columns = append(result.Columns, col)
	}
	if err := colRows.Err(); err != nil {
		return SQLDescribeResult{}, err
	}

	idxRows, err := db.QueryContext(ctx,
		"SELECT INDEX_NAME, GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX), NOT NON_UNIQUE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? GROUP BY INDEX_NAME, NON_UNIQUE",
		schema, objectName)
	if err != nil {
		return SQLDescribeResult{}, fmt.Errorf("sql describe indexes %s: %w", datasource, err)
	}
	defer idxRows.Close()

	for idxRows.Next() {
		var idx SQLIndexInfo
		var columnsCSV string
		if err := idxRows.Scan(&idx.Name, &columnsCSV, &idx.Unique); err != nil {
			return SQLDescribeResult{}, fmt.Errorf("sql scan index %s: %w", datasource, err)
		}
		idx.Columns = strings.Split(columnsCSV, ",")
		result.Indexes = append(result.Indexes, idx)
	}
	if err := idxRows.Err(); err != nil {
		return SQLDescribeResult{}, err
	}

	if objectType == "view" {
		defRow := db.QueryRowContext(ctx,
			"SELECT VIEW_DEFINITION FROM information_schema.VIEWS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?",
			schema, objectName)
		var def sql.NullString
		if err := defRow.Scan(&def); err == nil && def.Valid {
			result.Definition = def.String
		}
	}

	return result, nil
}

// ColumnSearch returns matching MySQL columns from information_schema.COLUMNS.
func (b *SQLDriverBackend) ColumnSearch(ctx context.Context, req SQLColumnSearchRequest) ([]SQLColumnSearchResult, error) {
	release, err := b.manager.Acquire(ctx, req.Datasource)
	if err != nil {
		return nil, err
	}
	defer release()

	db, err := b.getDB(req.Datasource)
	if err != nil {
		return nil, err
	}

	query, args := buildColumnSearchQuery(req, b.cfg.Server.MaxResultRows)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sql column search %s: %w", req.Datasource, err)
	}
	defer rows.Close()

	var results []SQLColumnSearchResult
	for rows.Next() {
		var item SQLColumnSearchResult
		var nullable string
		if err := rows.Scan(&item.Schema, &item.Table, &item.Column, &item.DataType, &item.ColumnType, &nullable, &item.ColumnKey, &item.Comment); err != nil {
			return nil, fmt.Errorf("sql scan column search %s: %w", req.Datasource, err)
		}
		item.Nullable = nullable == "YES"
		results = append(results, item)
	}
	return results, rows.Err()
}

// TextColumnPlan builds a metadata-only plan for scanning text-like columns.
func (b *SQLDriverBackend) TextColumnPlan(ctx context.Context, req SQLTextColumnPlanRequest) (SQLTextColumnPlanResult, error) {
	release, err := b.manager.Acquire(ctx, req.Datasource)
	if err != nil {
		return SQLTextColumnPlanResult{}, err
	}
	defer release()

	db, err := b.getDB(req.Datasource)
	if err != nil {
		return SQLTextColumnPlanResult{}, err
	}

	query, args := buildTextColumnPlanQuery(req, b.cfg.Server.MaxResultRows)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return SQLTextColumnPlanResult{}, fmt.Errorf("sql text column plan %s: %w", req.Datasource, err)
	}
	defer rows.Close()

	var candidates []SQLTextColumnCandidate
	for rows.Next() {
		var item SQLTextColumnCandidate
		if err := rows.Scan(&item.Schema, &item.Table, &item.Column, &item.DataType, &item.ColumnType); err != nil {
			return SQLTextColumnPlanResult{}, fmt.Errorf("sql scan text column plan %s: %w", req.Datasource, err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return SQLTextColumnPlanResult{}, err
	}

	var warnings []string
	if len(candidates) == 0 {
		warnings = append(warnings, "未找到匹配的文本列，请放宽 table_pattern 或 column_pattern")
	}
	if req.MaxTables > 0 && countDistinctTables(candidates) >= req.MaxTables {
		warnings = append(warnings, "候选表数量达到 max_tables，请进一步收窄范围")
	}
	const defaultTextScanBatchSize = 5
	if len(candidates) > defaultTextScanBatchSize {
		warnings = append(warnings, fmt.Sprintf("扫描计划按每批最多 %d 个字段给出建议；P0 sql_text_scan 仍按单字段和单关键词逐项执行", defaultTextScanBatchSize))
	}

	return SQLTextColumnPlanResult{
		Candidates: candidates,
		Batches:    buildTextScanBatches(candidates, req.Keywords, defaultTextScanBatchSize),
		Warnings:   warnings,
	}, nil
}

// TextScan executes count-only LIKE scans over explicit text targets.
func (b *SQLDriverBackend) TextScan(ctx context.Context, req SQLTextScanRequest) (SQLTextScanResult, error) {
	release, err := b.manager.Acquire(ctx, req.Datasource)
	if err != nil {
		return SQLTextScanResult{}, err
	}
	defer release()

	db, err := b.getDB(req.Datasource)
	if err != nil {
		return SQLTextScanResult{}, err
	}

	if req.Mode == "" {
		req.Mode = "count"
	}
	if req.Mode != "count" {
		return SQLTextScanResult{}, fmt.Errorf("unsupported text scan mode %q", req.Mode)
	}

	var matches []SQLTextScanMatch
	for _, target := range req.Targets {
		for _, keyword := range req.Keywords {
			start := time.Now()
			query, args := buildTextCountSQL(req.Schema, target, keyword)
			var count int64
			queryCtx, cancel := context.WithTimeout(ctx, textScanTimeout(req.Timeout, b.cfg.Server.QueryTimeout.Duration))
			err := db.QueryRowContext(queryCtx, query, args...).Scan(&count)
			cancel()
			match := SQLTextScanMatch{
				Table:   target.Table,
				Column:  target.Column,
				Keyword: keyword,
				Elapsed: time.Since(start),
			}
			if err != nil {
				classified := nbderrors.ClassifySQLErrorMessage(err.Error())
				match.Error = err.Error()
				match.ErrorCode = string(classified.Code)
				match.TimedOut = classified.Code == nbderrors.CodeQueryTimeout
			} else {
				match.Count = count
			}
			matches = append(matches, match)
		}
	}
	return SQLTextScanResult{Matches: matches}, nil
}

// getDB returns the underlying *sql.DB for a datasource.
func (b *SQLDriverBackend) getDB(datasource string) (*sql.DB, error) {
	res, ok := b.manager.Get(datasource)
	if !ok {
		return nil, fmt.Errorf("datasource %q not found in lifecycle manager", datasource)
	}
	sqlRes, ok := res.(*sqlResource)
	if !ok {
		return nil, fmt.Errorf("unexpected resource type for datasource %q", datasource)
	}
	return sqlRes.db, nil
}

// findSQLConnectionAndDatasource looks up the connection config and datasource
// config for a given datasource name.
func findSQLConnectionAndDatasource(cfg config.Config, datasource string) (config.SQLConnectionConfig, config.SQLDatasourceConfig, error) {
	var dsCfg config.SQLDatasourceConfig
	found := false
	for _, ds := range cfg.Datasources.SQL {
		if ds.Name == datasource {
			dsCfg = ds
			found = true
			break
		}
	}
	if !found {
		return config.SQLConnectionConfig{}, config.SQLDatasourceConfig{}, fmt.Errorf("datasource %q not found", datasource)
	}

	for _, conn := range cfg.Connections.SQL {
		if conn.Name == dsCfg.Connection {
			return conn, dsCfg, nil
		}
	}

	return config.SQLConnectionConfig{}, config.SQLDatasourceConfig{}, fmt.Errorf("connection %q not found for datasource %q", dsCfg.Connection, datasource)
}

// buildDSN appends the default database to a base DSN. The base DSN
// typically has the form "user:pass@tcp(host:port)/?params". This function
// inserts the database name before the "?".
func buildDSN(baseDSN, database string) string {
	if database == "" {
		return baseDSN
	}
	idx := strings.Index(baseDSN, "?")
	if idx < 0 {
		if strings.HasSuffix(baseDSN, "/") {
			return baseDSN + database
		}
		return baseDSN + "/" + database
	}
	prefix := baseDSN[:idx]
	suffix := baseDSN[idx:]
	if strings.HasSuffix(prefix, "/") {
		return prefix + database + suffix
	}
	return prefix + "/" + database + suffix
}

// applyLimit wraps a SQL query with a LIMIT clause by wrapping it as a
// subquery with a parameter placeholder. If the original query already
// contains a LIMIT clause, it is returned as-is with hasLimitParam=false.
func applyLimit(sqlStr string, limit int) (string, bool) {
	normalized := trimTrailingSemicolon(sqlStr)
	upper := strings.ToUpper(strings.TrimSpace(normalized))
	if isReadMetadataStatement(upper) {
		return normalized, false
	}
	if strings.Contains(upper, " LIMIT ") {
		return normalized, false
	}
	return fmt.Sprintf("SELECT * FROM (%s) AS ndb_limited LIMIT ?", normalized), true
}

func trimTrailingSemicolon(sqlStr string) string {
	trimmed := strings.TrimSpace(sqlStr)
	for strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	}
	return trimmed
}

func isReadMetadataStatement(upper string) bool {
	return strings.HasPrefix(upper, "SHOW ") ||
		strings.HasPrefix(upper, "DESCRIBE ") ||
		strings.HasPrefix(upper, "DESC ") ||
		strings.HasPrefix(upper, "EXPLAIN ")
}

func normalizeSQLValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return value
	}
	if utf8.Valid(bytes) {
		return string(bytes)
	}
	return map[string]interface{}{
		"encoding": "base64",
		"data":     base64.StdEncoding.EncodeToString(bytes),
	}
}

func buildColumnSearchQuery(req SQLColumnSearchRequest, maxRows int) (string, []interface{}) {
	limit := req.Limit
	if limit <= 0 || limit > maxRows {
		limit = maxRows
	}

	query := "SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_COMMENT FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ?"
	args := []interface{}{req.Schema}
	if req.TablePattern != "" {
		query += " AND TABLE_NAME LIKE ?"
		args = append(args, req.TablePattern)
	}
	if req.ColumnPattern != "" {
		query += " AND COLUMN_NAME LIKE ?"
		args = append(args, req.ColumnPattern)
	}
	if len(req.DataTypes) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?, ", len(req.DataTypes)), ", ")
		query += " AND DATA_TYPE IN (" + placeholders + ")"
		for _, dataType := range req.DataTypes {
			args = append(args, dataType)
		}
	}
	query += " ORDER BY TABLE_NAME, ORDINAL_POSITION LIMIT ?"
	args = append(args, limit)
	return query, args
}

func buildTextColumnPlanQuery(req SQLTextColumnPlanRequest, maxRows int) (string, []interface{}) {
	limit := req.MaxColumns
	if limit <= 0 || limit > maxRows {
		limit = maxRows
	}
	textTypes := []string{"char", "varchar", "tinytext", "text", "mediumtext", "longtext", "json"}
	query := "SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND DATA_TYPE IN (?, ?, ?, ?, ?, ?, ?)"
	args := []interface{}{req.Schema}
	for _, textType := range textTypes {
		args = append(args, textType)
	}
	if req.TablePattern != "" {
		query += " AND TABLE_NAME LIKE ?"
		args = append(args, req.TablePattern)
	}
	if req.ColumnPattern != "" {
		query += " AND COLUMN_NAME LIKE ?"
		args = append(args, req.ColumnPattern)
	}
	query += " ORDER BY TABLE_NAME, ORDINAL_POSITION LIMIT ?"
	args = append(args, limit)
	return query, args
}

func buildTextScanBatches(candidates []SQLTextColumnCandidate, keywords []string, maxTargets int) []SQLTextScanBatch {
	if maxTargets <= 0 {
		maxTargets = 5
	}
	var batches []SQLTextScanBatch
	for start := 0; start < len(candidates); start += maxTargets {
		end := start + maxTargets
		if end > len(candidates) {
			end = len(candidates)
		}
		var targets []SQLTextScanTarget
		for _, candidate := range candidates[start:end] {
			targets = append(targets, SQLTextScanTarget{Table: candidate.Table, Column: candidate.Column})
		}
		batches = append(batches, SQLTextScanBatch{Targets: targets, Keywords: keywords})
	}
	return batches
}

func countDistinctTables(candidates []SQLTextColumnCandidate) int {
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		seen[candidate.Table] = struct{}{}
	}
	return len(seen)
}

func buildTextCountSQL(schema string, target SQLTextScanTarget, keyword string) (string, []interface{}) {
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s` WHERE `%s` LIKE ?",
		escapeIdentifier(schema), escapeIdentifier(target.Table), escapeIdentifier(target.Column))
	return sqlStr, []interface{}{"%" + keyword + "%"}
}

func escapeIdentifier(identifier string) string {
	return strings.ReplaceAll(identifier, "`", "``")
}

func textScanTimeout(requested time.Duration, global time.Duration) time.Duration {
	if requested <= 0 || requested > global {
		return global
	}
	return requested
}
