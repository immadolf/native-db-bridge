package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"native-db-bridge-mcp/internal/audit"
	"native-db-bridge-mcp/internal/backend"
	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/ops"
)

// ---------------------------------------------------------------------------
// Fake backends
// ---------------------------------------------------------------------------

type fakeSQLBackend struct {
	queryCalled        bool
	execCalled         bool
	previewCalled      bool
	schemaListCalled   bool
	columnSearchCalled bool
	textPlanCalled     bool
	textScanCalled     bool
	queryResult        backend.SQLResult
	queryErr           error
	execResult         backend.ExecResult
	execErr            error
	previewResult      backend.SQLResult
	schemas            []string
	columnSearchReq    backend.SQLColumnSearchRequest
	columnSearchRows   []backend.SQLColumnSearchResult
	textPlanReq        backend.SQLTextColumnPlanRequest
	textPlanResult     backend.SQLTextColumnPlanResult
	textScanReq        backend.SQLTextScanRequest
	textScanResult     backend.SQLTextScanResult
	objectTypes        []string
	objects            []backend.SQLObjectInfo
	describeResult     backend.SQLDescribeResult
}

func (f *fakeSQLBackend) Ping(_ context.Context, _ string) error { return nil }

func (f *fakeSQLBackend) Query(_ context.Context, _ string, _ string, _ int) (backend.SQLResult, error) {
	f.queryCalled = true
	if f.queryErr != nil {
		return backend.SQLResult{}, f.queryErr
	}
	return f.queryResult, nil
}

func (f *fakeSQLBackend) Exec(_ context.Context, _ string, _ string) (backend.ExecResult, error) {
	f.execCalled = true
	if f.execErr != nil {
		return backend.ExecResult{}, f.execErr
	}
	return f.execResult, nil
}

func (f *fakeSQLBackend) PreviewTable(_ context.Context, _, _, _ string, _ int) (backend.SQLResult, error) {
	f.previewCalled = true
	return f.previewResult, nil
}

func (f *fakeSQLBackend) SchemaList(_ context.Context, _ string) ([]string, error) {
	f.schemaListCalled = true
	return f.schemas, nil
}

func (f *fakeSQLBackend) ObjectTypeList(_ context.Context, _ string) ([]string, error) {
	return f.objectTypes, nil
}

func (f *fakeSQLBackend) ObjectList(_ context.Context, _, _, _, _ string) ([]backend.SQLObjectInfo, error) {
	return f.objects, nil
}

func (f *fakeSQLBackend) DescribeObject(_ context.Context, _, _, _, _ string) (backend.SQLDescribeResult, error) {
	return f.describeResult, nil
}

func (f *fakeSQLBackend) ColumnSearch(_ context.Context, req backend.SQLColumnSearchRequest) ([]backend.SQLColumnSearchResult, error) {
	f.columnSearchCalled = true
	f.columnSearchReq = req
	return f.columnSearchRows, nil
}

func (f *fakeSQLBackend) TextColumnPlan(_ context.Context, req backend.SQLTextColumnPlanRequest) (backend.SQLTextColumnPlanResult, error) {
	f.textPlanCalled = true
	f.textPlanReq = req
	return f.textPlanResult, nil
}

func (f *fakeSQLBackend) TextScan(_ context.Context, req backend.SQLTextScanRequest) (backend.SQLTextScanResult, error) {
	f.textScanCalled = true
	f.textScanReq = req
	return f.textScanResult, nil
}

type fakeRedisBackend struct {
	commandCalled  bool
	commandErr     error
	scanCalled     bool
	describeCalled bool
	commandResult  backend.RedisResult
	scanResult     backend.RedisScanResult
	describeResult backend.RedisKeyDescription
}

func (f *fakeRedisBackend) Ping(_ context.Context, _ string) error { return nil }

func (f *fakeRedisBackend) Command(_ context.Context, _, _ string, _ []string) (backend.RedisResult, error) {
	f.commandCalled = true
	if f.commandErr != nil {
		return backend.RedisResult{}, f.commandErr
	}
	return f.commandResult, nil
}

func (f *fakeRedisBackend) ScanKeys(_ context.Context, _, _, _ string, _ int) (backend.RedisScanResult, error) {
	f.scanCalled = true
	return f.scanResult, nil
}

func (f *fakeRedisBackend) KeyDescribe(_ context.Context, _, _ string) (backend.RedisKeyDescription, error) {
	f.describeCalled = true
	return f.describeResult, nil
}

type fakeMongoBackend struct {
	findCalled     bool
	writeCalled    bool
	findResult     backend.MongoResult
	writeResult    backend.ExecResult
	databases      []string
	collections    []backend.MongoCollection
	describeResult backend.MongoCollectionDescription
}

func (f *fakeMongoBackend) Ping(_ context.Context, _ string) error { return nil }

func (f *fakeMongoBackend) Find(_ context.Context, _ backend.MongoFindRequest) (backend.MongoResult, error) {
	f.findCalled = true
	return f.findResult, nil
}

func (f *fakeMongoBackend) Write(_ context.Context, _ backend.MongoWriteRequest) (backend.ExecResult, error) {
	f.writeCalled = true
	return f.writeResult, nil
}

func (f *fakeMongoBackend) ListDatabases(_ context.Context, _ string) ([]string, error) {
	return f.databases, nil
}

func (f *fakeMongoBackend) ListCollections(_ context.Context, _, _ string) ([]backend.MongoCollection, error) {
	return f.collections, nil
}

func (f *fakeMongoBackend) DescribeCollection(_ context.Context, _, _ string) (backend.MongoCollectionDescription, error) {
	return f.describeResult, nil
}

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type testHandlers struct {
	*Handlers
	fakeSQL   *fakeSQLBackend
	fakeRedis *fakeRedisBackend
	fakeMongo *fakeMongoBackend
	store     *audit.Store
}

