# Native DB Bridge MCP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个 Go 实现的本机 Streamable HTTP MCP 服务，用低资源常驻进程替代开发/测试环境 DataGrip MCP 的 SQL、Redis、MongoDB 操作入口。

**Architecture:** 服务按 `config -> audit -> policy -> lifecycle -> backend -> tools -> server` 分层。所有业务数据源懒加载，所有写操作通过 SQLite confirmation 两阶段确认，MCP 层只暴露工具契约，不直接执行写操作。

**Tech Stack:** Go 1.23+、官方 MCP Go SDK、`modernc.org/sqlite`、`gopkg.in/yaml.v3`、`github.com/go-sql-driver/mysql`、`github.com/redis/go-redis/v9`、`go.mongodb.org/mongo-driver/mongo`、`github.com/xwb1989/sqlparser`、macOS launchd。

---

## 参考文档

- 设计文档：`/Users/repairman/opt/native-db-bridge/docs/superpowers/specs/2026-06-05-native-db-bridge-mcp-design.md`
- 实现阶段参考 ECC Go skill：`https://github.com/affaan-m/ECC/tree/main/skills`
- Go MCP SDK 以官方仓库为准，封装在 `internal/server`，不要让 SDK 类型渗透到 policy/backend。

## 文件结构

```text
/Users/repairman/opt/native-db-bridge/
  cmd/native-db-bridge-mcp/main.go
  configs/config.example.yaml
  docs/superpowers/plans/2026-06-05-native-db-bridge-mcp-implementation.md
  docs/superpowers/specs/2026-06-05-native-db-bridge-mcp-design.md
  internal/app/app.go
  internal/audit/migrations.go
  internal/audit/store.go
  internal/audit/store_test.go
  internal/backend/backend.go
  internal/backend/mongo.go
  internal/backend/redis.go
  internal/backend/sql.go
  internal/cli/commands.go
  internal/config/config.go
  internal/config/config_test.go
  internal/config/types.go
  internal/lifecycle/manager.go
  internal/lifecycle/manager_test.go
  internal/model/model.go
  internal/nbderrors/errors.go
  internal/ops/tracker.go
  internal/ops/tracker_test.go
  internal/policy/mongo.go
  internal/policy/mongo_test.go
  internal/policy/redis.go
  internal/policy/redis_test.go
  internal/policy/sql.go
  internal/policy/sql_test.go
  internal/server/auth.go
  internal/server/http.go
  internal/server/mcp.go
  internal/tools/handlers.go
  internal/tools/handlers_test.go
  internal/tools/schema.go
  internal/tools/schema_test.go
  testdata/config/valid.yaml
  testdata/config/invalid-prod.yaml
  .gitignore
  go.mod
```

职责：

- `internal/config`：加载 YAML、解析 duration、路径、权限、数据源校验。
- `internal/audit`：SQLite schema migration、confirmation、audit event、operation 状态。
- `internal/policy`：SQL/Redis/Mongo 读写分类、安全白名单、风险摘要。
- `internal/lifecycle`：懒加载、in-flight 计数、空闲关闭。
- `internal/backend`：SQL、Redis、Mongo 后端接口和真实驱动实现。
- `internal/tools`：MCP 工具入参/出参结构和 handler，使用 backend/policy/audit。
- `internal/server`：Streamable HTTP MCP、鉴权、SDK 适配。
- `internal/cli`：`serve`、`healthcheck`、`install-service`、`uninstall-service`。

## Task 1: Go 项目骨架与基础命令

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/go.mod`
- Create: `/Users/repairman/opt/native-db-bridge/cmd/native-db-bridge-mcp/main.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/cli/commands.go`
- Create: `/Users/repairman/opt/native-db-bridge/.gitignore`
- Create: `/Users/repairman/opt/native-db-bridge/configs/config.example.yaml`

- [ ] **Step 1: 写最小 CLI 骨架测试**

Create `/Users/repairman/opt/native-db-bridge/internal/cli/commands_test.go`:

```go
package cli

import "testing"

