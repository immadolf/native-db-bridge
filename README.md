# native-db-bridge-mcp

[English](README.en.md)

本机常驻的 MCP 服务，替代 DataGrip MCP 作为开发/测试环境的数据库操作入口。

## 为什么

DataGrip MCP 依赖 DataGrip 常驻运行，带来额外内存、CPU 和 IDE 卡顿成本。当 agent 已经通过 MCP 访问数据库时，DataGrip 常驻的价值大幅降低。

本项目提供一个低资源、Go 实现的单进程 MCP 服务，支持多个 Codex/Claude 会话共享同一个本地实例。

## 特性

- **SQL** — MySQL 查询、DDL/DML 准备与执行
- **Redis** — 按逻辑 namespace 隔离，只读命令白名单，写操作需确认
- **MongoDB** — find、aggregate（stage 白名单）、写操作需确认
- **写操作两阶段确认** — 所有写操作先 `prepare` 生成 confirmation，再 `execute_confirmation` 真正执行
- **审计** — 所有操作写入本地 SQLite 审计库
- **安全** — 只监听 `127.0.0.1`，Bearer token 认证，配置文件权限校验（`0600`）
- **生产隔离** — 拒绝生产环境数据源，只允许 dev/support

## 技术栈

- Go 1.23+
- 官方 MCP Go SDK（Streamable HTTP）
- `modernc.org/sqlite`（纯 Go SQLite）
- `github.com/go-sql-driver/mysql`
- `github.com/redis/go-redis/v9`
- `go.mongodb.org/mongo-driver/mongo`
- `github.com/xwb1989/sqlparser`

## 快速开始

### 1. 配置

```bash
mkdir -p var
cp configs/config.example.yaml var/config.yaml
chmod 600 var/config.yaml
# 编辑 var/config.yaml，填入数据库连接信息和 client_token
```

### 2. 构建与运行

```bash
go build -o native-db-bridge-mcp ./cmd/native-db-bridge-mcp
./native-db-bridge-mcp serve --home ./var
```

或使用环境变量：

```bash
NATIVE_DB_BRIDGE_HOME=./var ./native-db-bridge-mcp serve
```

### 3. 安装为 launchd 服务（macOS）

```bash
./native-db-bridge-mcp install-service --home ./var
```

### 4. 健康检查

```bash
# 只检查配置和驱动
./native-db-bridge-mcp healthcheck

# 执行真实连接测试
./native-db-bridge-mcp healthcheck --connect
```

## MCP 客户端配置

在 Codex 或 Claude Code 的 MCP 配置中添加：

```json
{
  "mcpServers": {
    "native-db-bridge": {
      "url": "http://127.0.0.1:38987/mcp",
      "headers": {
        "Authorization": "Bearer <your-client-token>"
      }
    }
  }
}
```

## MCP 工具列表

### Metadata

| 工具 | 说明 |
|------|------|
| `datasource_list` | 列出可用数据源 |
| `datasource_healthcheck` | 数据源健康检查 |
| `sql_schema_list` | 列出 SQL schema |
| `sql_object_type_list` | 列出对象类型（table/view/procedure/function） |
| `sql_object_list` | 列出 schema 下的对象 |
| `sql_object_describe` | 描述表/视图/存储过程结构 |
| `sql_table_preview` | 预览表数据 |
| `sql_column_search` | 搜索 MySQL 字段元数据 |
| `sql_text_column_plan` | 生成文本列扫描计划，不扫描业务数据 |
| `redis_key_scan` | SCAN 扫描 Redis key |
| `redis_key_describe` | 描述 Redis key 类型、TTL、长度 |
| `mongo_database_list` | 列出 MongoDB 数据库 |
| `mongo_collection_list` | 列出集合 |
| `mongo_collection_describe` | 描述集合索引和样本 schema |

### Execution

| 工具 | 说明 |
|------|------|
| `sql_query` | 只读 SQL 查询（SELECT/SHOW/DESC/EXPLAIN） |
| `sql_text_scan` | 对指定文本字段执行 count-only 扫描 |
| `sql_prepare_change` | 准备 SQL 写操作，返回 confirmation |
| `redis_command` | Redis 只读命令 |
| `redis_prepare_change` | 准备 Redis 写操作，返回 confirmation |
| `mongo_find` | MongoDB 读操作（find/aggregate） |
| `mongo_prepare_change` | 准备 MongoDB 写操作，返回 confirmation |
| `execute_confirmation` | 执行已确认的写操作 |

### Control & Audit

| 工具 | 说明 |
|------|------|
| `operation_list` | 列出运行中和近期操作 |
| `cancel_operation` | 取消运行中的操作 |
| `audit_recent` | 查询最近审计事件 |
| `audit_summary` | 聚合审计事件，复盘失败率和慢查询 |
| `confirmation_get` | 查询 confirmation 状态 |
| `cancel_confirmation` | 取消 pending 状态的 confirmation |

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

系统诊断类查询如果因为权限失败，例如 `performance_schema`、`SHOW PROCESSLIST`、`SHOW GLOBAL STATUS`，agent 应先保留结构化错误并改用普通 schema 元数据工具或受控文本扫描；需要高权限诊断时，先让用户确认授权边界，不反复重试同一类无权限语句。

## 写操作流程

```
prepare_change → 返回 confirmation_id（pending）
                    ↓
         Agent 展示风险摘要，等待用户确认
                    ↓
execute_confirmation(confirmation_id) → 真正执行
```

- confirmation 有效期默认 10 分钟，过期自动标记为 `expired`
- pending 状态可随时 `cancel_confirmation`
- 所有状态变更写入审计日志

## 项目结构

```text
cmd/native-db-bridge-mcp/    入口
configs/                      示例配置
internal/
  app/                        应用组装
  audit/                      SQLite 审计存储
  backend/                    SQL/Redis/Mongo 后端接口与实现
  cli/                        CLI 命令（serve/healthcheck/install-service）
  config/                     配置加载与校验
  lifecycle/                  连接懒加载与空闲回收
  model/                      共享数据模型
  nbderrors/                  结构化错误码
  ops/                        操作 tracker（cancel function 管理）
  policy/                     读写分类、安全白名单、风险评估
  server/                     Streamable HTTP MCP、鉴权
  tools/                      MCP 工具 schema 与 handler
testdata/                     测试 fixtures
var/                          运行态文件（gitignored）
```

## 安全设计

- 配置文件权限必须为 `0600`，否则拒绝启动
- 审计库 `audit.db` 同样校验 `0600` 权限
- 只监听 `127.0.0.1`，不开放网络访问
- SQL 使用 parser 识别语句类型，拒绝 `FOR UPDATE`、`LOAD_FILE`、`INTO OUTFILE` 等危险语法
- Redis 默认拒绝未知命令，只允许白名单内的操作
- MongoDB aggregate 只允许安全 stage（`$match`/`$project`/`$sort` 等）
- DSN 强制禁用 `multiStatements`
- 生产环境数据源直接拒绝加载

## 文档

- [设计文档](docs/superpowers/specs/2026-06-05-native-db-bridge-mcp-design.md)
- [实现计划](docs/superpowers/plans/2026-06-05-native-db-bridge-mcp-implementation.md)

## License

Private