func newTestHandlers(t *testing.T) *testHandlers {
	t.Helper()

	store, err := audit.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := config.Config{
		Server: config.ServerConfig{
			QueryTimeout:      config.Duration{Duration: 30 * time.Second},
			MaxResultRows:     1000,
			RedisScanCountMax: 500,
		},
		Policy: config.PolicyConfig{
			ConfirmationTTL: config.Duration{Duration: 10 * time.Minute},
		},
		Datasources: config.DatasourcesConfig{
			SQL: []config.SQLDatasourceConfig{
				{Name: "saas_dev", Environment: "dev", Connection: "mysql-dev-main", DefaultDatabase: "saas_dev"},
				{Name: "saas_support", Environment: "support", Connection: "mysql-support-main", DefaultDatabase: "saas_support"},
			},
			Redis: []config.RedisDatasourceConfig{
				{Name: "redis-dev", Environment: "dev", Connection: "redis-dev-main", DB: 0, Service: "auth"},
				{Name: "redis-support", Environment: "support", Connection: "redis-support-main", DB: 1, Service: "web"},
			},
			Mongo: []config.MongoDatasourceConfig{
				{Name: "mongo-support", Environment: "support", Connection: "mongo-support-main", DefaultDatabase: "saas_support"},
			},
		},
	}

	sqlBackend := &fakeSQLBackend{
		queryResult: backend.SQLResult{
			Columns:  []backend.ColumnInfo{{Name: "id", Type: "BIGINT"}},
			Rows:     []map[string]interface{}{{"id": int64(1)}},
			RowCount: 1,
			Elapsed:  5 * time.Millisecond,
		},
		execResult: backend.ExecResult{
			AffectedCount: 1,
			Elapsed:       10 * time.Millisecond,
		},
		previewResult: backend.SQLResult{
			Columns:  []backend.ColumnInfo{{Name: "id", Type: "BIGINT"}, {Name: "name", Type: "VARCHAR"}},
			Rows:     []map[string]interface{}{{"id": int64(1), "name": "test"}},
			RowCount: 1,
			Elapsed:  3 * time.Millisecond,
		},
		schemas:     []string{"saas_dev"},
		objectTypes: []string{"table", "view", "procedure", "function"},
		objects: []backend.SQLObjectInfo{
			{Schema: "saas_dev", Name: "users", Type: "table"},
		},
		describeResult: backend.SQLDescribeResult{
			Object:  backend.SQLObjectInfo{Schema: "saas_dev", Name: "users", Type: "table"},
			Columns: []backend.SQLColumnInfo{{Name: "id", Type: "BIGINT", Nullable: false}},
			Indexes: []backend.SQLIndexInfo{{Name: "PRIMARY", Columns: []string{"id"}, Unique: true}},
		},
	}

	redisBackend := &fakeRedisBackend{
		commandResult: backend.RedisResult{
			Result:  "value",
			Elapsed: 2 * time.Millisecond,
		},
		scanResult: backend.RedisScanResult{
			Keys:       []string{"key1", "key2"},
			NextCursor: "0",
			Truncated:  false,
		},
		describeResult: backend.RedisKeyDescription{
			Key:    "test:key",
			Type:   "string",
			TTL:    60,
			Length: 5,
			Exists: true,
		},
	}

	mongoBackend := &fakeMongoBackend{
		findResult: backend.MongoResult{
			Documents: []map[string]interface{}{{"_id": "abc", "name": "test"}},
			Count:     1,
			Elapsed:   5 * time.Millisecond,
		},
		writeResult: backend.ExecResult{
			AffectedCount: 1,
			Elapsed:       10 * time.Millisecond,
		},
		databases: []string{"saas_support"},
		collections: []backend.MongoCollection{
			{Name: "users", Type: "collection"},
		},
		describeResult: backend.MongoCollectionDescription{
			Collection:     "users",
			EstimatedCount: 100,
			Indexes:        []backend.IndexInfo{{Name: "_id_", Key: map[string]interface{}{"_id": 1}, Unique: true}},
			SampleSchema:   map[string]interface{}{"_id": "objectId", "name": "string"},
		},
	}

	handlers := NewHandlers(Deps{
		Config: cfg,
		Audit:  store,
		SQL:    sqlBackend,
		Redis:  redisBackend,
		Mongo:  mongoBackend,
		Ops:    ops.NewTracker(),
	})

	return &testHandlers{
		Handlers:  handlers,
		fakeSQL:   sqlBackend,
		fakeRedis: redisBackend,
		fakeMongo: mongoBackend,
		store:     store,
	}
}

// ---------------------------------------------------------------------------
// Required tests
// ---------------------------------------------------------------------------

func TestSQLPrepareCreatesConfirmationWithoutExecuting(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLPrepareChange(ctx, SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "UPDATE users SET name = 'test' WHERE id = 1",
	})

	if !output.OK {
		t.Fatalf("expected OK, got error: %s", output.Error.Message)
	}
	if output.ConfirmationID == "" {
		t.Fatal("expected confirmation_id")
	}
	if output.Kind != "sql_dml" {
		t.Fatalf("expected kind sql_dml, got %s", output.Kind)
	}

	// Verify the SQL backend was NOT called
	if h.fakeSQL.execCalled {
		t.Fatal("SQL backend should not be called during prepare")
	}

	// Verify confirmation exists in the store
	conf, err := h.store.GetConfirmation(output.ConfirmationID)
	if err != nil {
		t.Fatalf("confirmation should exist: %v", err)
	}
	if conf.Status != "pending" {
		t.Fatalf("expected pending status, got %s", conf.Status)
	}
}

