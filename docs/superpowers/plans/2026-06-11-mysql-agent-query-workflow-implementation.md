# MySQL Agent Query Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build MySQL-focused agent query workflow improvements for native-db-bridge so agents can discover schema, scan text columns safely, get readable SQL results, and receive actionable SQL errors.

**Architecture:** Keep the existing `tools -> backend -> policy/audit -> driver` layering. Add MySQL metadata and text-scan methods to `backend`, expose them through typed MCP tool schemas and handlers in `tools`, wire them in `server`, and refine SQL execution/error behavior without changing Redis, MongoDB, production policy, or launchd setup.

**Tech Stack:** Go 1.23+, `database/sql`, `github.com/go-sql-driver/mysql`, `github.com/xwb1989/sqlparser`, existing SQLite audit store, existing table-driven Go tests.

---

## Reference

- Design spec: `/Users/repairman/opt/native-db-bridge/docs/superpowers/specs/2026-06-11-mysql-agent-query-workflow-design.md`
- Existing implementation plan style: `/Users/repairman/opt/native-db-bridge/docs/superpowers/plans/2026-06-05-native-db-bridge-mcp-implementation.md`
- Commit policy: run commit steps only after explicit user confirmation in the active execution session.

## File Structure

- Modify `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`  
  Add MySQL metadata/search/scan request and result structs.
- Modify `/Users/repairman/opt/native-db-bridge/internal/backend/backend.go`  
  Do not add workflow/search methods to base `SQLBackend`; new MySQL workflow methods stay in the tools-layer SQL extension interface.
- Modify `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`  
  Implement query normalization, row value normalization, column search, text scan planning, and text scan count execution. Task 6 imports `nbderrors` here to classify per-field text scan failures.
- Modify `/Users/repairman/opt/native-db-bridge/internal/backend/backend_test.go`  
  Add focused unit tests for limit handling and value normalization.
- Modify `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors.go`  
  Add SQL-specific error codes and retryable semantics.
- Create `/Users/repairman/opt/native-db-bridge/internal/nbderrors/mysql.go`  
  Map MySQL/driver error messages to structured error codes.
- Modify `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors_test.go`  
  Test SQL error code mapping and retryability.
- Modify `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`  
  Add input/output schemas for `sql_column_search`, `sql_text_column_plan`, `sql_text_scan`, and `audit_summary`.
- Modify `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go`  
  Keep required-tool coverage and hardcoded tool count aligned after each new tool is added.
- Modify `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`  
  Rename `SQLMetaBackend` to `SQLWorkflowBackend`, extend it, and use classified SQL errors when recording failures.
- Modify `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_metadata.go`  
  Add handlers for MySQL metadata workflow tools.
- Modify `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_execution.go`  
  Use classified SQL errors in `SQLQuery` and SQL `execute_confirmation` failures, add execution-style handlers for `sql_text_scan` and `audit_summary`.
- Modify `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`  
  Extend fake SQL backend and add handler tests.
- Modify `/Users/repairman/opt/native-db-bridge/internal/audit/store.go`  
  Add summary, top error, and confirmation summary query methods for P1.
- Modify `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`  
  Wire new tools into dispatcher, schema list, and descriptions.
- Modify `/Users/repairman/opt/native-db-bridge/README.md`  
  Document agent MySQL workflow and examples.
- Modify `/Users/repairman/opt/native-db-bridge/README.en.md`  
  Mirror the tool list and workflow summary in English.

## Task 1: SQL Query Normalization and Row Value Normalization

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/backend_test.go`

- [ ] **Step 1: Create or extend `TestApplyLimit` with failing cases**

If `TestApplyLimit` already exists in `/Users/repairman/opt/native-db-bridge/internal/backend/backend_test.go`, add these cases to it. If it does not exist in the execution branch, create a new table-driven `TestApplyLimit` function containing the existing baseline cases plus these cases:

```go
{
	name:         "trailing semicolon",
	sql:          "SELECT * FROM users;",
	limit:        100,
	wantSQL:      "SELECT * FROM (SELECT * FROM users) AS ndb_limited LIMIT ?",
	wantHasParam: true,
},
{
	name:         "show tables is not wrapped",
	sql:          "SHOW TABLES;",
	limit:        100,
	wantSQL:      "SHOW TABLES",
	wantHasParam: false,
},
{
	name:         "describe is not wrapped",
	sql:          "DESCRIBE users;",
	limit:        100,
	wantSQL:      "DESCRIBE users",
	wantHasParam: false,
},
{
	name:         "desc is not wrapped",
	sql:          "DESC users;",
	limit:        100,
	wantSQL:      "DESC users",
	wantHasParam: false,
},
{
	name:         "explain is not wrapped",
	sql:          "EXPLAIN SELECT * FROM users;",
	limit:        100,
	wantSQL:      "EXPLAIN SELECT * FROM users",
	wantHasParam: false,
},
{
	name:         "union without limit is wrapped",
	sql:          "SELECT id FROM users UNION ALL SELECT id FROM admins;",
	limit:        100,
	wantSQL:      "SELECT * FROM (SELECT id FROM users UNION ALL SELECT id FROM admins) AS ndb_limited LIMIT ?",
	wantHasParam: true,
},
```

- [ ] **Step 2: Add value normalization failing test**

Add this test to `/Users/repairman/opt/native-db-bridge/internal/backend/backend_test.go`:

```go
func TestNormalizeSQLValue(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{
			name: "utf8 bytes become string",
			in:   []byte("https://example.com/avatar.png"),
			want: "https://example.com/avatar.png",
		},
		{
			name: "nil stays nil",
			in:   nil,
			want: nil,
		},
		{
			name: "int64 stays int64",
			in:   int64(42),
			want: int64(42),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSQLValue(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeSQLValue(%v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeSQLValueBinary(t *testing.T) {
	got := normalizeSQLValue([]byte{0xff, 0x00, 0x01})
	asMap, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected binary map, got %#v", got)
	}
	if asMap["encoding"] != "base64" {
		t.Fatalf("encoding=%#v", asMap["encoding"])
	}
	if asMap["data"] != "/wAB" {
		t.Fatalf("data=%#v", asMap["data"])
	}
}
```

- [ ] **Step 3: Run failing backend tests**

Run:

```bash
go test ./internal/backend -run 'TestApplyLimit|TestNormalizeSQLValue' -v
```

Expected: FAIL because `SHOW`/semicolon handling and `normalizeSQLValue` do not exist yet.

- [ ] **Step 4: Implement query and value normalization**

In `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`, add imports:

```go
import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)
```

Replace `applyLimit` and add helpers near the bottom of the file:

```go
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
	// Keep DESC as a defensive DESCRIBE shorthand accepted by some clients.
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
```

In the row scan loop, replace:

```go
row[c.Name()] = values[i]
```

with:

```go
row[c.Name()] = normalizeSQLValue(values[i])
```

- [ ] **Step 5: Run backend tests**

Run:

```bash
go test ./internal/backend -run 'TestApplyLimit|TestNormalizeSQLValue' -v
go test ./internal/backend -v
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

After explicit user confirmation for committing, run:

```bash
git add internal/backend/sql_driver.go internal/backend/backend_test.go
git commit -m "fix(mysql): 修正只读查询包装与文本结果归一化" \
  -m "改动背景：MySQL 查询中 SHOW、尾部分号和文本列 []byte 输出会造成 ndb_limited 语法错误与 Base64 绕路。" \
  -m "改动动机：先修稳定性基础，避免 agent 在简单只读查询和 URL 字段读取上反复绕路。" \
  -m "关键决策：元数据语句不做子查询 LIMIT 包装；普通 SELECT/UNION 保持限行保护；UTF-8 字节转字符串，非 UTF-8 字节显式返回 base64 结构。" \
  -m "影响范围：仅影响 SQL 查询执行前包装和查询结果 JSON 表示，不改变 Redis、MongoDB 或写操作流程。" \
  -m "验证方式：go test ./internal/backend -run 'TestApplyLimit|TestNormalizeSQLValue' -v；go test ./internal/backend -v。"
```

## Task 2: Structured MySQL Error Classification

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/nbderrors/mysql.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_execution.go`

- [ ] **Step 1: Add failing error classification tests**

Add this test to `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors_test.go`:

```go
func TestClassifySQLError(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantCode  Code
		retryable bool
	}{
		{
			name:      "unknown column",
			message:   "sql query saas_support: Error 1054 (42S22): Unknown column 'remark' in 'field list'",
			wantCode:  CodeSQLUnknownColumn,
			retryable: false,
		},
		{
			name:      "unknown table",
			message:   "sql query saas_support: Error 1146 (42S02): Table 'saas_support.tch_x' doesn't exist",
			wantCode:  CodeSQLUnknownTable,
			retryable: false,
		},
		{
			name:      "syntax",
			message:   "sql query saas_support: Error 1064 (42000): You have an error in your SQL syntax",
			wantCode:  CodeQuerySyntaxError,
			retryable: false,
		},
		{
			name:      "timeout",
			message:   "sql query saas_support: context deadline exceeded",
			wantCode:  CodeQueryTimeout,
			retryable: true,
		},
		{
			name:      "connection reset",
			message:   "sql query saas_support: connection reset by peer",
			wantCode:  CodeConnectionFailed,
			retryable: true,
		},
		{
			name:      "other driver",
			message:   "sql query saas_support: unexpected driver failure",
			wantCode:  CodeDriverError,
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifySQLErrorMessage(tt.message)
			if err.Code != tt.wantCode {
				t.Fatalf("code=%s, want %s", err.Code, tt.wantCode)
			}
			if err.Retryable != tt.retryable {
				t.Fatalf("retryable=%v, want %v", err.Retryable, tt.retryable)
			}
		})
	}
}
```

- [ ] **Step 2: Run failing error tests**

Run:

```bash
go test ./internal/nbderrors -run TestClassifySQLError -v
```

Expected: FAIL because `CodeSQLUnknownColumn`, `CodeSQLUnknownTable`, and `ClassifySQLErrorMessage` do not exist.

- [ ] **Step 3: Add SQL error codes and classifier**

In `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors.go`, add codes:

```go
CodeSQLUnknownColumn Code = "SQL_UNKNOWN_COLUMN"
CodeSQLUnknownTable  Code = "SQL_UNKNOWN_TABLE"
```

Update `categoryFor`:

```go
case CodeQuerySyntaxError, CodeSQLUnknownColumn, CodeSQLUnknownTable:
	return CategorySyntax