func TestCommandNames(t *testing.T) {
	got := CommandNames()
	want := []string{"serve", "healthcheck", "install-service", "uninstall-service"}
	if len(got) != len(want) {
		t.Fatalf("len(CommandNames())=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CommandNames()[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}
```

- [ ] **Step 2: 初始化 module 并运行失败测试**

Run:

```bash
cd /Users/repairman/opt/native-db-bridge
go mod init native-db-bridge-mcp
go test ./internal/cli -run TestCommandNames -v
```

Expected: FAIL，提示 `undefined: CommandNames`。

- [ ] **Step 3: 实现最小 CLI**

Create `/Users/repairman/opt/native-db-bridge/internal/cli/commands.go`:

```go
package cli

func CommandNames() []string {
	return []string{"serve", "healthcheck", "install-service", "uninstall-service"}
}
```

Create `/Users/repairman/opt/native-db-bridge/cmd/native-db-bridge-mcp/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"native-db-bridge-mcp/internal/cli"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: native-db-bridge-mcp <%v>\n", cli.CommandNames())
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve", "healthcheck", "install-service", "uninstall-service":
		fmt.Printf("%s is not implemented in this task\n", os.Args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}
```

Create `/Users/repairman/opt/native-db-bridge/.gitignore`:

```gitignore
/var/
/native-db-bridge-mcp
/coverage.out
```

Create `/Users/repairman/opt/native-db-bridge/configs/config.example.yaml` using the YAML shape from the design doc, with fake hostnames and `client_token: "replace-with-openssl-rand-hex-32"`.

- [ ] **Step 4: 验证**

Run:

```bash
go test ./internal/cli -run TestCommandNames -v
go test ./...
go run ./cmd/native-db-bridge-mcp
```

Expected:

- tests PASS。
- `go run` exits code 2 and prints usage.

- [ ] **Step 5: 提交**

```bash
git add go.mod .gitignore cmd internal configs
git commit -m "chore(project): 初始化 Go 项目骨架" \
  -m "改动背景：native-db-bridge-mcp 需要 Go 项目基础结构承载后续开发。" \
  -m "验证方式：go test ./...；go run ./cmd/native-db-bridge-mcp。"
```

## Task 2: 配置模型、路径和权限校验

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/config/types.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/config/config.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/config/config_test.go`
- Create: `/Users/repairman/opt/native-db-bridge/testdata/config/valid.yaml`
- Create: `/Users/repairman/opt/native-db-bridge/testdata/config/invalid-prod.yaml`

- [ ] **Step 1: 写配置测试**

Create `/Users/repairman/opt/native-db-bridge/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "testdata", "config", "valid.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Transport != "streamable_http" {
		t.Fatalf("transport=%q", cfg.Server.Transport)
	}
	if cfg.Policy.ProductionEnabled {
		t.Fatalf("production must be disabled")
	}
	if len(cfg.Datasources.SQL) != 2 {
		t.Fatalf("sql datasources=%d, want 2", len(cfg.Datasources.SQL))
	}
}

func TestRejectProductionDatasource(t *testing.T) {
	_, err := Load(filepath.Join("..", "..", "testdata", "config", "invalid-prod.yaml"))
	if err == nil {
		t.Fatalf("Load() expected production datasource error")
	}
}

func TestRejectTooOpenConfigPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "valid.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	_, err = Load(path)
	if err == nil {
		t.Fatalf("Load() expected permission error")
	}
}

func TestRejectGroupReadableConfigPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", "valid.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0640); err != nil {
		t.Fatal(err)
	}
	_, err = Load(path)
	if err == nil {
		t.Fatalf("Load() expected group-readable permission error")
	}
}
```

- [ ] **Step 2: 创建测试配置**

Create `/Users/repairman/opt/native-db-bridge/testdata/config/valid.yaml` with one dev SQL datasource, one support SQL datasource, one support Redis namespace, one support Mongo datasource, fake credentials, `production_enabled: false`, `confirm_all_writes: true`, `transport: streamable_http`.

Create `/Users/repairman/opt/native-db-bridge/testdata/config/invalid-prod.yaml` same shape but with one datasource `environment: prod`.

- [ ] **Step 3: 运行失败测试**

```bash
go test ./internal/config -v
```

Expected: FAIL，提示 `undefined: Load`。

- [ ] **Step 4: 实现配置类型和加载**

Create `/Users/repairman/opt/native-db-bridge/internal/config/types.go`:

```go
package config

import "time"

type Config struct {
	Server              ServerConfig              `yaml:"server"`
	Policy              PolicyConfig              `yaml:"policy"`
	ConnectionLifecycle ConnectionLifecycleConfig `yaml:"connection_lifecycle"`
	Storage             StorageConfig             `yaml:"storage"`
	Connections         ConnectionsConfig         `yaml:"connections"`
	Datasources         DatasourcesConfig         `yaml:"datasources"`
}

type ServerConfig struct {
	Listen            string        `yaml:"listen"`
	MCPPath           string        `yaml:"mcp_path"`
	Transport         string        `yaml:"transport"`
	ClientToken       string        `yaml:"client_token"`
	RequestTimeout    time.Duration `yaml:"request_timeout"`
	QueryTimeout      time.Duration `yaml:"query_timeout"`
	MaxResultRows     int           `yaml:"max_result_rows"`
	RedisScanCountMax int           `yaml:"redis_scan_count_max"`
}

type PolicyConfig struct {
	AllowedEnvironments            []string      `yaml:"allowed_environments"`
	ProductionEnabled              bool          `yaml:"production_enabled"`
	ConfirmAllWrites               bool          `yaml:"confirm_all_writes"`
	RejectWriteWithoutConfirmation bool          `yaml:"reject_write_without_confirmation"`
	ConfirmationTTL                time.Duration `yaml:"confirmation_ttl"`
	ConfirmationExpireScanInterval time.Duration `yaml:"confirmation_expire_scan_interval"`
}

type ConnectionLifecycleConfig struct {
	Defaults LifecycleDefaults `yaml:"defaults"`
	SQL      LifecycleOverride `yaml:"sql"`
	Redis    LifecycleOverride `yaml:"redis"`
	Mongo    LifecycleOverride `yaml:"mongo"`
}

type LifecycleDefaults struct {
	LazyConnect       bool          `yaml:"lazy_connect"`
	IdleTTL           time.Duration `yaml:"idle_ttl"`
	CloseScanInterval time.Duration `yaml:"close_scan_interval"`
	ConnectTimeout    time.Duration `yaml:"connect_timeout"`
}

type LifecycleOverride struct {
	IdleTTL time.Duration `yaml:"idle_ttl"`
}

type StorageConfig struct {
	SQLitePath string `yaml:"sqlite_path"`
	LogPath    string `yaml:"log_path"`
}

type ConnectionsConfig struct {
	SQL   []SQLConnectionConfig   `yaml:"sql"`
	Redis []RedisConnectionConfig `yaml:"redis"`
	Mongo []MongoConnectionConfig `yaml:"mongo"`
}

type SQLConnectionConfig struct {
	Name        string     `yaml:"name"`
	Environment string     `yaml:"environment"`
	Driver      string     `yaml:"driver"`
	DSN         string     `yaml:"dsn"`
	Pool        SQLPoolCfg `yaml:"pool"`
}

type SQLPoolCfg struct {
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type RedisConnectionConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	Address     string `yaml:"address"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	TLS         bool   `yaml:"tls"`
}

type MongoConnectionConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	URI         string `yaml:"uri"`
}

type DatasourcesConfig struct {
	SQL   []SQLDatasourceConfig   `yaml:"sql"`
	Redis []RedisDatasourceConfig `yaml:"redis"`
	Mongo []MongoDatasourceConfig `yaml:"mongo"`
}

type SQLDatasourceConfig struct {
	Name            string `yaml:"name"`
	Environment     string `yaml:"environment"`
	Connection      string `yaml:"connection"`
	DefaultDatabase string `yaml:"default_database"`
}

type RedisDatasourceConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	Connection  string `yaml:"connection"`
	DB          int    `yaml:"db"`
	Service     string `yaml:"service"`
}

type MongoDatasourceConfig struct {
	Name            string `yaml:"name"`
	Environment     string `yaml:"environment"`
	Connection      string `yaml:"connection"`
	DefaultDatabase string `yaml:"default_database"`
}
```

Implement `/Users/repairman/opt/native-db-bridge/internal/config/config.go` with YAML duration parsing through a custom duration type or `yaml.v3` decode hook. Validate:

- config permission max `0600`
- `transport == "streamable_http"`
- `production_enabled == false`
- datasource environments only `dev` or `support`
- SQL DSN does not contain `multiStatements=true`
- referenced connection names exist

- [ ] **Step 5: 验证**

```bash
go get gopkg.in/yaml.v3
go test ./internal/config -v
go test ./...
```

Expected: PASS.

- [ ] **Step 6: 提交**

```bash
git add internal/config testdata/config go.mod go.sum
git commit -m "feat(config): 加载并校验固定全局数据源配置" \
  -m "改动背景：服务需要从 var/config.yaml 加载固定 SQL Redis Mongo 数据源。" \
  -m "验证方式：go test ./internal/config -v；go test ./..."