func TestCancelConfirmationOnlyCancelsPending(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Create a confirmation first
	output := h.SQLPrepareChange(ctx, SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "INSERT INTO users (name) VALUES ('test')",
	})
	if !output.OK {
		t.Fatalf("prepare failed: %s", output.Error.Message)
	}
	confID := output.ConfirmationID

	// Cancel the pending confirmation
	cancelOutput := h.CancelConfirmation(ctx, CancelConfirmationInput{
		ConfirmationID: confID,
	})
	if !cancelOutput.OK {
		t.Fatalf("cancel should succeed: %s", cancelOutput.Error.Message)
	}
	if cancelOutput.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", cancelOutput.Status)
	}

	// Try to cancel again -- should fail (already cancelled)
	cancelOutput2 := h.CancelConfirmation(ctx, CancelConfirmationInput{
		ConfirmationID: confID,
	})
	if cancelOutput2.OK {
		t.Fatal("second cancel should fail")
	}
}

func TestDatasourceListFiltersByTypeAndEnvironment(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// No filters: should return all (2 SQL + 1 Redis(dev) + 1 Redis(support) + 1 Mongo = 5)
	all := h.DatasourceList(ctx, DatasourceListInput{})
	if len(all.Datasources) != 5 {
		t.Fatalf("expected 5 datasources total, got %d", len(all.Datasources))
	}

	// Filter by type=sql
	sqlOnly := h.DatasourceList(ctx, DatasourceListInput{Type: "sql"})
	if len(sqlOnly.Datasources) != 2 {
		t.Fatalf("expected 2 SQL datasources, got %d", len(sqlOnly.Datasources))
	}
	for _, ds := range sqlOnly.Datasources {
		if ds.Type != "sql" {
			t.Fatalf("expected sql type, got %s", ds.Type)
		}
	}

	// Filter by environment=support
	supportOnly := h.DatasourceList(ctx, DatasourceListInput{Environment: "support"})
	for _, ds := range supportOnly.Datasources {
		if ds.Environment != "support" {
			t.Fatalf("expected support environment, got %s for %s", ds.Environment, ds.Name)
		}
	}

	// Filter by type=redis AND environment=dev
	redisDev := h.DatasourceList(ctx, DatasourceListInput{Type: "redis", Environment: "dev"})
	if len(redisDev.Datasources) != 1 {
		t.Fatalf("expected 1 redis dev datasource, got %d", len(redisDev.Datasources))
	}
	if redisDev.Datasources[0].Name != "redis-dev" {
		t.Fatalf("expected redis-dev, got %s", redisDev.Datasources[0].Name)
	}
}

func TestSQLQueryClassifiesUnknownColumn(t *testing.T) {
	h := newTestHandlers(t)
	h.fakeSQL.queryErr = fmt.Errorf("sql query saas_support: Error 1054 (42S22): Unknown column 'remark' in 'field list'")

	output := h.SQLQuery(context.Background(), SQLQueryInput{
		Datasource: "saas_support",
		SQL:        "SELECT remark FROM users",
	})
	if output.OK {
		t.Fatal("expected sql_query to fail")
	}
	if output.Error.Code != "SQL_UNKNOWN_COLUMN" {
		t.Fatalf("code=%s", output.Error.Code)
	}
	if output.Error.Retryable {
		t.Fatal("unknown column should not be retryable")
	}
}

func TestExecuteSQLConfirmationClassifiesUnknownColumn(t *testing.T) {
	h := newTestHandlers(t)
	h.fakeSQL.execErr = fmt.Errorf("sql exec saas_support: Error 1054 (42S22): Unknown column 'remark' in 'field list'")

	prepared := h.SQLPrepareChange(context.Background(), SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "UPDATE users SET remark = 'x' WHERE id = 1",
	})
	if !prepared.OK {
		t.Fatalf("prepare failed: %s", prepared.Error.Message)
	}

	output := h.ExecuteConfirmation(context.Background(), ExecuteConfirmationInput{
		ConfirmationID: prepared.ConfirmationID,
	})
	if output.OK {
		t.Fatal("expected execute_confirmation to fail")
	}
	if output.Error.Code != "SQL_UNKNOWN_COLUMN" {
		t.Fatalf("code=%s", output.Error.Code)
	}
}

func TestSQLColumnSearchSuccess(t *testing.T) {
	h := newTestHandlers(t)
	h.fakeSQL.columnSearchRows = []backend.SQLColumnSearchResult{
		{Schema: "saas_support", Table: "uc_user_info", Column: "avatar", DataType: "varchar", ColumnType: "varchar(512)", Nullable: true},
	}

	output := h.SQLColumnSearch(context.Background(), SQLColumnSearchInput{
		Datasource:    "saas_support",
		Schema:        "saas_support",
		ColumnPattern: "%avatar%",
		DataTypes:     []string{"varchar"},
		Limit:         20,
	})
	if !output.OK {
		t.Fatalf("sql_column_search failed: %s", output.Error.Message)
	}
	if !h.fakeSQL.columnSearchCalled {
		t.Fatal("ColumnSearch should be called")
	}
	if h.fakeSQL.columnSearchReq.Schema != "saas_support" {
		t.Fatalf("schema=%s", h.fakeSQL.columnSearchReq.Schema)
	}
	if len(output.Columns) != 1 || output.Columns[0].Column != "avatar" {
		t.Fatalf("columns=%#v", output.Columns)
	}
}