```

Keep `retryableFor` as:

```go
func retryableFor(code Code) bool {
	return code == CodeConnectionFailed || code == CodeQueryTimeout || code == CodeDriverError
}
```

Create `/Users/repairman/opt/native-db-bridge/internal/nbderrors/mysql.go`:

```go
package nbderrors

import "strings"

// ClassifySQLErrorMessage maps common MySQL and driver failures to actionable
// native-db-bridge error codes.
func ClassifySQLErrorMessage(message string) *Error {
	lower := strings.ToLower(message)

	switch {
	case strings.Contains(lower, "context deadline exceeded"):
		return New(CodeQueryTimeout, message)
	case strings.Contains(lower, "error 1054") || strings.Contains(lower, "unknown column"):
		return New(CodeSQLUnknownColumn, message)
	case strings.Contains(lower, "error 1146") || (strings.Contains(lower, "table") && strings.Contains(lower, "doesn't exist")):
		return New(CodeSQLUnknownTable, message)
	case strings.Contains(lower, "error 1064") || strings.Contains(lower, "sql syntax"):
		return New(CodeQuerySyntaxError, message)
	case strings.Contains(lower, "broken pipe") || strings.Contains(lower, "connection reset by peer"):
		return New(CodeConnectionFailed, message)
	default:
		return New(CodeDriverError, message)
	}
}
```

- [ ] **Step 4: Use classified SQL errors in handlers**

In `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`, add:

```go
func makeErrorFromNative(err *nbderrors.Error) BaseOutput {
	return BaseOutput{
		OK: false,
		Error: &ErrorOutput{
			Code:      string(err.Code),
			Category:  string(err.Category),
			Message:   err.Message,
			Retryable: err.Retryable,
		},
	}
}

func (h *Handlers) finishSQLOperation(operationID string, err error) {
	if err == nil {
		h.finishOperation(operationID, nil)
		return
	}
	classified := nbderrors.ClassifySQLErrorMessage(err.Error())
	h.deps.Ops.Finish(operationID)
	_ = h.deps.Audit.MarkOperationFinished(operationID, string(classified.Code), err.Error())
}