```

## Task 3: 结构化错误模型

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors_test.go`

- [ ] **Step 1: 写错误模型测试**

Create `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors_test.go`:

```go
package nbderrors

import "testing"

func TestErrorShape(t *testing.T) {
	err := New(CodePolicyRedisSelectRejected, "Redis SELECT is rejected").WithDatasource("saas-auth-support")
	if err.Code != CodePolicyRedisSelectRejected {
		t.Fatalf("code=%s", err.Code)
	}
	if err.Category != CategoryPolicy {
		t.Fatalf("category=%s", err.Category)
	}
	if err.Datasource != "saas-auth-support" {
		t.Fatalf("datasource=%s", err.Datasource)
	}
	if err.Retryable {
		t.Fatalf("policy errors must not be retryable")
	}
}
```

- [ ] **Step 2: 运行失败测试**

```bash
go test ./internal/nbderrors -v
```

Expected: FAIL，提示 `undefined: New`。

- [ ] **Step 3: 实现错误模型**

Create `/Users/repairman/opt/native-db-bridge/internal/nbderrors/errors.go`:

```go
package nbderrors

type Category string
type Code string

const (
	CategoryConfig     Category = "config"
	CategoryPolicy     Category = "policy"
	CategoryConnection Category = "connection"
	CategorySyntax     Category = "syntax"
	CategoryTimeout    Category = "timeout"
	CategoryDriver     Category = "driver"
	CategoryInternal   Category = "internal"
)

const (
	CodeConfigFileNotFound             Code = "CONFIG_FILE_NOT_FOUND"
	CodeConfigPermissionTooOpen        Code = "CONFIG_PERMISSION_TOO_OPEN"
	CodeConfigDatasourceNotFound       Code = "CONFIG_DATASOURCE_NOT_FOUND"
	CodeConfigProductionRejected       Code = "CONFIG_PRODUCTION_DATASOURCE_REJECTED"
	CodeAuthMissingToken               Code = "AUTH_MISSING_TOKEN"
	CodeAuthInvalidToken               Code = "AUTH_INVALID_TOKEN"
	CodePolicyWriteRequiresConfirm     Code = "POLICY_WRITE_REQUIRES_CONFIRMATION"
	CodePolicyProductionRejected       Code = "POLICY_PRODUCTION_REJECTED"
	CodePolicyRedisSelectRejected      Code = "POLICY_REDIS_SELECT_REJECTED"
	CodePolicyReadonlyToolRejectedWrite Code = "POLICY_READONLY_TOOL_REJECTED_WRITE"
	CodeConnectionFailed               Code = "CONNECTION_FAILED"
	CodeConnectionAuthFailed           Code = "CONNECTION_AUTH_FAILED"
	CodeQueryTimeout                   Code = "QUERY_TIMEOUT"
	CodeQuerySyntaxError               Code = "QUERY_SYNTAX_ERROR"
	CodeQueryLockingReadRejected       Code = "QUERY_LOCKING_READ_REJECTED"
	CodeConfirmationNotFound           Code = "CONFIRMATION_NOT_FOUND"
	CodeConfirmationExpired            Code = "CONFIRMATION_EXPIRED"
	CodeConfirmationAlreadyExecuted    Code = "CONFIRMATION_ALREADY_EXECUTED"
	CodeConfirmationInvalidState       Code = "CONFIRMATION_INVALID_STATE"
	CodeOperationNotFound              Code = "OPERATION_NOT_FOUND"
	CodeOperationNotCancelable         Code = "OPERATION_NOT_CANCELABLE"
	CodeDriverError                    Code = "DRIVER_ERROR"
	CodeInternalError                  Code = "INTERNAL_ERROR"
)

type Error struct {
	Code        Code                   `json:"code"`
	Category    Category               `json:"category"`
	Message     string                 `json:"message"`
	Datasource  string                 `json:"datasource,omitempty"`
	OperationID string                 `json:"operation_id,omitempty"`
	Retryable   bool                   `json:"retryable"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

func New(code Code, message string) *Error {
	return &Error{Code: code, Category: categoryFor(code), Message: message, Retryable: retryableFor(code)}
}

func (e *Error) WithDatasource(ds string) *Error { e.Datasource = ds; return e }

func categoryFor(code Code) Category {
	switch code {
	case CodeConfigFileNotFound, CodeConfigPermissionTooOpen, CodeConfigDatasourceNotFound, CodeConfigProductionRejected:
		return CategoryConfig
	case CodeConnectionFailed, CodeConnectionAuthFailed:
		return CategoryConnection
	case CodeQuerySyntaxError:
		return CategorySyntax
	case CodeQueryTimeout:
		return CategoryTimeout
	case CodeDriverError:
		return CategoryDriver
	case CodeInternalError:
		return CategoryInternal
	default:
		return CategoryPolicy
	}
}

func retryableFor(code Code) bool {
	return code == CodeConnectionFailed || code == CodeQueryTimeout || code == CodeDriverError
}
```

- [ ] **Step 4: 验证**

```bash
go test ./internal/nbderrors -v
go test ./...
```

Expected: PASS.

- [ ] **Step 5: 提交**

```bash
git add internal/nbderrors
git commit -m "feat(errors): 定义结构化错误模型" \
  -m "改动背景：MCP 工具需要统一返回可解析错误。" \
  -m "验证方式：go test ./internal/nbderrors -v；go test ./..."
```

## Task 4: SQLite audit store 与 migration

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/audit/migrations.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/audit/store.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/audit/store_test.go`

- [ ] **Step 1: 写 SQLite store 测试**

Create `/Users/repairman/opt/native-db-bridge/internal/audit/store_test.go`:

```go
package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenCreatesDatabaseAndMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.db")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error=%v", err)
	}
	defer store.Close()
	if err := store.CheckSchema(); err != nil {
		t.Fatalf("CheckSchema() error=%v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("audit.db mode=%#o, want 0600", got)
	}
}

func TestConfirmationLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	conf := Confirmation{
		ID:          "conf_test",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		PayloadJSON: `{"sql":"UPDATE t SET a=1 WHERE id=1"}`,
		PayloadHash: "hash",
		Summary:     "UPDATE t ...",
		RiskLevel:   "medium",
		ImpactJSON:  `{"mode":"estimated","rows":1}`,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	if err := store.CreateConfirmation(conf); err != nil {
		t.Fatalf("CreateConfirmation() error=%v", err)
	}
	got, err := store.GetConfirmation("conf_test")
	if err != nil {
		t.Fatalf("GetConfirmation() error=%v", err)
	}
	if got.Summary != conf.Summary {
		t.Fatalf("summary=%q", got.Summary)
	}
}

func TestExecuteConfirmationCanWinOnlyOnce(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	conf := Confirmation{
		ID:          "conf_race",
		Kind:        "sql_dml",
		Datasource:  "saas_support",
		PayloadJSON: `{"sql":"UPDATE t SET a=1 WHERE id=1"}`,
		PayloadHash: "hash",
		Summary:     "UPDATE t ...",
		RiskLevel:   "medium",
		ImpactJSON:  `{"mode":"estimated","rows":1}`,
		Status:      "pending",
		ExpiresAt:   time.Now().Add(time.Minute),
	}
	if err := store.CreateConfirmation(conf); err != nil {
		t.Fatal(err)
	}
	const workers = 8
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			results <- store.MarkConfirmationExecuting("conf_race")
		}()
	}
	success := 0
	for i := 0; i < workers; i++ {
		if err := <-results; err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("success=%d, want 1", success)
	}
}
```

- [ ] **Step 2: 运行失败测试**

```bash
go test ./internal/audit -v
```

Expected: FAIL，提示 `undefined: Open`。

- [ ] **Step 3: 实现 migration 和 store**

Implement `Open(path)` using `database/sql` and `_ "modernc.org/sqlite"`.

Migration SQL must create:

```sql
CREATE TABLE IF NOT EXISTS confirmations (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  datasource TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  payload_hash TEXT NOT NULL,
  summary TEXT NOT NULL,
  risk_level TEXT NOT NULL,
  impact_json TEXT NOT NULL,
  status TEXT NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  executed_at TIMESTAMP NULL,
  error_summary TEXT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  datasource TEXT NOT NULL,
  operation_id TEXT NULL,
  confirmation_id TEXT NULL,
  summary TEXT NOT NULL,
  status TEXT NOT NULL,
  elapsed_ms INTEGER NOT NULL DEFAULT 0,
  row_count INTEGER NOT NULL DEFAULT 0,
  error_code TEXT NULL,
  created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS operations (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  datasource TEXT NOT NULL,
  status TEXT NOT NULL,
  confirmation_id TEXT NULL,
  started_at TIMESTAMP NOT NULL,
  finished_at TIMESTAMP NULL,
  cancel_requested_at TIMESTAMP NULL,
  error_code TEXT NULL,
  error_summary TEXT NULL
);
```

Create indexes for `created_at`, `datasource`, `confirmation_id`, `status`.

Open rules:

- create missing db by first calling `os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)`, closing that file, then opening it through `database/sql`
- if `sql.Open` or migration creates any file with broader permission, immediately `os.Chmod(path, 0600)` before returning from `Open`
- reject existing db if group/others bits are set
- run `PRAGMA integrity_check`
- enable WAL
- run migration

- [ ] **Step 4: 验证**

```bash
go get modernc.org/sqlite
go test ./internal/audit -v
go test ./...
```

Expected: PASS.

- [ ] **Step 5: 提交**

```bash
git add internal/audit go.mod go.sum
git commit -m "feat(audit): 增加 SQLite 审计与确认存储" \
  -m "改动背景：确认状态和审计事件需要本地 SQLite 作为 source of truth。" \
  -m "验证方式：go test ./internal/audit -v；go test ./..."
```

## Task 5: Policy 分类与安全白名单

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/policy/sql.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/policy/sql_test.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/policy/redis.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/policy/redis_test.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/policy/mongo.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/policy/mongo_test.go`

- [ ] **Step 1: 写 SQL policy 测试**

Use table tests:

```go
func TestClassifySQLQuery(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		read bool
	}{
		{"select", "SELECT * FROM t", true},
		{"show", "SHOW TABLES", true},
		{"desc", "DESC t", true},
		{"describe", "DESCRIBE t", true},
		{"explain", "EXPLAIN SELECT * FROM t", true},
		{"update", "UPDATE t SET a=1", false},
		{"multi", "SELECT 1; DROP TABLE t", false},
		{"for update", "SELECT * FROM t FOR UPDATE", false},
		{"lock in share mode", "SELECT * FROM t LOCK IN SHARE MODE", false},
		{"load file", "SELECT LOAD_FILE('/etc/passwd')", false},
		{"into outfile", "SELECT * FROM t INTO OUTFILE '/tmp/x'", false},
		{"into dumpfile", "SELECT * FROM t INTO DUMPFILE '/tmp/x'", false},
		{"use", "USE other_db", false},
		{"lock", "LOCK TABLES t WRITE", false},
		{"unlock", "UNLOCK TABLES", false},
		{"call", "CALL p()", false},
		{"set", "SET @a=1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSQLReadAllowed(tc.sql)
			if got != tc.read {
				t.Fatalf("IsSQLReadAllowed=%v, want %v", got, tc.read)
			}
		})
	}
}
```

- [ ] **Step 2: 写 Redis policy 测试**

```go
func TestRedisCommandPolicy(t *testing.T) {
	if !IsRedisReadAllowed("GET") {
		t.Fatalf("GET should be read")
	}
	if IsRedisReadAllowed("SET") {
		t.Fatalf("SET must not be read")
	}
	if !IsRedisAlwaysRejected("SELECT") {
		t.Fatalf("SELECT must be always rejected")
	}
	if !IsRedisAlwaysRejected("EVAL") {
		t.Fatalf("EVAL must be always rejected")
	}
}
```

- [ ] **Step 3: 写 Mongo policy 测试**

```go
func TestMongoAggregateStages(t *testing.T) {
	allowed := []string{"$match", "$project", "$limit", "$skip", "$sort", "$group", "$count", "$unwind"}
	for _, stage := range allowed {
		if !IsMongoAggregateStageAllowed(stage) {
			t.Fatalf("%s should be allowed", stage)
		}
	}
	rejected := []string{"$out", "$merge", "$function", "$accumulator", "$where", "$graphLookup", "$lookup"}
	for _, stage := range rejected {
		if IsMongoAggregateStageAllowed(stage) {
			t.Fatalf("%s must be rejected", stage)
		}
	}
}