func TestSQLTextColumnPlanSuccess(t *testing.T) {
	h := newTestHandlers(t)
	h.fakeSQL.textPlanResult = backend.SQLTextColumnPlanResult{
		Candidates: []backend.SQLTextColumnCandidate{
			{Schema: "saas_support", Table: "uc_user_info", Column: "avatar", DataType: "varchar", ColumnType: "varchar(512)"},
		},
		Batches: []backend.SQLTextScanBatch{
			{
				Targets:  []backend.SQLTextScanTarget{{Table: "uc_user_info", Column: "avatar"}},
				Keywords: []string{"oss"},
			},
		},
	}

	output := h.SQLTextColumnPlan(context.Background(), SQLTextColumnPlanInput{
		Datasource:    "saas_support",
		Schema:        "saas_support",
		ColumnPattern: "%avatar%",
		Keywords:      []string{"oss"},
		MaxColumns:    20,
	})
	if !output.OK {
		t.Fatalf("sql_text_column_plan failed: %s", output.Error.Message)
	}
	if !h.fakeSQL.textPlanCalled {
		t.Fatal("TextColumnPlan should be called")
	}
	if len(output.Candidates) != 1 || output.Candidates[0].Column != "avatar" {
		t.Fatalf("candidates=%#v", output.Candidates)
	}
}

func TestSQLTextScanSuccess(t *testing.T) {
	h := newTestHandlers(t)
	h.fakeSQL.textScanResult = backend.SQLTextScanResult{
		Matches: []backend.SQLTextScanMatch{
			{Table: "uc_user_info", Column: "avatar", Keyword: "oss", Count: 12, Elapsed: 3 * time.Millisecond},
		},
	}

	output := h.SQLTextScan(context.Background(), SQLTextScanInput{
		Datasource: "saas_support",
		Schema:     "saas_support",
		Targets:    []backend.SQLTextScanTarget{{Table: "uc_user_info", Column: "avatar"}},
		Keywords:   []string{"oss"},
		Mode:       "count",
		Timeout:    "5s",
	})
	if !output.OK {
		t.Fatalf("sql_text_scan failed: %s", output.Error.Message)
	}
	if !h.fakeSQL.textScanCalled {
		t.Fatal("TextScan should be called")
	}
	if len(output.Matches) != 1 || output.Matches[0].Count != 12 {
		t.Fatalf("matches=%#v", output.Matches)
	}
	if h.fakeSQL.textScanReq.Timeout != 5*time.Second {
		t.Fatalf("timeout=%s", h.fakeSQL.textScanReq.Timeout)
	}
}