func (h *Handlers) recordSQLAuditEvent(eventType, datasource, opID, summary, status string, elapsedMs int64, rowCount int, err error) {
	evt := audit.AuditEvent{
		ID:          "evt_" + uuid.New().String(),
		EventType:   eventType,
		Datasource:  datasource,
		OperationID: opID,
		Summary:     summary,
		Status:      status,
		ElapsedMs:   elapsedMs,
		RowCount:    rowCount,
		CreatedAt:   time.Now(),
	}
	if err != nil {
		evt.ErrorCode = string(nbderrors.ClassifySQLErrorMessage(err.Error()).Code)
	}
	_ = h.deps.Audit.InsertAuditEvent(evt)
}
```

Keep the shared `finishOperation` and `recordAuditEvent` error branches unchanged so Redis and MongoDB keep their existing `DRIVER_ERROR` fallback semantics. Use `finishSQLOperation` and `recordSQLAuditEvent` only from SQL-specific handlers that should persist classified MySQL error codes.

In `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_execution.go`, change SQL query error branch from:

```go
h.recordAuditEvent("sql_query", input.Datasource, opID, "", input.SQL, "error", 0, 0, err)
h.finishOperation(opID, err)
return SQLQueryOutput{BaseOutput: makeError(nbderrors.CodeDriverError, err.Error()).withOpID(opID)}
```

to:

```go
h.recordSQLAuditEvent("sql_query", input.Datasource, opID, input.SQL, "error", 0, 0, err)
classified := nbderrors.ClassifySQLErrorMessage(err.Error())
h.finishSQLOperation(opID, err)
return SQLQueryOutput{BaseOutput: makeErrorFromNative(classified).withOpID(opID)}
```

Also classify SQL write confirmation execution failures. In `ExecuteConfirmation`, when `conf.Kind` is `sql_dml` or `sql_ddl` and `execErr != nil`, use `recordSQLAuditEvent`, `finishSQLOperation`, and `makeErrorFromNative(nbderrors.ClassifySQLErrorMessage(execErr.Error()))`. Keep Redis and Mongo confirmation failures on the existing generic branches.

- [ ] **Step 5: Add SQL handler classification test**

Add to `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
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
```

Add a companion confirmation test:

```go
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
```

- [ ] **Step 6: Run error tests**

Run:

```bash
go test ./internal/nbderrors -v
go test ./internal/tools -run TestSQLQueryClassifiesUnknownColumn -v
go test ./internal/tools -v
```

Expected: PASS.

- [ ] **Step 7: Commit Task 2**

After explicit user confirmation for committing, run:

```bash
git add internal/nbderrors/errors.go internal/nbderrors/mysql.go internal/nbderrors/errors_test.go internal/tools/handlers.go internal/tools/handlers_execution.go internal/tools/handlers_test.go
git commit -m "feat(mysql): 细化 SQL 错误分类" \
  -m "改动背景：MySQL 查询失败此前大多表现为 DRIVER_ERROR，语法错、未知字段、缺表和超时缺少可行动分类。" \
  -m "改动动机：让 agent 能根据错误码选择字段搜索、对象列表、缩小扫描范围或重试连接。" \
  -m "关键决策：1054 映射为 SQL_UNKNOWN_COLUMN，1146 映射为 SQL_UNKNOWN_TABLE，1064 映射为 QUERY_SYNTAX_ERROR，超时映射为 QUERY_TIMEOUT。" \
  -m "影响范围：SQL 查询错误输出、operation error_code 和 audit event error_code；Redis/Mongo 仍保留现有路径。" \
  -m "验证方式：go test ./internal/nbderrors -v；go test ./internal/tools -v。"
```

## Task 3: Backend SQL Column Search

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`

- [ ] **Step 1: Add backend types**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`:

```go
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
```

- [ ] **Step 2: Rename and extend SQL workflow backend interface**

In `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`, rename internal interface `SQLMetaBackend` to `SQLWorkflowBackend` and update `Deps.SQL` to use the new name. This is a tools-package internal rename only; it does not change the public MCP contract or `backend.SQLBackend`.

Then add this method to `SQLWorkflowBackend`:

```go
ColumnSearch(ctx context.Context, req backend.SQLColumnSearchRequest) ([]backend.SQLColumnSearchResult, error)
```

Update `fakeSQLBackend` in `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
columnSearchCalled bool
columnSearchReq    backend.SQLColumnSearchRequest
columnSearchRows   []backend.SQLColumnSearchResult
```

Add method:

```go
func (f *fakeSQLBackend) ColumnSearch(_ context.Context, req backend.SQLColumnSearchRequest) ([]backend.SQLColumnSearchResult, error) {
	f.columnSearchCalled = true
	f.columnSearchReq = req
	return f.columnSearchRows, nil
}
```

- [ ] **Step 3: Add SQL driver test with sqlmock-free query builder hook**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/backend_test.go`:

```go
func TestBuildColumnSearchQuery(t *testing.T) {
	req := SQLColumnSearchRequest{
		Schema:        "saas_support",
		TablePattern:  "%user%",
		ColumnPattern: "%avatar%",
		DataTypes:     []string{"varchar", "text"},
		Limit:         50,
	}
	gotSQL, gotArgs := buildColumnSearchQuery(req, 1000)
	wantSQL := "SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, COLUMN_COMMENT FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME LIKE ? AND COLUMN_NAME LIKE ? AND DATA_TYPE IN (?, ?) ORDER BY TABLE_NAME, ORDINAL_POSITION LIMIT ?"
	if gotSQL != wantSQL {
		t.Fatalf("sql=%q, want %q", gotSQL, wantSQL)
	}
	wantArgs := []interface{}{"saas_support", "%user%", "%avatar%", "varchar", "text", 50}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args len=%d, want %d", len(gotArgs), len(wantArgs))
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("arg[%d]=%#v, want %#v", i, gotArgs[i], wantArgs[i])
		}
	}
}
```

- [ ] **Step 4: Run failing backend test**

Run:

```bash
go test ./internal/backend -run TestBuildColumnSearchQuery -v
```

Expected: FAIL because `buildColumnSearchQuery` does not exist.

- [ ] **Step 5: Implement query builder and `ColumnSearch`**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`:

```go
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
```

- [ ] **Step 6: Run backend tests**

Run:

```bash
go test ./internal/backend -run TestBuildColumnSearchQuery -v
go test ./internal/backend -v
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

After explicit user confirmation for committing, run:

```bash
git add internal/backend/sql.go internal/backend/sql_driver.go internal/backend/backend_test.go internal/tools/handlers.go internal/tools/handlers_test.go
git commit -m "feat(mysql): 增加字段元数据搜索后端" \
  -m "改动背景：agent 在 MySQL 查询前缺少字段发现能力，导致大量 Unknown column 失败。" \
  -m "改动动机：先提供 information_schema 字段搜索后端，为 MCP 工具层提供稳定基础。" \
  -m "关键决策：schema 必填，支持表名、字段名、数据类型和 limit 过滤；将内部 SQLMetaBackend 重命名为 SQLWorkflowBackend。" \
  -m "影响范围：新增 SQL workflow backend 能力和测试桩，不改变现有工具输出。" \
  -m "验证方式：go test ./internal/backend -run TestBuildColumnSearchQuery -v；go test ./internal/backend -v。"
```