func TestMongoWriteMatrix(t *testing.T) {
	cases := []struct {
		name         string
		operation    string
		hasFilter    bool
		hasDocument  bool
		hasDocuments bool
		want         bool
	}{
		{"insertOne ok", "insertOne", false, true, false, true},
		{"insertOne rejects filter", "insertOne", true, true, false, false},
		{"insertMany ok", "insertMany", false, false, true, true},
		{"updateOne ok", "updateOne", true, true, false, true},
		{"deleteMany ok", "deleteMany", true, false, false, true},
		{"dropCollection ok", "dropCollection", false, false, false, true},
		{"dropCollection rejects filter", "dropCollection", true, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateMongoWrite(tc.operation, tc.hasFilter, tc.hasDocument, tc.hasDocuments)
			if got != tc.want {
				t.Fatalf("ValidateMongoWrite=%v, want %v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 4: 实现 policy**

Implement:

- `IsSQLReadAllowed(sql string) bool`
- `IsSQLWriteAllowed(sql string) (kind string, ok bool)`
- `IsRedisReadAllowed(command string) bool`
- `IsRedisWriteCommand(command string) bool`
- `IsRedisAlwaysRejected(command string) bool`
- `IsMongoAggregateStageAllowed(stage string) bool`
- `ValidateMongoWrite(operation string, hasFilter bool, hasDocument bool, hasDocuments bool) bool`

Use `github.com/xwb1989/sqlparser` to classify statements. If parser cannot parse a MySQL statement, return unsafe/rejected. Do not fall back to prefix matching.

- [ ] **Step 5: 验证**

```bash
go test ./internal/policy -v
go test ./...
```

Expected: PASS.

- [ ] **Step 6: 提交**

```bash
git add internal/policy go.mod go.sum
git commit -m "feat(policy): 实现数据库操作安全分类" \
  -m "改动背景：所有工具必须先经过读写和风险分类。" \
  -m "验证方式：go test ./internal/policy -v；go test ./..."
```

## Task 6: 连接生命周期管理

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/lifecycle/manager.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/lifecycle/manager_test.go`

- [ ] **Step 1: 写懒加载和空闲关闭测试**

```go
func TestManagerLazyLoadsAndClosesIdle(t *testing.T) {
	created := 0
	closed := 0
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		created++
		return ResourceFunc(func() error { closed++; return nil }), nil
	})
	ctx := context.Background()
	release, err := m.Acquire(ctx, "saas_support")
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("created=%d", created)
	}
	release()
	m.CloseIdle(time.Now().Add(2 * time.Minute))
	if closed != 1 {
		t.Fatalf("closed=%d", closed)
	}
}

func TestManagerDoesNotCloseInFlightResource(t *testing.T) {
	closed := 0
	m := NewManager[string](time.Minute, func(ctx context.Context, key string) (Resource, error) {
		return ResourceFunc(func() error { closed++; return nil }), nil
	})
	release, err := m.Acquire(context.Background(), "saas_support")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	m.CloseIdle(time.Now().Add(2 * time.Minute))
	if closed != 0 {
		t.Fatalf("closed in-flight resource")
	}
}
```

- [ ] **Step 2: 实现 Manager**

Implement generic manager with:

- mutex
- map key to entry
- `inFlight`
- `lastUsed`
- `Acquire(ctx,key) (release func(), err error)`
- `CloseIdle(now time.Time)`

Do not close if `inFlight > 0`. Double-check under lock before closing.

- [ ] **Step 3: 验证**

```bash
go test ./internal/lifecycle -race -v
go test ./...
```

Expected: PASS.

- [ ] **Step 4: 提交**

```bash
git add internal/lifecycle
git commit -m "feat(lifecycle): 增加懒加载连接生命周期管理" \
  -m "改动背景：业务数据源连接需要按需创建并空闲回收。" \
  -m "验证方式：go test ./internal/lifecycle -race -v；go test ./..."
```

## Task 7: Backend 接口与 fake 后端

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/backend/backend.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/backend/redis.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/backend/mongo.go`

- [ ] **Step 1: 定义接口**

Create interfaces:

```go
type SQLBackend interface {
	Ping(ctx context.Context, datasource string) error
	Query(ctx context.Context, datasource string, sql string, limit int) (SQLResult, error)
	Exec(ctx context.Context, datasource string, sql string) (ExecResult, error)
	PreviewTable(ctx context.Context, datasource, schema, table string, limit int) (SQLResult, error)
}

type RedisBackend interface {
	Ping(ctx context.Context, datasource string) error
	Command(ctx context.Context, datasource, command string, args []string) (RedisResult, error)
	ScanKeys(ctx context.Context, datasource, match, cursor string, count int) (RedisScanResult, error)
	KeyDescribe(ctx context.Context, datasource, key string) (RedisKeyDescription, error)
}

type MongoBackend interface {
	Ping(ctx context.Context, datasource string) error
	Find(ctx context.Context, req MongoFindRequest) (MongoResult, error)
	Write(ctx context.Context, req MongoWriteRequest) (ExecResult, error)
	ListDatabases(ctx context.Context, datasource string) ([]string, error)
	ListCollections(ctx context.Context, datasource, pattern string) ([]MongoCollection, error)
	DescribeCollection(ctx context.Context, datasource, collection string) (MongoCollectionDescription, error)
}
```

Result types must contain row/document/result summary, affected count, elapsed duration.

- [ ] **Step 2: 增加 fake 后端用于 handler 测试**

Implement in `_test.go` files later inside `internal/tools`, not production package.

- [ ] **Step 3: 验证**

```bash
go test ./internal/backend -v
go test ./...
```

Expected: PASS or `? native-db-bridge-mcp/internal/backend [no test files]`.

- [ ] **Step 4: 提交**

```bash
git add internal/backend
git commit -m "feat(backend): 定义数据库执行后端接口" \
  -m "改动背景：MCP 工具层需要与真实驱动解耦以便测试。" \
  -m "验证方式：go test ./..."
```

## Task 8: Tool schema 与 handler 纯单元测试

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/tools/schema.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/tools/schema_test.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/tools/handlers_test.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/ops/tracker.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/ops/tracker_test.go`

- [ ] **Step 1: 写 schema 测试**

Test every tool name is present:

```go
func TestToolSchemasIncludeRequiredTools(t *testing.T) {
	names := ToolNames()
	want := []string{
		"datasource_list", "datasource_healthcheck",
		"sql_schema_list", "sql_object_type_list", "sql_object_list", "sql_object_describe", "sql_table_preview",
		"redis_key_scan", "redis_key_describe",
		"mongo_database_list", "mongo_collection_list", "mongo_collection_describe",
		"sql_query", "sql_prepare_change", "redis_command", "redis_prepare_change", "mongo_find", "mongo_prepare_change", "execute_confirmation",
		"operation_list", "cancel_operation", "audit_recent", "confirmation_get", "cancel_confirmation",
	}
	for _, w := range want {
		if !slices.Contains(names, w) {
			t.Fatalf("missing tool %s", w)
		}
	}
}
```

- [ ] **Step 2: 写 handler 测试：写操作只 prepare**

```go
func TestSQLPrepareCreatesConfirmationWithoutExecuting(t *testing.T) {
	h := newTestHandlers(t)
	out, err := h.SQLPrepareChange(context.Background(), SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "UPDATE tc_org SET name='x' WHERE id=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ConfirmationID == "" {
		t.Fatalf("confirmation id empty")
	}
	if h.fakeSQL.execCalled {
		t.Fatalf("prepare must not execute SQL")
	}
}
```

Add tests for diagnostics and safety handlers:

```go
type testHandlers struct {
	*Handlers
	fakeSQL   *fakeSQLBackend
	fakeRedis *fakeRedisBackend
	fakeMongo *fakeMongoBackend
}

func newTestHandlers(t *testing.T) *testHandlers {
	t.Helper()
	store, err := audit.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.Config{
		Server: config.ServerConfig{QueryTimeout: 30 * time.Second, MaxResultRows: 1000, RedisScanCountMax: 500},
	}
	sqlBackend := &fakeSQLBackend{}
	redisBackend := &fakeRedisBackend{}
	mongoBackend := &fakeMongoBackend{}
	handlers := NewHandlers(Deps{
		Config: cfg,
		Audit: store,
		SQL:   sqlBackend,
		Redis: redisBackend,
		Mongo: mongoBackend,
		Ops:   ops.NewTracker(),
	})
	return &testHandlers{Handlers: handlers, fakeSQL: sqlBackend, fakeRedis: redisBackend, fakeMongo: mongoBackend}
}

func TestCancelConfirmationOnlyCancelsPending(t *testing.T) {
	h := newTestHandlers(t)
	out, err := h.SQLPrepareChange(context.Background(), SQLPrepareChangeInput{
		Datasource: "saas_support",
		SQL:        "UPDATE tc_org SET name='x' WHERE id=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := h.CancelConfirmation(context.Background(), CancelConfirmationInput{ConfirmationID: out.ConfirmationID})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("status=%s", cancelled.Status)
	}
	if _, err := h.ExecuteConfirmation(context.Background(), ExecuteConfirmationInput{ConfirmationID: out.ConfirmationID}); err == nil {
		t.Fatalf("cancelled confirmation must not execute")
	}
}

func TestDatasourceListFiltersByTypeAndEnvironment(t *testing.T) {
	h := newTestHandlers(t)
	out, err := h.DatasourceList(context.Background(), DatasourceListInput{Type: "redis", Environment: "support"})
	if err != nil {
		t.Fatal(err)
	}
	for _, ds := range out.Datasources {
		if ds.Type != "redis" || ds.Environment != "support" {
			t.Fatalf("unexpected datasource %+v", ds)
		}
	}
}

func TestAuditRecentFiltersByDatasource(t *testing.T) {
	h := newTestHandlers(t)
	if _, err := h.SQLQuery(context.Background(), SQLQueryInput{Datasource: "saas_support", SQL: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	out, err := h.AuditRecent(context.Background(), AuditRecentInput{Datasource: "saas_support", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) == 0 {
		t.Fatalf("expected audit events")
	}
}

func TestOperationListFiltersStatus(t *testing.T) {
	h := newTestHandlers(t)
	out, err := h.OperationList(context.Background(), OperationListInput{Status: "running", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range out.Operations {
		if op.Status != "running" {
			t.Fatalf("unexpected operation status %s", op.Status)
		}
	}
}

func TestRedisKeyScanCapsCount(t *testing.T) {
	h := newTestHandlers(t)
	out, err := h.RedisKeyScan(context.Background(), RedisKeyScanInput{
		Datasource: "saas-auth-support",
		Match:      "*",
		Count:      999999,
		Cursor:     "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.RequestedCount > h.Config.Server.RedisScanCountMax {
		t.Fatalf("scan count was not capped")
	}
}

func TestCancelOperationCallsTracker(t *testing.T) {
	h := newTestHandlers(t)
	ctx, cancel := context.WithCancel(context.Background())
	h.Ops.Register("op_cancel", cancel)
	out, err := h.CancelOperation(context.Background(), CancelOperationInput{OperationID: "op_cancel"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "cancel_requested" {
		t.Fatalf("status=%s", out.Status)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatalf("operation context was not cancelled")
	}
}

func TestCancelOperationRejectsMissingOperation(t *testing.T) {
	h := newTestHandlers(t)
	_, err := h.CancelOperation(context.Background(), CancelOperationInput{OperationID: "op_missing"})
	if err == nil {
		t.Fatalf("expected OPERATION_NOT_FOUND")
	}
}
```

- [ ] **Step 3: 实现工具 schema 与 handler**

Implement Go structs for every input/output in the design doc. Handler methods call policy first, then audit/backend.

Implement `/Users/repairman/opt/native-db-bridge/internal/ops/tracker.go`:

```go
package ops

import (
	"context"
	"sync"
)

type Tracker struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewTracker() *Tracker {
	return &Tracker{cancels: map[string]context.CancelFunc{}}
}

func (t *Tracker) Register(operationID string, cancel context.CancelFunc) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancels[operationID] = cancel
}

func (t *Tracker) Cancel(operationID string) bool {
	t.mu.Lock()
	cancel, ok := t.cancels[operationID]
	if ok {
		delete(t.cancels, operationID)
	}
	t.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (t *Tracker) Finish(operationID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cancels, operationID)
}
```

Create `/Users/repairman/opt/native-db-bridge/internal/ops/tracker_test.go`:

```go
package ops

import (
	"context"
	"testing"
)

func TestTrackerCancelCallsCancelFuncOnce(t *testing.T) {
	tracker := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	tracker.Register("op_1", cancel)
	if !tracker.Cancel("op_1") {
		t.Fatalf("first cancel should find operation")
	}
	if tracker.Cancel("op_1") {
		t.Fatalf("second cancel should not find operation")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatalf("context was not cancelled")
	}
}
```

Start with:

- `SQLQuery`
- `SQLPrepareChange`
- `ExecuteConfirmation`
- `CancelConfirmation`

Then add Redis and Mongo methods.

- [ ] **Step 4: 验证**

```bash
go test ./internal/tools -v
go test ./...
```

Expected: PASS.

- [ ] **Step 5: 提交**

```bash
git add internal/tools
git commit -m "feat(tools): 实现 MCP 工具契约和核心 handler" \
  -m "改动背景：MCP server 需要稳定工具 schema 和可测试 handler。" \
  -m "验证方式：go test ./internal/tools -v；go test ./..."
```

## Task 9: 真实 SQL / Redis / Mongo 后端

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/sql.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/redis.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/backend/mongo.go`

- [ ] **Step 1: 写后端构造测试**

Use config fixtures and verify constructors do not connect immediately:

```go
func TestSQLBackendDoesNotConnectOnCreate(t *testing.T) {
	b := NewSQLDriverBackend(testConfigWithInvalidHost())
	if b == nil {
		t.Fatalf("backend nil")
	}
}
```

- [ ] **Step 2: 实现 SQL backend**

Use `database/sql` and `github.com/go-sql-driver/mysql`.

Rules:

- one pool per SQL datasource
- DSN includes `default_database`
- lazy open via lifecycle manager
- set pool options from config
- queries use context timeout
- if caller omits limit, use `server.max_result_rows`
- if caller passes limit above `server.max_result_rows`, cap it
- apply limit by wrapping read SQL as `SELECT * FROM (<original>) AS ndb_limited LIMIT ?` when parser says the original query has no limit; if the original query already has a stricter limit, keep it
- never append raw user-provided limit text to SQL

- [ ] **Step 3: 实现 Redis backend**

Use `github.com/redis/go-redis/v9`.

Rules:

- one client per Redis datasource namespace
- client `DB` fixed to namespace `db`
- `SELECT` never sent
- lazy open via lifecycle manager
- `ScanKeys` uses Redis SCAN and caps count at `server.redis_scan_count_max`
- `KeyDescribe` combines EXISTS, TYPE, PTTL and type-specific length commands

- [ ] **Step 4: 实现 Mongo backend**

Use `go.mongodb.org/mongo-driver/mongo`.

Rules:

- one client per Mongo datasource
- database fixed to `default_database`
- no database override
- aggregate stage whitelist checked before backend call
- implement Mongo metadata methods with `ListCollectionNames`, `Indexes().List`, and a bounded sample query using `limit=20`
- `ListDatabases` returns datasource `default_database` in v1; it does not enumerate server-wide databases

- [ ] **Step 5: 验证**

```bash
go get github.com/go-sql-driver/mysql github.com/redis/go-redis/v9 go.mongodb.org/mongo-driver/mongo
go test ./internal/backend -v
go test ./...
```

Expected: PASS without real database connections in constructor tests.

- [ ] **Step 6: 提交**

```bash
git add internal/backend go.mod go.sum
git commit -m "feat(backend): 接入 SQL Redis Mongo 原生驱动" \
  -m "改动背景：执行层需要原生驱动访问开发测试数据源。" \
  -m "验证方式：go test ./internal/backend -v；go test ./..."
```

## Task 10: Streamable HTTP MCP server 与鉴权

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/server/auth.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/server/http.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/server/mcp.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/server/http_test.go`

- [ ] **Step 1: 写鉴权测试**

Before writing code, confirm the official Go MCP SDK version and Streamable HTTP support:

```bash
go list -m -versions github.com/modelcontextprotocol/go-sdk
go doc github.com/modelcontextprotocol/go-sdk/mcp
```

Expected: identify the current SDK module path and Streamable HTTP server API. If the official SDK module path or API differs, keep the difference inside `internal/server/mcp.go` and do not change policy/backend/tool interfaces.

```go
func TestBearerTokenAuth(t *testing.T) {
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	if !Authorized(req, "secret") {
		t.Fatalf("expected authorized")
	}
	req.Header.Set("Authorization", "Bearer wrong")
	if Authorized(req, "secret") {
		t.Fatalf("wrong token authorized")
	}
}
```

- [ ] **Step 2: 实现鉴权与 HTTP server**

Implement:

- only bind configured `127.0.0.1:port`
- reject missing/invalid token
- expose `/healthz`
- expose configured MCP path

Keep official MCP SDK usage inside `internal/server/mcp.go`.

- [ ] **Step 3: 注册工具**

Map each `internal/tools` handler to SDK tool registration. If SDK API changes, keep conversion in `server/mcp.go`; do not change policy/backend types.

- [ ] **Step 4: 验证**

```bash
go test ./internal/server -v
go test ./...
```

Expected: PASS.

- [ ] **Step 5: 提交**

```bash
git add internal/server go.mod go.sum
git commit -m "feat(server): 提供 Streamable HTTP MCP 服务" \
  -m "改动背景：多 Codex 会话需要共享同一个本机 MCP endpoint。" \
  -m "验证方式：go test ./internal/server -v；go test ./..."
```

## Task 11: 应用装配、CLI serve / healthcheck

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/internal/app/app.go`
- Modify: `/Users/repairman/opt/native-db-bridge/internal/cli/commands.go`
- Modify: `/Users/repairman/opt/native-db-bridge/cmd/native-db-bridge-mcp/main.go`

- [ ] **Step 1: 写 app 构造测试**

```go
func TestNewAppDoesNotConnectBusinessDatasources(t *testing.T) {
	app, err := NewForTest(validConfigPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if app.BusinessConnectionCount() != 0 {
		t.Fatalf("business connections opened on startup")
	}
}
```

- [ ] **Step 2: 实现 app wiring**

Create app that wires:

- config
- audit store
- policy
- backend
- tool handlers
- server

Startup initializes SQLite but does not connect SQL/Redis/Mongo.

- [ ] **Step 3: 实现 CLI**

Commands:

- `serve --home <path>`
- `healthcheck --home <path>`
- `healthcheck --home <path> --connect`

`--home` default resolution:

1. explicit flag
2. `NATIVE_DB_BRIDGE_HOME`
3. `./var`

- [ ] **Step 4: 验证**

```bash
go test ./internal/app ./internal/cli -v
go test ./...
go run ./cmd/native-db-bridge-mcp healthcheck --home ./testdata/runtime
```

Expected: tests PASS. Healthcheck returns config-level result; create `testdata/runtime/config.yaml` from fake config for this command.

- [ ] **Step 5: 提交**

```bash
git add cmd internal/app internal/cli testdata
git commit -m "feat(app): 装配服务并实现 CLI 入口" \
  -m "改动背景：需要可运行的 serve 和 healthcheck 命令。" \
  -m "验证方式：go test ./...；go run ./cmd/native-db-bridge-mcp healthcheck --home ./testdata/runtime。"
```

## Task 12: launchd 安装卸载和运行态目录

**Files:**
- Modify: `/Users/repairman/opt/native-db-bridge/internal/cli/commands.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/cli/launchd.go`
- Create: `/Users/repairman/opt/native-db-bridge/internal/cli/launchd_test.go`

- [ ] **Step 1: 写 plist 生成测试**

```go
func TestLaunchdPlistContainsHomeAndBinary(t *testing.T) {
	got := RenderLaunchdPlist("/bin/native-db-bridge-mcp", "/Users/repairman/opt/native-db-bridge/var")
	if !strings.Contains(got, "/bin/native-db-bridge-mcp") {
		t.Fatalf("binary missing")
	}
	if !strings.Contains(got, "/Users/repairman/opt/native-db-bridge/var") {
		t.Fatalf("home missing")
	}
	if strings.Contains(got, "password") {
		t.Fatalf("plist must not contain secrets")
	}
}
```

- [ ] **Step 2: 实现 install/uninstall**

Implement:

- plist path `~/Library/LaunchAgents/com.repairman.native-db-bridge.plist`
- `install-service --home <path> --bin <path>`
- `uninstall-service`
- no secrets in plist

- [ ] **Step 3: 验证**

```bash
go test ./internal/cli -run Launchd -v
go test ./...
```

Expected: PASS.

- [ ] **Step 4: 提交**

```bash
git add internal/cli
git commit -m "feat(cli): 增加 launchd 用户服务安装卸载" \
  -m "改动背景：服务默认以本机二进制和 launchd 常驻运行。" \
  -m "验证方式：go test ./internal/cli -run Launchd -v；go test ./..."
```

## Task 13: 集成测试与最终验收

**Files:**
- Create: `/Users/repairman/opt/native-db-bridge/docker-compose.test.yml`
- Create: `/Users/repairman/opt/native-db-bridge/internal/integration/integration_test.go`
- Modify: `/Users/repairman/opt/native-db-bridge/README.md`

- [ ] **Step 1: 添加 docker compose**

Create `/Users/repairman/opt/native-db-bridge/docker-compose.test.yml`:

```yaml
services:
  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: root
      MYSQL_DATABASE: ndb_test
      MYSQL_USER: ndb
      MYSQL_PASSWORD: ndb
    ports:
      - "33067:3306"
    command: ["--default-authentication-plugin=mysql_native_password"]

  redis:
    image: redis:7
    ports:
      - "6389:6379"

  mongo:
    image: mongo:7
    ports:
      - "27027:27017"
```

- [ ] **Step 2: 写真实集成测试入口**

Create `/Users/repairman/opt/native-db-bridge/internal/integration/integration_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestIntegrationSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	mysqlDB, err := sql.Open("mysql", "ndb:ndb@tcp(127.0.0.1:33067)/ndb_test?parseTime=true&charset=utf8mb4")
	if err != nil {
		t.Fatal(err)
	}
	defer mysqlDB.Close()
	var one int
	if err := mysqlDB.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("mysql SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("mysql returned %d", one)
	}

	redisDB0 := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6389", DB: 0})
	defer redisDB0.Close()
	redisDB1 := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6389", DB: 1})
	defer redisDB1.Close()
	if err := redisDB0.Set(ctx, "namespace-key", "db0", time.Minute).Err(); err != nil {
		t.Fatalf("redis db0 set: %v", err)
	}
	if got, err := redisDB1.Exists(ctx, "namespace-key").Result(); err != nil || got != 0 {
		t.Fatalf("redis namespace isolation exists=%d err=%v", got, err)
	}

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://127.0.0.1:27027"))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	defer mongoClient.Disconnect(ctx)
	if err := mongoClient.Ping(ctx, nil); err != nil {
		t.Fatalf("mongo ping: %v", err)
	}
}
```

- [ ] **Step 3: README 写运行命令**

Create `/Users/repairman/opt/native-db-bridge/README.md` with:

```markdown
# native-db-bridge-mcp

本机低资源数据库 MCP bridge。运行态文件放在 `var/`，真实配置不提交。

## 常用命令

```bash
go test ./...
go run ./cmd/native-db-bridge-mcp healthcheck --home ./var
go run ./cmd/native-db-bridge-mcp serve --home ./var
docker compose -f docker-compose.test.yml up -d
go test -tags=integration ./internal/integration
```
```

- [ ] **Step 4: 执行验收命令**

```bash
go test ./...
go test -race ./internal/lifecycle ./internal/tools ./internal/audit
docker compose -f docker-compose.test.yml up -d
go test -tags=integration ./internal/integration
```

Expected:

- unit tests PASS
- race tests PASS
- integration tests PASS when Docker services are running

- [ ] **Step 5: 提交**

```bash
git add docker-compose.test.yml internal/integration README.md
git commit -m "test(integration): 增加数据库桥接集成验收" \
  -m "改动背景：首版需要验证 MySQL Redis Mongo 和审计链路。" \
  -m "验证方式：go test ./...；go test -race ./internal/lifecycle ./internal/tools ./internal/audit；go test -tags=integration ./internal/integration。"
```

## Plan 自审

### Spec 覆盖

- Go 技术栈：Task 1。
- 本地运行和 launchd：Task 11、Task 12。
- `var/` 与 `.gitignore`：Task 1、Task 11。
- config 权限、固定全局数据源、生产拒绝：Task 2。
- SQLite `modernc.org/sqlite`、migration、权限、integrity check：Task 4。
- 结构化错误：Task 3。
- SQL/Redis/Mongo policy：Task 5。
- 懒加载和空闲回收：Task 6、Task 9。
- backend 接口和原生驱动：Task 7、Task 9。
- MCP tool schema 和 handler：Task 8。
- Streamable HTTP 和 token：Task 10。
- confirmation 状态机、取消、审计：Task 4、Task 8。
- operation tracker 和 cancel function 管理：Task 8。
- 集成测试和验收：Task 13。

### 占位符扫描

计划不保留空白章节、未定义函数名、临时跳过测试或要求执行者自行补齐的步骤。集成测试任务直接写真实 Docker 依赖和真实 smoke 测试。

### 类型一致性

统一命名：

- confirmation id 字段：`confirmation_id`
- operation id 字段：`operation_id`
- datasource 字段：`datasource`
- confirmation kind：`sql_dml`、`sql_ddl`、`redis_write`、`mongo_write`
- transport：`streamable_http`