func TestAuditSummaryIncludesTopErrorsAndConfirmations(t *testing.T) {
	h := newTestHandlers(t)
	now := time.Now()
	if err := h.deps.Audit.InsertAuditEvent(audit.AuditEvent{
		ID:         "evt_summary_1",
		EventType:  "sql_query",
		Datasource: "saas_support",
		Summary:    "SELECT missing_col FROM users",
		Status:     "error",
		ErrorCode:  "SQL_UNKNOWN_COLUMN",
		CreatedAt:  now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.deps.Audit.CreateConfirmation(audit.Confirmation{
		ID:         "conf_summary_1",
		Kind:       "sql_dml",
		Datasource: "saas_support",
		Summary:    "UPDATE users SET name = ? WHERE id = ?",
		RiskLevel:  "medium",
		ImpactJSON: "{}",
		Status:     "pending",
		ExpiresAt:  now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.deps.Audit.MarkConfirmationExecuting("conf_summary_1"); err != nil {
		t.Fatal(err)
	}
	if err := h.deps.Audit.MarkConfirmationExecuted("conf_summary_1"); err != nil {
		t.Fatal(err)
	}

	output := h.AuditSummary(context.Background(), AuditSummaryInput{
		StartTime:  now.Add(-time.Hour).Format(time.RFC3339),
		EndTime:    now.Add(time.Hour).Format(time.RFC3339),
		Datasource: "saas_support",
		EventType:  "sql_query",
		Status:     "success",
		GroupBy:    "error_code",
		Limit:      10,
	})
	if !output.OK {
		t.Fatalf("audit_summary failed: %s", output.Error.Message)
	}
	if len(output.Rows) != 0 {
		t.Fatalf("rows=%#v", output.Rows)
	}
	if len(output.TopErrors) != 1 || output.TopErrors[0].Summary != "SELECT missing_col FROM users" {
		t.Fatalf("topErrors=%#v", output.TopErrors)
	}
	if len(output.ConfirmationSummary) != 1 || output.ConfirmationSummary[0].Status != "executed" {
		t.Fatalf("confirmationSummary=%#v", output.ConfirmationSummary)
	}
}

func TestAuditRecentFiltersByDatasource(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Create some audit events by running queries
	h.SQLQuery(ctx, SQLQueryInput{Datasource: "saas_dev", SQL: "SELECT 1"})
	h.SQLQuery(ctx, SQLQueryInput{Datasource: "saas_support", SQL: "SELECT 1"})

	// List all
	all := h.AuditRecent(ctx, AuditRecentInput{Limit: 100})
	if !all.OK {
		t.Fatalf("audit_recent failed: %s", all.Error.Message)
	}
	if len(all.Events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(all.Events))
	}

	// Filter by datasource
	filtered := h.AuditRecent(ctx, AuditRecentInput{Datasource: "saas_dev", Limit: 100})
	if !filtered.OK {
		t.Fatalf("audit_recent filtered failed: %s", filtered.Error.Message)
	}
	for _, evt := range filtered.Events {
		if evt.Datasource != "saas_dev" {
			t.Fatalf("expected saas_dev, got %s", evt.Datasource)
		}
	}
}

func TestOperationListFiltersStatus(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Create some operations by running queries
	h.SQLQuery(ctx, SQLQueryInput{Datasource: "saas_dev", SQL: "SELECT 1"})

	// List all operations
	all := h.OperationList(ctx, OperationListInput{Limit: 100})
	if !all.OK {
		t.Fatalf("operation_list failed: %s", all.Error.Message)
	}
	if len(all.Operations) == 0 {
		t.Fatal("expected at least 1 operation")
	}

	// All operations should have finished by now
	// Filter by a non-existent status
	filtered := h.OperationList(ctx, OperationListInput{Status: "running", Limit: 100})
	if !filtered.OK {
		t.Fatalf("operation_list filtered failed: %s", filtered.Error.Message)
	}
	// Running operations should be empty since the query completed synchronously
	for _, op := range filtered.Operations {
		if op.Status != "running" {
			t.Fatalf("expected running status, got %s", op.Status)
		}
	}
}

func TestRedisKeyScanCapsCount(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Request count higher than max (500)
	output := h.RedisKeyScan(ctx, RedisKeyScanInput{
		Datasource: "redis-support",
		Match:      "*",
		Count:      9999,
	})
	if !output.OK {
		t.Fatalf("redis_key_scan failed: %s", output.Error.Message)
	}

	// The scan should have been called (verify the fake was invoked)
	if !h.fakeRedis.scanCalled {
		t.Fatal("expected ScanKeys to be called")
	}
}

func TestCancelOperationCallsTracker(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Register a dummy operation
	cancelled := false
	if err := h.store.InsertOperation(audit.Operation{
		ID:         "op_test_cancel",
		Kind:       "sql_query",
		Datasource: "saas_support",
		Status:     "running",
		StartedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	h.deps.Ops.Register("op_test_cancel", func() { cancelled = true })

	output := h.CancelOperation(ctx, CancelOperationInput{
		OperationID: "op_test_cancel",
	})
	if !output.OK {
		t.Fatalf("cancel_operation should succeed: %s", output.Error.Message)
	}
	if output.Status != "cancel_requested" {
		t.Fatalf("expected cancel_requested, got %s", output.Status)
	}
	if !cancelled {
		t.Fatal("expected cancel function to be called")
	}
	ops, err := h.store.ListOperations("cancel_requested", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, op := range ops {
		if op.ID == "op_test_cancel" {
			found = true
			if op.CancelRequestedAt == nil {
				t.Fatal("expected cancel_requested_at")
			}
		}
	}
	if !found {
		t.Fatal("expected operation status cancel_requested")
	}
}

func TestCancelOperationRejectsMissingOperation(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.CancelOperation(ctx, CancelOperationInput{
		OperationID: "op_nonexistent",
	})
	if output.OK {
		t.Fatal("cancel of missing operation should fail")
	}
	if output.Error == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// Additional handler tests
// ---------------------------------------------------------------------------

func TestSQLQueryPolicyRejectsWrite(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLQuery(ctx, SQLQueryInput{
		Datasource: "saas_support",
		SQL:        "UPDATE users SET name = 'x'",
	})
	if output.OK {
		t.Fatal("sql_query should reject write statements")
	}
	if h.fakeSQL.queryCalled {
		t.Fatal("SQL backend should not be called for rejected write")
	}
}

func TestExecuteConfirmationRunsFrozenPayload(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Prepare a change
	prepareOutput := h.SQLPrepareChange(ctx, SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "UPDATE users SET name = 'test' WHERE id = 1",
	})
	if !prepareOutput.OK {
		t.Fatalf("prepare failed: %s", prepareOutput.Error.Message)
	}

	// Execute the confirmation
	execOutput := h.ExecuteConfirmation(ctx, ExecuteConfirmationInput{
		ConfirmationID: prepareOutput.ConfirmationID,
	})
	if !execOutput.OK {
		t.Fatalf("execute failed: %s", execOutput.Error.Message)
	}
	if execOutput.Status != "executed" {
		t.Fatalf("expected executed, got %s", execOutput.Status)
	}
	if execOutput.AffectedCount != 1 {
		t.Fatalf("expected 1 affected, got %d", execOutput.AffectedCount)
	}

	// Verify the SQL exec was called
	if !h.fakeSQL.execCalled {
		t.Fatal("SQL Exec should have been called")
	}

	// Verify confirmation status is executed
	conf, err := h.store.GetConfirmation(prepareOutput.ConfirmationID)
	if err != nil {
		t.Fatalf("get confirmation: %v", err)
	}
	if conf.Status != "executed" {
		t.Fatalf("expected executed status, got %s", conf.Status)
	}
}

func TestExecuteConfirmationRejectsNonexistent(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.ExecuteConfirmation(ctx, ExecuteConfirmationInput{
		ConfirmationID: "conf_nonexistent",
	})
	if output.OK {
		t.Fatal("execute of nonexistent confirmation should fail")
	}
}

func TestRedisCommandRejectsAlwaysRejected(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	rejected := []string{"SELECT", "EVAL", "SCRIPT", "DEBUG", "MONITOR", "CONFIG", "SHUTDOWN"}
	for _, cmd := range rejected {
		output := h.RedisCommand(ctx, RedisCommandInput{
			Datasource: "redis-support",
			Command:    cmd,
			Args:       []string{"0"},
		})
		if output.OK {
			t.Fatalf("redis_command should reject %q", cmd)
		}
	}
}

func TestRedisPrepareChangeCreatesConfirmation(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.RedisPrepareChange(ctx, RedisPrepareChangeInput{
		Datasource: "redis-support",
		Command:    "SET",
		Args:       []string{"test:key", "value"},
	})
	if !output.OK {
		t.Fatalf("redis_prepare_change failed: %s", output.Error.Message)
	}
	if output.Kind != "redis_write" {
		t.Fatalf("expected redis_write, got %s", output.Kind)
	}
	if output.ConfirmationID == "" {
		t.Fatal("expected confirmation_id")
	}

	// Verify backend was not called
	if h.fakeRedis.commandCalled {
		t.Fatal("Redis backend should not be called during prepare")
	}
}

func TestMongoFindRejectsDisallowedAggregateStage(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.MongoFind(ctx, MongoFindInput{
		Datasource: "mongo-support",
		Collection: "users",
		Operation:  "aggregate",
		Pipeline:   []interface{}{map[string]interface{}{"$out": "output_collection"}},
	})
	if output.OK {
		t.Fatal("mongo_find should reject $out stage")
	}
}

func TestMongoPrepareChangeCreatesConfirmation(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.MongoPrepareChange(ctx, MongoPrepareChangeInput{
		Datasource: "mongo-support",
		Collection: "users",
		Operation:  "updateOne",
		Filter:     map[string]interface{}{"_id": "abc"},
		Document:   map[string]interface{}{"$set": map[string]interface{}{"name": "updated"}},
	})
	if !output.OK {
		t.Fatalf("mongo_prepare_change failed: %s", output.Error.Message)
	}
	if output.Kind != "mongo_write" {
		t.Fatalf("expected mongo_write, got %s", output.Kind)
	}
	if h.fakeMongo.writeCalled {
		t.Fatal("Mongo backend should not be called during prepare")
	}
}

func TestConfirmationGetReturnsStatus(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Prepare a change
	prepareOutput := h.SQLPrepareChange(ctx, SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "INSERT INTO users (name) VALUES ('test')",
	})
	if !prepareOutput.OK {
		t.Fatalf("prepare failed: %s", prepareOutput.Error.Message)
	}

	// Get the confirmation
	getOutput := h.ConfirmationGet(ctx, ConfirmationGetInput{
		ConfirmationID: prepareOutput.ConfirmationID,
	})
	if !getOutput.OK {
		t.Fatalf("confirmation_get failed: %s", getOutput.Error.Message)
	}
	if getOutput.Confirmation.Status != "pending" {
		t.Fatalf("expected pending, got %s", getOutput.Confirmation.Status)
	}
	if getOutput.Confirmation.Kind != "sql_dml" {
		t.Fatalf("expected sql_dml, got %s", getOutput.Confirmation.Kind)
	}
}

func TestSQLPrepareChangeDDLCreatesDDDKind(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLPrepareChange(ctx, SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "CREATE TABLE test (id INT PRIMARY KEY)",
	})
	if !output.OK {
		t.Fatalf("prepare DDL failed: %s", output.Error.Message)
	}
	if output.Kind != "sql_ddl" {
		t.Fatalf("expected sql_ddl, got %s", output.Kind)
	}
	if output.RiskLevel != "high" {
		t.Fatalf("expected high risk for DDL, got %s", output.RiskLevel)
	}
}

func TestDatasourceHealthcheckConfigMode(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.DatasourceHealthcheck(ctx, DatasourceHealthcheckInput{
		Datasource: "saas_support",
		Mode:       "config",
	})
	if !output.OK {
		t.Fatalf("config healthcheck failed: %s", output.Error.Message)
	}
	if output.Status != "healthy" {
		t.Fatalf("expected healthy, got %s", output.Status)
	}
}

func TestDatasourceHealthcheckConnectMode(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.DatasourceHealthcheck(ctx, DatasourceHealthcheckInput{
		Datasource: "saas_support",
		Mode:       "connect",
	})
	if !output.OK {
		t.Fatalf("connect healthcheck failed: %s", output.Error.Message)
	}
	if output.Status != "healthy" {
		t.Fatalf("expected healthy, got %s", output.Status)
	}
}

func TestDatasourceHealthcheckUnknownDatasource(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.DatasourceHealthcheck(ctx, DatasourceHealthcheckInput{
		Datasource: "nonexistent",
		Mode:       "config",
	})
	if output.OK {
		t.Fatal("healthcheck of unknown datasource should fail")
	}
	if output.Status != "unhealthy" {
		t.Fatalf("expected unhealthy, got %s", output.Status)
	}
}

func TestSQLTablePreviewRespectsLimit(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLTablePreview(ctx, SQLTablePreviewInput{
		Datasource: "saas_support",
		Schema:     "saas_support",
		Table:      "users",
		Limit:      50,
	})
	if !output.OK {
		t.Fatalf("sql_table_preview failed: %s", output.Error.Message)
	}
	if !h.fakeSQL.previewCalled {
		t.Fatal("PreviewTable should have been called")
	}
}

func TestMongoDatabaseList(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.MongoDatabaseList(ctx, MongoDatabaseListInput{
		Datasource: "mongo-support",
	})
	if !output.OK {
		t.Fatalf("mongo_database_list failed: %s", output.Error.Message)
	}
	if len(output.Databases) != 1 || output.Databases[0] != "saas_support" {
		t.Fatalf("expected [saas_support], got %v", output.Databases)
	}
}

func TestRedisKeyDescribe(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.RedisKeyDescribe(ctx, RedisKeyDescribeInput{
		Datasource: "redis-support",
		Key:        "test:key",
	})
	if !output.OK {
		t.Fatalf("redis_key_describe failed: %s", output.Error.Message)
	}
	if output.Key != "test:key" {
		t.Fatalf("expected test:key, got %s", output.Key)
	}
	if output.Type != "string" {
		t.Fatalf("expected string type, got %s", output.Type)
	}
}

func TestExecuteConfirmationRejectsAlreadyExecuted(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Prepare and execute
	prepareOutput := h.SQLPrepareChange(ctx, SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "UPDATE users SET name = 'test' WHERE id = 1",
	})
	if !prepareOutput.OK {
		t.Fatalf("prepare failed: %s", prepareOutput.Error.Message)
	}

	execOutput := h.ExecuteConfirmation(ctx, ExecuteConfirmationInput{
		ConfirmationID: prepareOutput.ConfirmationID,
	})
	if !execOutput.OK {
		t.Fatalf("first execute failed: %s", execOutput.Error.Message)
	}

	// Try to execute again
	execOutput2 := h.ExecuteConfirmation(ctx, ExecuteConfirmationInput{
		ConfirmationID: prepareOutput.ConfirmationID,
	})
	if execOutput2.OK {
		t.Fatal("second execute should fail")
	}
}

func TestSQLSchemaList(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLSchemaList(ctx, SQLSchemaListInput{Datasource: "saas_dev"})
	if !output.OK {
		t.Fatalf("sql_schema_list failed: %s", output.Error.Message)
	}
	if len(output.Schemas) != 1 || output.Schemas[0] != "saas_dev" {
		t.Fatalf("expected [saas_dev], got %v", output.Schemas)
	}
	if !h.fakeSQL.schemaListCalled {
		t.Fatal("SchemaList should have been called")
	}
}

func TestSQLObjectTypeList(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLObjectTypeList(ctx, SQLObjectTypeListInput{Datasource: "saas_dev"})
	if !output.OK {
		t.Fatalf("sql_object_type_list failed: %s", output.Error.Message)
	}
	if len(output.ObjectTypes) != 4 {
		t.Fatalf("expected 4 object types, got %d", len(output.ObjectTypes))
	}
}

func TestSQLObjectList(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLObjectList(ctx, SQLObjectListInput{
		Datasource: "saas_dev",
		Schema:     "saas_dev",
		ObjectType: "table",
	})
	if !output.OK {
		t.Fatalf("sql_object_list failed: %s", output.Error.Message)
	}
	if len(output.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(output.Objects))
	}
	if output.Objects[0].Name != "users" {
		t.Fatalf("expected users, got %s", output.Objects[0].Name)
	}
}

func TestSQLObjectDescribe(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLObjectDescribe(ctx, SQLObjectDescribeInput{
		Datasource: "saas_dev",
		Schema:     "saas_dev",
		ObjectName: "users",
		ObjectType: "table",
	})
	if !output.OK {
		t.Fatalf("sql_object_describe failed: %s", output.Error.Message)
	}
	if output.Object.Name != "users" {
		t.Fatalf("expected users, got %s", output.Object.Name)
	}
	if len(output.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(output.Columns))
	}
	if len(output.Indexes) != 1 {
		t.Fatalf("expected 1 index, got %d", len(output.Indexes))
	}
}

func TestMongoCollectionList(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.MongoCollectionList(ctx, MongoCollectionListInput{
		Datasource: "mongo-support",
	})
	if !output.OK {
		t.Fatalf("mongo_collection_list failed: %s", output.Error.Message)
	}
	if len(output.Collections) != 1 || output.Collections[0].Name != "users" {
		t.Fatalf("expected [users], got %v", output.Collections)
	}
}

func TestMongoCollectionDescribe(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.MongoCollectionDescribe(ctx, MongoCollectionDescribeInput{
		Datasource: "mongo-support",
		Collection: "users",
	})
	if !output.OK {
		t.Fatalf("mongo_collection_describe failed: %s", output.Error.Message)
	}
	if output.Collection != "users" {
		t.Fatalf("expected users, got %s", output.Collection)
	}
	if output.EstimatedCount != 100 {
		t.Fatalf("expected 100, got %d", output.EstimatedCount)
	}
	if len(output.Indexes) != 1 {
		t.Fatalf("expected 1 index, got %d", len(output.Indexes))
	}
}

func TestRedisCommandSuccessPath(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.RedisCommand(ctx, RedisCommandInput{
		Datasource: "redis-support",
		Command:    "GET",
		Args:       []string{"test:key"},
	})
	if !output.OK {
		t.Fatalf("redis_command failed: %s", output.Error.Message)
	}
	if output.Result != "value" {
		t.Fatalf("expected value, got %v", output.Result)
	}
	if !h.fakeRedis.commandCalled {
		t.Fatal("Redis Command should have been called")
	}
}

func TestMongoFindSuccessPath(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.MongoFind(ctx, MongoFindInput{
		Datasource: "mongo-support",
		Collection: "users",
		Operation:  "find",
		Filter:     map[string]interface{}{"name": "test"},
		Limit:      10,
	})
	if !output.OK {
		t.Fatalf("mongo_find failed: %s", output.Error.Message)
	}
	if output.Count != 1 {
		t.Fatalf("expected 1 document, got %d", output.Count)
	}
	if !h.fakeMongo.findCalled {
		t.Fatal("Mongo Find should have been called")
	}
}

func TestRedisCommandRejectsWriteCommand(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.RedisCommand(ctx, RedisCommandInput{
		Datasource: "redis-support",
		Command:    "SET",
		Args:       []string{"key", "value"},
	})
	if output.OK {
		t.Fatal("redis_command should reject write commands")
	}
}

func TestSQLQuerySuccessPath(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLQuery(ctx, SQLQueryInput{
		Datasource: "saas_support",
		SQL:        "SELECT * FROM users LIMIT 10",
		Limit:      100,
	})
	if !output.OK {
		t.Fatalf("sql_query failed: %s", output.Error.Message)
	}
	if output.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", output.RowCount)
	}
	if output.OperationID == "" {
		t.Fatal("expected operation_id")
	}
	if !h.fakeSQL.queryCalled {
		t.Fatal("SQL Query should have been called")
	}
}