## Task 4: Expose `sql_column_search` MCP Tool

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_metadata.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`

- [ ] **Step 1: Add schema test**

Add to `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go`:

```go
func TestToolNamesIncludesSQLColumnSearch(t *testing.T) {
	names := ToolNames()
	found := false
	for _, name := range names {
		if name == "sql_column_search" {
			found = true
		}
	}
	if !found {
		t.Fatal("ToolNames should include sql_column_search")
	}
}
```

- [ ] **Step 2: Run failing schema test**

Run:

```bash
go test ./internal/tools -run TestToolNamesIncludesSQLColumnSearch -v
```

Expected: FAIL because `ToolNames` does not include `sql_column_search`.

- [ ] **Step 3: Add input/output schemas and tool name**

In `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`, add `sql_column_search` to `ToolNames` metadata section.

Update `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go` in the same change:

- Add `sql_column_search` to `TestToolSchemasIncludeRequiredTools`.
- Change `TestToolNamesCount` from `24` to `25`.

Add input/output types:

```go
type SQLColumnSearchInput struct {
	Datasource    string   `json:"datasource"`
	Schema        string   `json:"schema"`
	TablePattern  string   `json:"table_pattern,omitempty"`
	ColumnPattern string   `json:"column_pattern,omitempty"`
	DataTypes     []string `json:"data_types,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}

type SQLColumnSearchOutput struct {
	BaseOutput
	Columns []backend.SQLColumnSearchResult `json:"columns"`
}
```

- [ ] **Step 4: Add handler**

In `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_metadata.go`, add:

```go
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
```

- [ ] **Step 5: Add handler test**

Add to `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
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
```

- [ ] **Step 6: Wire MCP dispatcher**

In `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`, add dispatcher entry:

```go
"sql_column_search": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var input tools.SQLColumnSearchInput
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return h.SQLColumnSearch(ctx, input), nil
},
```

Add input schema mapping:

```go
"sql_column_search": inputTypeOf[tools.SQLColumnSearchInput](),
```

Add description:

```go
"sql_column_search": "Search MySQL column metadata by schema, table pattern, column pattern, and data type.",
```

- [ ] **Step 7: Run tools/server tests**

Run:

```bash
go test ./internal/tools -run 'TestToolNamesIncludesSQLColumnSearch|TestSQLColumnSearchSuccess' -v
go test ./internal/tools -v
go test ./internal/server -v
```

Expected: PASS.

- [ ] **Step 8: Commit Task 4**

After explicit user confirmation for committing, run:

```bash
git add internal/tools/schema.go internal/tools/schema_test.go internal/tools/handlers_metadata.go internal/tools/handlers_test.go internal/server/mcp.go
git commit -m "feat(mysql): 暴露字段搜索 MCP 工具" \
  -m "改动背景：MySQL 字段搜索后端已具备，需要通过 MCP 工具提供给 agent 使用。" \
  -m "改动动机：让 agent 在写业务 SQL 前可先查询真实字段，减少 1054 Unknown column。" \
  -m "关键决策：新增 sql_column_search，schema 必填，入参保持元数据查询边界，不扫描业务数据。" \
  -m "影响范围：MCP 工具列表、工具 schema、handler 和 server dispatcher。" \
  -m "验证方式：go test ./internal/tools -v；go test ./internal/server -v。"
```

## Task 5: Text Column Plan Backend and Tool

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_metadata.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`

- [ ] **Step 1: Add backend plan types**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`:

```go
type SQLTextColumnPlanRequest struct {
	Datasource    string
	Schema        string
	TablePattern  string
	ColumnPattern string
	Keywords      []string
	MaxTables     int
	MaxColumns    int
}

type SQLTextColumnCandidate struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Column     string `json:"column"`
	DataType   string `json:"data_type"`
	ColumnType string `json:"column_type"`
}

type SQLTextScanBatch struct {
	Targets  []SQLTextScanTarget `json:"targets"`
	Keywords []string            `json:"keywords"`
}

type SQLTextColumnPlanResult struct {
	Candidates []SQLTextColumnCandidate `json:"candidates"`
	Batches    []SQLTextScanBatch       `json:"batches"`
	Warnings   []string                 `json:"warnings,omitempty"`
}

type SQLTextScanTarget struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}
```

- [ ] **Step 2: Extend backend interface and fake backend**

Add to `SQLWorkflowBackend` in `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`:

```go
TextColumnPlan(ctx context.Context, req backend.SQLTextColumnPlanRequest) (backend.SQLTextColumnPlanResult, error)
```

Add to `fakeSQLBackend` in `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
textPlanCalled bool
textPlanReq    backend.SQLTextColumnPlanRequest
textPlanResult backend.SQLTextColumnPlanResult
```

Add method:

```go
func (f *fakeSQLBackend) TextColumnPlan(_ context.Context, req backend.SQLTextColumnPlanRequest) (backend.SQLTextColumnPlanResult, error) {
	f.textPlanCalled = true
	f.textPlanReq = req
	return f.textPlanResult, nil
}
```

- [ ] **Step 3: Add plan builder test**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/backend_test.go`:

```go
func TestBuildTextColumnPlanQuery(t *testing.T) {
	req := SQLTextColumnPlanRequest{
		Schema:        "saas_support",
		TablePattern:  "uc_%",
		ColumnPattern: "%image%",
		MaxColumns:    25,
	}
	gotSQL, gotArgs := buildTextColumnPlanQuery(req, 1000)
	wantSQL := "SELECT TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND DATA_TYPE IN (?, ?, ?, ?, ?, ?, ?) AND TABLE_NAME LIKE ? AND COLUMN_NAME LIKE ? ORDER BY TABLE_NAME, ORDINAL_POSITION LIMIT ?"
	if gotSQL != wantSQL {
		t.Fatalf("sql=%q, want %q", gotSQL, wantSQL)
	}
	wantArgs := []interface{}{"saas_support", "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "json", "uc_%", "%image%", 25}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("args len=%d, want %d", len(gotArgs), len(wantArgs))
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("arg[%d]=%#v, want %#v", i, gotArgs[i], wantArgs[i])
		}
	}
}
```

- [ ] **Step 4: Run failing plan builder test**

Run:

```bash
go test ./internal/backend -run TestBuildTextColumnPlanQuery -v
```

Expected: FAIL because `buildTextColumnPlanQuery` does not exist.

- [ ] **Step 5: Implement text column planning**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`:

```go
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
```

- [ ] **Step 6: Add tool schema and handler**

In `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`, add `sql_text_column_plan` to `ToolNames`.

Update `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go` in the same change:

- Add `sql_text_column_plan` to `TestToolSchemasIncludeRequiredTools`.
- Change `TestToolNamesCount` from `25` to `26`.

Add:

```go
type SQLTextColumnPlanInput struct {
	Datasource    string   `json:"datasource"`
	Schema        string   `json:"schema"`
	TablePattern  string   `json:"table_pattern,omitempty"`
	ColumnPattern string   `json:"column_pattern,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	MaxTables     int      `json:"max_tables,omitempty"`
	MaxColumns    int      `json:"max_columns,omitempty"`
}

type SQLTextColumnPlanOutput struct {
	BaseOutput
	Candidates []backend.SQLTextColumnCandidate `json:"candidates"`
	Batches    []backend.SQLTextScanBatch       `json:"batches"`
	Warnings   []string                         `json:"warnings,omitempty"`
}
```

In `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_metadata.go`, add:

```go
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
```

- [ ] **Step 7: Add handler test**

Add to `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
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
```

- [ ] **Step 8: Wire MCP dispatcher**

In `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`, add dispatcher, input schema, and description for `sql_text_column_plan`:

```go
"sql_text_column_plan": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var input tools.SQLTextColumnPlanInput
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return h.SQLTextColumnPlan(ctx, input), nil
},
```

```go
"sql_text_column_plan": inputTypeOf[tools.SQLTextColumnPlanInput](),
```

```go
"sql_text_column_plan": "Build a safe MySQL text-column scan plan from metadata without scanning business rows.",
```

- [ ] **Step 9: Run tests**

Run:

```bash
go test ./internal/backend -run TestBuildTextColumnPlanQuery -v
go test ./internal/tools -run TestSQLTextColumnPlanSuccess -v
go test ./internal/tools -v
go test ./internal/server -v
```

Expected: PASS.

- [ ] **Step 10: Commit Task 5**

After explicit user confirmation for committing, run:

```bash
git add internal/backend/sql.go internal/backend/sql_driver.go internal/backend/backend_test.go internal/tools/schema.go internal/tools/schema_test.go internal/tools/handlers.go internal/tools/handlers_metadata.go internal/tools/handlers_test.go internal/server/mcp.go
git commit -m "feat(mysql): 增加文本列扫描计划工具" \
  -m "改动背景：URL 和文本扫描此前依赖 agent 手工拼宽范围 LIKE，容易超时或误扫字段。" \
  -m "改动动机：先基于 information_schema 生成候选文本列和扫描批次，让 agent 先收窄范围。" \
  -m "关键决策：sql_text_column_plan 只读元数据，不扫描业务表；候选列限定文本类型，并返回 warnings 提示收窄范围。" \
  -m "影响范围：新增 MySQL 元数据计划能力和 MCP 工具，不改变 sql_query 行为。" \
  -m "验证方式：go test ./internal/backend -run TestBuildTextColumnPlanQuery -v；go test ./internal/tools -v；go test ./internal/server -v。"