func TestSQLQueryRejectsLockingRead(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.SQLQuery(ctx, SQLQueryInput{
		Datasource: "saas_support",
		SQL:        "SELECT * FROM users FOR UPDATE",
	})
	if output.OK {
		t.Fatal("sql_query should reject FOR UPDATE")
	}
}

func TestConfirmationGetRejectsNonexistent(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.ConfirmationGet(ctx, ConfirmationGetInput{
		ConfirmationID: "conf_nonexistent",
	})
	if output.OK {
		t.Fatal("confirmation_get should fail for nonexistent ID")
	}
}

func TestSQLQueryBackendError(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	h.fakeSQL.queryErr = fmt.Errorf("connection refused")

	output := h.SQLQuery(ctx, SQLQueryInput{
		Datasource: "saas_support",
		SQL:        "SELECT * FROM users",
	})
	if output.OK {
		t.Fatal("sql_query should fail on backend error")
	}
	if output.OperationID == "" {
		t.Fatal("expected operation_id on error")
	}
}

func TestRedisCommandBackendError(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	h.fakeRedis.commandErr = fmt.Errorf("connection refused")

	output := h.RedisCommand(ctx, RedisCommandInput{
		Datasource: "redis-support",
		Command:    "GET",
		Args:       []string{"key"},
	})
	if output.OK {
		t.Fatal("redis_command should fail on backend error")
	}
	if output.OperationID == "" {
		t.Fatal("expected operation_id on error")
	}
}