```

## Task 6: Text Scan Count Mode

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_execution.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`

- [ ] **Step 1: Add scan result types**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`:

```go
type SQLTextScanRequest struct {
	Datasource         string
	Schema             string
	Targets            []SQLTextScanTarget
	Keywords           []string
	Mode               string
	MaxColumnsPerQuery int
	Timeout            time.Duration
}

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

type SQLTextScanResult struct {
	Matches []SQLTextScanMatch `json:"matches"`
}
```

P0 scans one target-keyword pair at a time. `MaxColumnsPerQuery` is accepted and passed through as a reserved P2 batching control; it is intentionally not used for grouping until P2-style batching is extended in a later plan. `Timeout` is the per target-keyword timeout and must already be capped by the handler to the global query timeout.

- [ ] **Step 2: Extend backend interface and fake backend**

Add to `SQLWorkflowBackend` in `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`:

```go
TextScan(ctx context.Context, req backend.SQLTextScanRequest) (backend.SQLTextScanResult, error)
```

Add to `fakeSQLBackend` in `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
textScanCalled bool
textScanReq    backend.SQLTextScanRequest
textScanResult backend.SQLTextScanResult
```

Add method:

```go
func (f *fakeSQLBackend) TextScan(_ context.Context, req backend.SQLTextScanRequest) (backend.SQLTextScanResult, error) {
	f.textScanCalled = true
	f.textScanReq = req
	return f.textScanResult, nil
}
```

- [ ] **Step 3: Add count SQL builder test**

Add to `/Users/repairman/opt/native-db-bridge/internal/backend/backend_test.go`:

```go
func TestBuildTextCountSQL(t *testing.T) {
	sqlStr, args := buildTextCountSQL("saas_support", SQLTextScanTarget{Table: "uc_user_info", Column: "avatar"}, "oss")
	wantSQL := "SELECT COUNT(*) FROM `saas_support`.`uc_user_info` WHERE `avatar` LIKE ?"
	if sqlStr != wantSQL {
		t.Fatalf("sql=%q, want %q", sqlStr, wantSQL)
	}
	if len(args) != 1 || args[0] != "%oss%" {
		t.Fatalf("args=%#v", args)
	}
}
```

- [ ] **Step 4: Run failing scan builder test**

Run:

```bash
go test ./internal/backend -run TestBuildTextCountSQL -v
```

Expected: FAIL because `buildTextCountSQL` does not exist.

- [ ] **Step 5: Implement text scan count**

Ensure `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go` imports include:

```go
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
```

Then add to `/Users/repairman/opt/native-db-bridge/internal/backend/sql_driver.go`:

```go
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