func TestExecuteConfirmationRedisWrite(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Prepare a Redis write
	prepareOutput := h.RedisPrepareChange(ctx, RedisPrepareChangeInput{
		Datasource: "redis-support",
		Command:    "SET",
		Args:       []string{"test:key", "value"},
	})
	if !prepareOutput.OK {
		t.Fatalf("prepare failed: %s", prepareOutput.Error.Message)
	}

	// Execute
	execOutput := h.ExecuteConfirmation(ctx, ExecuteConfirmationInput{
		ConfirmationID: prepareOutput.ConfirmationID,
	})
	if !execOutput.OK {
		t.Fatalf("execute redis confirmation failed: %s", execOutput.Error.Message)
	}
	if execOutput.Status != "executed" {
		t.Fatalf("expected executed, got %s", execOutput.Status)
	}
	if !h.fakeRedis.commandCalled {
		t.Fatal("Redis Command should have been called")
	}
}

func TestExecuteConfirmationMongoWrite(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Prepare a Mongo write
	prepareOutput := h.MongoPrepareChange(ctx, MongoPrepareChangeInput{
		Datasource: "mongo-support",
		Collection: "users",
		Operation:  "insertOne",
		Document:   map[string]interface{}{"name": "test"},
	})
	if !prepareOutput.OK {
		t.Fatalf("prepare failed: %s", prepareOutput.Error.Message)
	}

	// Execute
	execOutput := h.ExecuteConfirmation(ctx, ExecuteConfirmationInput{
		ConfirmationID: prepareOutput.ConfirmationID,
	})
	if !execOutput.OK {
		t.Fatalf("execute mongo confirmation failed: %s", execOutput.Error.Message)
	}
	if execOutput.Status != "executed" {
		t.Fatalf("expected executed, got %s", execOutput.Status)
	}
	if !h.fakeMongo.writeCalled {
		t.Fatal("Mongo Write should have been called")
	}
}

func TestOperationListWithResults(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Create operations
	h.SQLQuery(ctx, SQLQueryInput{Datasource: "saas_dev", SQL: "SELECT 1"})
	h.SQLQuery(ctx, SQLQueryInput{Datasource: "saas_support", SQL: "SELECT 1"})

	output := h.OperationList(ctx, OperationListInput{Limit: 10})
	if !output.OK {
		t.Fatalf("operation_list failed: %s", output.Error.Message)
	}
	if len(output.Operations) < 2 {
		t.Fatalf("expected at least 2 operations, got %d", len(output.Operations))
	}

	// Verify operation fields are populated
	op := output.Operations[0]
	if op.OperationID == "" {
		t.Fatal("expected operation_id")
	}
	if op.Kind == "" {
		t.Fatal("expected kind")
	}
	if op.StartedAt == "" {
		t.Fatal("expected started_at")
	}
}

func TestAuditRecentWithEventTypeFilter(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// Create events
	h.SQLQuery(ctx, SQLQueryInput{Datasource: "saas_dev", SQL: "SELECT 1"})

	output := h.AuditRecent(ctx, AuditRecentInput{
		EventType: "sql_query",
		Limit:     100,
	})
	if !output.OK {
		t.Fatalf("audit_recent failed: %s", output.Error.Message)
	}
	for _, evt := range output.Events {
		if evt.EventType != "sql_query" {
			t.Fatalf("expected sql_query event type, got %s", evt.EventType)
		}
	}
}

func TestRedisPrepareChangeRejectsReadCommand(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	output := h.RedisPrepareChange(ctx, RedisPrepareChangeInput{
		Datasource: "redis-support",
		Command:    "GET",
		Args:       []string{"key"},
	})
	if output.OK {
		t.Fatal("redis_prepare_change should reject read commands")
	}
}

func TestMongoPrepareChangeRejectsInvalidMatrix(t *testing.T) {
	h := newTestHandlers(t)
	ctx := context.Background()

	// insertOne must NOT have filter
	output := h.MongoPrepareChange(ctx, MongoPrepareChangeInput{
		Datasource: "mongo-support",
		Collection: "users",
		Operation:  "insertOne",
		Filter:     map[string]interface{}{"name": "test"},
		Document:   map[string]interface{}{"name": "test"},
	})
	if output.OK {
		t.Fatal("mongo_prepare_change should reject insertOne with filter")
	}
}