func buildTextCountSQL(schema string, target SQLTextScanTarget, keyword string) (string, []interface{}) {
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`%s` WHERE `%s` LIKE ?", escapeIdentifier(schema), escapeIdentifier(target.Table), escapeIdentifier(target.Column))
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
```

- [ ] **Step 6: Add tool schema and handler**

In `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`, add `sql_text_scan` to `ToolNames`.

Update `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go` in the same change:

- Add `sql_text_scan` to `TestToolSchemasIncludeRequiredTools`.
- Change `TestToolNamesCount` from `26` to `27`.

Add:

```go
type SQLTextScanInput struct {
	Datasource         string                      `json:"datasource"`
	Schema             string                      `json:"schema"`
	Targets            []backend.SQLTextScanTarget `json:"targets"`
	Keywords           []string                    `json:"keywords"`
	Mode               string                      `json:"mode,omitempty"`
	MaxColumnsPerQuery int                         `json:"max_columns_per_query,omitempty"`
	Timeout            string                      `json:"timeout,omitempty"`
}

type SQLTextScanOutput struct {
	BaseOutput
	Matches []SQLTextScanMatchOutput `json:"matches"`
}

type SQLTextScanMatchOutput struct {
	Table     string `json:"table"`
	Column    string `json:"column"`
	Keyword   string `json:"keyword"`
	Count     int64  `json:"count"`
	ElapsedMs int64  `json:"elapsed_ms"`
	TimedOut  bool   `json:"timed_out,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}
```

In `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_execution.go`, add:

```go
func (h *Handlers) SQLTextScan(ctx context.Context, input SQLTextScanInput) SQLTextScanOutput {
	ctx, opID := h.withOperation(ctx, "sql_text_scan", input.Datasource, "")
	var opErr error
	defer func() { h.finishSQLOperation(opID, opErr) }()

	if input.Schema == "" {
		opErr = fmt.Errorf("schema is required")
		return SQLTextScanOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, "schema is required")}
	}
	if len(input.Targets) == 0 {
		opErr = fmt.Errorf("targets are required")
		return SQLTextScanOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, "targets are required")}
	}
	if len(input.Keywords) == 0 {
		opErr = fmt.Errorf("keywords are required")
		return SQLTextScanOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, "keywords are required")}
	}

	result, err := h.deps.SQL.TextScan(ctx, backend.SQLTextScanRequest{
		Datasource:         input.Datasource,
		Schema:             input.Schema,
		Targets:            input.Targets,
		Keywords:           input.Keywords,
		Mode:               input.Mode,
		MaxColumnsPerQuery: input.MaxColumnsPerQuery,
		Timeout:            h.parseTimeout(input.Timeout),
	})
	if err != nil {
		opErr = err
		return SQLTextScanOutput{BaseOutput: makeErrorFromNative(nbderrors.ClassifySQLErrorMessage(err.Error()))}
	}

	matches := make([]SQLTextScanMatchOutput, 0, len(result.Matches))
	for _, match := range result.Matches {
			matches = append(matches, SQLTextScanMatchOutput{
				Table:     match.Table,
				Column:    match.Column,
				Keyword:   match.Keyword,
				Count:     match.Count,
				ElapsedMs: match.Elapsed.Milliseconds(),
				TimedOut:  match.TimedOut,
				Error:     match.Error,
				ErrorCode: match.ErrorCode,
			})
		}
	return SQLTextScanOutput{BaseOutput: BaseOutput{OK: true}, Matches: matches}
}
```

	P0 returns elapsed time per target-keyword match via `elapsed_ms` and timeout state via `timed_out`. Batch-level elapsed and explicit `skipped` flags are deferred to the P2 scan-batching enhancement because P0 does not group targets into multi-column batches.

- [ ] **Step 7: Add handler test**

Add to `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
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
```

- [ ] **Step 8: Wire MCP dispatcher**

In `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`, add dispatcher, input schema, and description for `sql_text_scan`:

```go
"sql_text_scan": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var input tools.SQLTextScanInput
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return h.SQLTextScan(ctx, input), nil
},
```

```go
"sql_text_scan": inputTypeOf[tools.SQLTextScanInput](),
```

```go
"sql_text_scan": "Run controlled count-only scans across selected MySQL text columns.",
```

- [ ] **Step 9: Run tests**

Run:

```bash
go test ./internal/backend -run TestBuildTextCountSQL -v
go test ./internal/tools -run TestSQLTextScanSuccess -v
go test ./internal/backend -v
go test ./internal/tools -v
go test ./internal/server -v
```

Expected: PASS.

- [ ] **Step 10: Commit Task 6**

After explicit user confirmation for committing, run:

```bash
git add internal/backend/sql.go internal/backend/sql_driver.go internal/backend/backend_test.go internal/tools/schema.go internal/tools/schema_test.go internal/tools/handlers.go internal/tools/handlers_execution.go internal/tools/handlers_test.go internal/server/mcp.go
git commit -m "feat(mysql): 增加受控文本列扫描工具" \
  -m "改动背景：宽范围文本扫描此前容易拼出巨大 SQL 并触发超时。" \
  -m "改动动机：提供 count-only 的受控扫描入口，让 agent 对指定字段和关键词逐项统计命中数量。" \
  -m "关键决策：P0 仅支持 count 模式，不返回业务明细；每个字段和关键词单独记录耗时与错误码。" \
  -m "影响范围：新增 MySQL 文本扫描 backend、tools schema、handler 和 MCP dispatcher。" \
  -m "验证方式：go test ./internal/backend -v；go test ./internal/tools -v；go test ./internal/server -v。"
```

## Task 7: Audit Summary Tool

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/audit/store.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/audit/store_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_execution.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`

- [ ] **Step 1: Add audit summary types**

Add to `/Users/repairman/opt/native-db-bridge/internal/audit/store.go` near audit event types:

```go
type SummaryFilter struct {
	StartTime  time.Time
	EndTime    time.Time
	Datasource string
	EventType  string
	Status     string
	GroupBy    string
	Limit      int
}

type SummaryRow struct {
	Bucket       string
	EventType    string
	Datasource   string
	Status       string
	ErrorCode    string
	Count        int
	Slow1sCount  int
	Slow5sCount  int
	Slow10sCount int
}

type TopErrorSummary struct {
	ErrorCode string
	Summary   string
	Count     int
}

type ConfirmationSummaryRow struct {
	Kind       string
	Datasource string
	Status     string
	Count      int
	ErrorCount int
}
```

- [ ] **Step 2: Add audit store test**

Add to `/Users/repairman/opt/native-db-bridge/internal/audit/store_test.go`:

```go
func TestAuditSummaryByErrorCode(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	err = store.InsertAuditEvent(AuditEvent{
		ID:         "evt_1",
		EventType:  "sql_query",
		Datasource: "saas_support",
		Summary:    "SELECT missing_col FROM users",
		Status:     "error",
		ErrorCode:  "SQL_UNKNOWN_COLUMN",
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateConfirmation(Confirmation{
		ID:          "conf_1",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		Summary:     "UPDATE users SET name = ? WHERE id = ?",
		RiskLevel:   "medium",
		ImpactJSON:  "{}",
		Status:      "pending",
		ExpiresAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkConfirmationExecuting("conf_1"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkConfirmationExecuted("conf_1"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.Summary(SummaryFilter{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		Datasource: "saas_support",
		GroupBy:    "error_code",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%#v", rows)
	}
	if rows[0].ErrorCode != "SQL_UNKNOWN_COLUMN" || rows[0].Count != 1 {
		t.Fatalf("row=%#v", rows[0])
	}
	topErrors, err := store.TopErrorSummaries(SummaryFilter{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		Datasource: "saas_support",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(topErrors) != 1 || topErrors[0].Summary != "SELECT missing_col FROM users" {
		t.Fatalf("topErrors=%#v", topErrors)
	}
	confirmations, err := store.ConfirmationSummary(SummaryFilter{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		Datasource: "saas_support",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(confirmations) != 1 || confirmations[0].Status != "executed" || confirmations[0].Count != 1 {
		t.Fatalf("confirmations=%#v", confirmations)
	}
}
```

- [ ] **Step 3: Run failing audit test**

Run:

```bash
go test ./internal/audit -run TestAuditSummaryByErrorCode -v
```

Expected: FAIL because `Summary` does not exist.

- [ ] **Step 4: Implement audit summary**

Add to `/Users/repairman/opt/native-db-bridge/internal/audit/store.go`:

```go
func (s *Store) Summary(filter SummaryFilter) ([]SummaryRow, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.GroupBy == "" {
		filter.GroupBy = "error_code"
	}

	groupExpr := "COALESCE(error_code, '')"
	switch filter.GroupBy {
	case "day":
		groupExpr = "substr(created_at, 1, 10)"
	case "event_type":
		groupExpr = "event_type"
	case "datasource":
		groupExpr = "datasource"
	case "status":
		groupExpr = "status"
	case "error_code":
		groupExpr = "COALESCE(error_code, '')"
	}

	query := fmt.Sprintf(`SELECT %s AS bucket,
		COALESCE(event_type, ''), COALESCE(datasource, ''), COALESCE(status, ''), COALESCE(error_code, ''),
		COUNT(*),
		SUM(CASE WHEN elapsed_ms >= 1000 THEN 1 ELSE 0 END),
		SUM(CASE WHEN elapsed_ms >= 5000 THEN 1 ELSE 0 END),
		SUM(CASE WHEN elapsed_ms >= 10000 THEN 1 ELSE 0 END)
		FROM audit_events WHERE 1=1`, groupExpr)
	args := []interface{}{}
	if !filter.StartTime.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, filter.EndTime)
	}
	if filter.Datasource != "" {
		query += " AND datasource = ?"
		args = append(args, filter.Datasource)
	}
	if filter.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, filter.EventType)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	query += fmt.Sprintf(" GROUP BY %s, COALESCE(event_type, ''), COALESCE(datasource, ''), COALESCE(status, ''), COALESCE(error_code, '') ORDER BY COUNT(*) DESC LIMIT ?", groupExpr)
	args = append(args, filter.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: summary: %w", err)
	}
	defer rows.Close()

	var results []SummaryRow
	for rows.Next() {
		var row SummaryRow
		if err := rows.Scan(&row.Bucket, &row.EventType, &row.Datasource, &row.Status, &row.ErrorCode, &row.Count, &row.Slow1sCount, &row.Slow5sCount, &row.Slow10sCount); err != nil {
			return nil, fmt.Errorf("audit: scan summary: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (s *Store) TopErrorSummaries(filter SummaryFilter) ([]TopErrorSummary, error) {
	if filter.Limit <= 0 {
		filter.Limit = 10
	}
	query := `SELECT COALESCE(error_code, ''), COALESCE(summary, ''), COUNT(*)
		FROM audit_events WHERE COALESCE(error_code, '') != '' AND COALESCE(summary, '') != ''`
	args := []interface{}{}
	if filter.Status == "" {
		query += " AND status = 'error'"
	}
	if !filter.StartTime.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, filter.EndTime)
	}
	if filter.Datasource != "" {
		query += " AND datasource = ?"
		args = append(args, filter.Datasource)
	}
	if filter.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, filter.EventType)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	query += " GROUP BY error_code, summary ORDER BY COUNT(*) DESC LIMIT ?"
	args = append(args, filter.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: top error summaries: %w", err)
	}
	defer rows.Close()

	var results []TopErrorSummary
	for rows.Next() {
		var row TopErrorSummary
		if err := rows.Scan(&row.ErrorCode, &row.Summary, &row.Count); err != nil {
			return nil, fmt.Errorf("audit: scan top error summaries: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (s *Store) ConfirmationSummary(filter SummaryFilter) ([]ConfirmationSummaryRow, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	query := `SELECT COALESCE(kind, ''), COALESCE(datasource, ''), COALESCE(status, ''),
		COUNT(*),
		SUM(CASE WHEN COALESCE(error_summary, '') != '' THEN 1 ELSE 0 END)
		FROM confirmations WHERE 1=1`
	args := []interface{}{}
	if !filter.StartTime.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, filter.EndTime)
	}
	if filter.Datasource != "" {
		query += " AND datasource = ?"
		args = append(args, filter.Datasource)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	query += " GROUP BY kind, datasource, status ORDER BY COUNT(*) DESC LIMIT ?"
	args = append(args, filter.Limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: confirmation summary: %w", err)
	}
	defer rows.Close()

	var results []ConfirmationSummaryRow
	for rows.Next() {
		var row ConfirmationSummaryRow
		if err := rows.Scan(&row.Kind, &row.Datasource, &row.Status, &row.Count, &row.ErrorCount); err != nil {
			return nil, fmt.Errorf("audit: scan confirmation summary: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
```

- [ ] **Step 5: Add tool schema and handler**

In `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`, add `audit_summary` to `ToolNames`.

Update `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go` in the same change:

- Add `audit_summary` to `TestToolSchemasIncludeRequiredTools`.
- Change `TestToolNamesCount` from `27` to `28`.

Add:

```go
type AuditSummaryInput struct {
	StartTime  string `json:"start_time,omitempty"`
	EndTime    string `json:"end_time,omitempty"`
	Datasource string `json:"datasource,omitempty"`
	EventType  string `json:"event_type,omitempty"`
	Status     string `json:"status,omitempty"`
	GroupBy    string `json:"group_by,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type AuditSummaryOutput struct {
	BaseOutput
	Rows                []AuditSummaryRow                `json:"rows"`
	TopErrors           []AuditTopErrorSummary           `json:"top_errors"`
	ConfirmationSummary []AuditConfirmationSummaryOutput `json:"confirmation_summary"`
}

type AuditSummaryRow struct {
	Bucket       string `json:"bucket"`
	EventType    string `json:"event_type"`
	Datasource   string `json:"datasource"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	Count        int    `json:"count"`
	Slow1sCount  int    `json:"slow_1s_count"`
	Slow5sCount  int    `json:"slow_5s_count"`
	Slow10sCount int    `json:"slow_10s_count"`
}

type AuditTopErrorSummary struct {
	ErrorCode string `json:"error_code"`
	Summary   string `json:"summary"`
	Count     int    `json:"count"`
}

type AuditConfirmationSummaryOutput struct {
	Kind       string `json:"kind"`
	Datasource string `json:"datasource"`
	Status     string `json:"status"`
	Count      int    `json:"count"`
	ErrorCount int    `json:"error_count"`
}
```

In `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_execution.go`, add:

```go
func (h *Handlers) AuditSummary(ctx context.Context, input AuditSummaryInput) AuditSummaryOutput {
	_, opID := h.withOperation(ctx, "audit_summary", "", "")
	var opErr error
	defer func() { h.finishOperation(opID, opErr) }()

	if !isAllowedAuditSummaryGroupBy(input.GroupBy) {
		opErr = fmt.Errorf("unsupported group_by %q", input.GroupBy)
		return AuditSummaryOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, fmt.Sprintf("unsupported group_by %q", input.GroupBy))}
	}

	startTime, err := parseOptionalTime(input.StartTime)
	if err != nil {
		opErr = err
		return AuditSummaryOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, err.Error())}
	}
	endTime, err := parseOptionalTime(input.EndTime)
	if err != nil {
		opErr = err
		return AuditSummaryOutput{BaseOutput: makeError(nbderrors.CodeQuerySyntaxError, err.Error())}
	}

	filter := audit.SummaryFilter{
		StartTime:  startTime,
		EndTime:    endTime,
		Datasource: input.Datasource,
		EventType:  input.EventType,
		Status:     input.Status,
		GroupBy:    input.GroupBy,
		Limit:      input.Limit,
	}

	rows, err := h.deps.Audit.Summary(filter)
	if err != nil {
		opErr = err
		return AuditSummaryOutput{BaseOutput: makeError(nbderrors.CodeInternalError, err.Error())}
	}
	topErrorFilter := filter
	topErrorFilter.Status = ""
	topErrors, err := h.deps.Audit.TopErrorSummaries(topErrorFilter)
	if err != nil {
		opErr = err
		return AuditSummaryOutput{BaseOutput: makeError(nbderrors.CodeInternalError, err.Error())}
	}
	confirmationFilter := audit.SummaryFilter{
		StartTime:  startTime,
		EndTime:    endTime,
		Datasource: input.Datasource,
		Limit:      input.Limit,
	}
	confirmations, err := h.deps.Audit.ConfirmationSummary(confirmationFilter)
	if err != nil {
		opErr = err
		return AuditSummaryOutput{BaseOutput: makeError(nbderrors.CodeInternalError, err.Error())}
	}

	out := make([]AuditSummaryRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, AuditSummaryRow{
			Bucket:       row.Bucket,
			EventType:    row.EventType,
			Datasource:   row.Datasource,
			Status:       row.Status,
			ErrorCode:    row.ErrorCode,
			Count:        row.Count,
			Slow1sCount:  row.Slow1sCount,
			Slow5sCount:  row.Slow5sCount,
			Slow10sCount: row.Slow10sCount,
		})
	}
	top := make([]AuditTopErrorSummary, 0, len(topErrors))
	for _, row := range topErrors {
		top = append(top, AuditTopErrorSummary{
			ErrorCode: row.ErrorCode,
			Summary:   row.Summary,
			Count:     row.Count,
		})
	}
	confirmationOut := make([]AuditConfirmationSummaryOutput, 0, len(confirmations))
	for _, row := range confirmations {
		confirmationOut = append(confirmationOut, AuditConfirmationSummaryOutput{
			Kind:       row.Kind,
			Datasource: row.Datasource,
			Status:     row.Status,
			Count:      row.Count,
			ErrorCount: row.ErrorCount,
		})
	}
	return AuditSummaryOutput{
		BaseOutput:          BaseOutput{OK: true},
		Rows:                out,
		TopErrors:           top,
		ConfirmationSummary: confirmationOut,
	}
}

func parseOptionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("time %q must be RFC3339", value)
	}
	return parsed, nil
}

func isAllowedAuditSummaryGroupBy(value string) bool {
	switch value {
	case "", "day", "event_type", "datasource", "status", "error_code":
		return true
	default:
		return false
	}
}
```

Add imports for `audit` and `time` if not already present.

Filter semantics: `rows` uses the caller's `status`, `event_type`, `datasource`, and time filters. `top_errors` clears `status` and defaults to failed audit events while retaining `event_type`, `datasource`, and time filters. `confirmation_summary` only inherits `datasource` and time filters because confirmation records use confirmation statuses (`executed`, `failed`, `expired`, `cancelled`, `pending`) and do not have an `event_type` column.

- [ ] **Step 6: Add handler output test**

Add to `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`:

```go
func TestAuditSummaryIncludesTopErrorsAndConfirmations(t *testing.T) {
	h := newTestHandlers(t)
	now := time.Now().UTC()
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
```

Add `native-db-bridge-mcp/internal/audit` to test imports if not already present.

- [ ] **Step 7: Wire MCP dispatcher**

In `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`, add dispatcher, input schema, and description for `audit_summary`:

```go
"audit_summary": func(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var input tools.AuditSummaryInput
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	return h.AuditSummary(ctx, input), nil
},
```

```go
"audit_summary": inputTypeOf[tools.AuditSummaryInput](),
```

```go
"audit_summary": "Summarize audit events by time, datasource, status, event type, or error code.",
```

- [ ] **Step 8: Run audit/tool/server tests**

Run:

```bash
go test ./internal/audit -run TestAuditSummaryByErrorCode -v
go test ./internal/tools -run TestAuditSummaryIncludesTopErrorsAndConfirmations -v
go test ./internal/audit -v
go test ./internal/tools -v
go test ./internal/server -v
```

Expected: PASS.

- [ ] **Step 9: Commit Task 7**

After explicit user confirmation for committing, run:

```bash
git add internal/audit/store.go internal/audit/store_test.go internal/tools/schema.go internal/tools/schema_test.go internal/tools/handlers_execution.go internal/tools/handlers_test.go internal/server/mcp.go
git commit -m "feat(audit): 增加审计摘要工具" \
  -m "改动背景：native-db-bridge 使用摩擦复盘此前需要手写 SQLite join，无法直接通过 MCP 获取聚合结果。" \
  -m "改动动机：提供 audit_summary，让 agent 能按时间、数据源、状态和错误码复盘失败率与慢查询分布。" \
  -m "关键决策：P1 不扩展 audit schema，直接基于 audit_events 和 confirmations 聚合，时间输入使用 RFC3339。" \
  -m "影响范围：新增 audit store summary、top errors、confirmation summary 查询和 MCP 工具，不影响既有 audit_recent。" \
  -m "验证方式：go test ./internal/audit -v；go test ./internal/tools -v；go test ./internal/server -v。"
```

## Task 8: README Workflow Documentation and Final Verification

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/README.md`
- Modify: `/Users/repairman/opt/native-db-bridge/README.en.md`

- [ ] **Step 1: Add README MySQL workflow section**

In `/Users/repairman/opt/native-db-bridge/README.md`, add this section after MCP 工具列表:

```markdown
## Agent MySQL 查询流程

推荐 agent 查询 MySQL 时按以下顺序使用工具：

1. `datasource_list`：确认目标数据源和默认库。
2. `sql_column_search`：按表名、字段名或类型搜索真实字段，避免直接猜列名。
3. `sql_text_column_plan`：需要搜索 URL、图片、文本内容时先生成候选文本列和扫描批次。
4. `sql_text_scan`：对确认过的字段执行 count-only 扫描。
5. `sql_query`：在字段和范围明确后查询明细。
6. `audit_summary`：任务结束后按数据源、工具、状态或错误码复盘。

常见错误处理：

- `SQL_UNKNOWN_COLUMN`：先用 `sql_column_search` 搜字段。
- `SQL_UNKNOWN_TABLE`：先用 `sql_object_list` 搜表。
- `QUERY_SYNTAX_ERROR`：修正 SQL 语法，不要直接重试。
- `QUERY_TIMEOUT`：缩小条件，或先用 `sql_text_column_plan` 和 `sql_text_scan` 拆分扫描。
```

- [ ] **Step 2: Update tool count and tables**

Update README tool tables to include:

```markdown
| `sql_column_search` | 搜索 MySQL 字段元数据 |
| `sql_text_column_plan` | 生成文本列扫描计划，不扫描业务数据 |
| `sql_text_scan` | 对指定文本字段执行 count-only 扫描 |
| `audit_summary` | 聚合审计事件，复盘失败率和慢查询 |
```

Make the matching additions in `/Users/repairman/opt/native-db-bridge/README.en.md`:

```markdown
| `sql_column_search` | Search MySQL column metadata |
| `sql_text_column_plan` | Build a text-column scan plan without reading business rows |
| `sql_text_scan` | Run count-only scans on selected text columns |
| `audit_summary` | Summarize audit events for failures and slow queries |
```

- [ ] **Step 3: Run full tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Build binary**

Run:

```bash
go build -o native-db-bridge-mcp ./cmd/native-db-bridge-mcp
```

Expected: PASS and binary created at `/Users/repairman/opt/native-db-bridge/native-db-bridge-mcp`.

- [ ] **Step 5: Optional live MCP smoke test**

Only run this if the local service can be restarted safely in the execution session.

Run:

```bash
./native-db-bridge-mcp healthcheck
```

Expected: PASS configuration checks.

Then call the MCP tools from Codex in a fresh session or after service reload:

- `datasource_list`
- `sql_column_search` against `saas_support`
- `sql_text_column_plan` against `saas_support`
- `audit_summary` for the last 7 days

Expected: all return `ok=true` without changing databases.

- [ ] **Step 6: Commit Task 8**

After explicit user confirmation for committing, run:

```bash
git add README.md README.en.md
git commit -m "docs(mysql): 补充 agent 查询工作流说明" \
  -m "改动背景：新增 MySQL 查询辅助工具后，需要明确推荐工具顺序和常见错误处理方式。" \
  -m "改动动机：让后续 agent 先做 schema 发现和受控扫描，再执行明细查询，减少无效 SQL。" \
  -m "关键决策：README 只描述使用流程和工具用途，不写生产或远程共享场景。" \
  -m "影响范围：仅文档更新，不提交本地构建产物 native-db-bridge-mcp。" \
  -m "验证方式：go test ./...；go build -o native-db-bridge-mcp ./cmd/native-db-bridge-mcp。"
```

## Final Acceptance Checklist

- [ ] `go test ./...` passes.
- [ ] `go build -o native-db-bridge-mcp ./cmd/native-db-bridge-mcp` passes.
- [ ] `sql_query` no longer fails on `SHOW TABLES;` due to `ndb_limited`.
- [ ] UTF-8 text columns return readable strings in JSON.
- [ ] `SQL_UNKNOWN_COLUMN`, `SQL_UNKNOWN_TABLE`, `QUERY_SYNTAX_ERROR`, and `QUERY_TIMEOUT` appear in tool output and audit operation records.
- [ ] SQL `execute_confirmation` failures reuse the same MySQL error classifier as `sql_query`.
- [ ] `sql_column_search`, `sql_text_column_plan`, `sql_text_scan`, and `audit_summary` appear in MCP `tools/list`.
- [ ] README explains the recommended agent MySQL workflow, including permission-limited system diagnostics alternatives.
