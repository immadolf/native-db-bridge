# native-db-bridge-mcp 设计文档

## 1. 背景与目标

`native-db-bridge-mcp` 是一个用户级、本机常驻的 MCP 服务，用于替代开发环境和测试环境中的 DataGrip MCP 数据库操作入口。

现有 DataGrip MCP 依赖 DataGrip 启动 AI Assistant 插件。该模式会带来额外内存、CPU 与 IDE 卡顿成本；同时用户通常由 agent 通过 MCP 访问数据库，DataGrip 常驻价值降低。

本项目目标：

- 提供一个低资源、本机常驻、多个 Codex 会话共享的 MCP bridge。
- 支持固定全局数据源，不做项目级覆盖。
- 支持 SQL、Redis、MongoDB。
- 开发环境和测试环境允许 DDL、DML、Redis 写、MongoDB 写。
- 所有写操作必须逐次确认。
- 不接入生产环境，生产数据库继续走现有生产路由。
- 不依赖 dbx，底层直接使用原生数据库驱动。

非目标：

- 不复制 DataGrip / IntelliJ 的代码编辑、重构、文件操作能力。
- 不作为生产数据库写操作入口。
- 不做项目级数据源自动发现。
- 不在首版实现团队共享部署或远程访问。

## 2. 核心决策

### 2.1 技术栈

首版使用 Go。

原因：

- Go 单进程常驻资源远低于 DataGrip。
- Go 官方 MCP SDK 可用。
- SQL、Redis、MongoDB 驱动生态成熟。
- `context`、超时、取消、连接池和本地 daemon 实现直接。
- 相比 Rust，Go 在首版实现和维护成本上更合适。

实现阶段参考 ECC 仓库中 Go 相关 skill，重点采用：

- 小接口。
- 显式 `context.Context`。
- 超时与取消。
- 依赖注入。
- 表驱动测试。
- 清晰错误处理。
- 避免不必要的全局状态。

### 2.2 运行与部署

首版使用本地 Go 二进制和 macOS 用户级 `launchd` 服务，不使用 Docker 作为主部署方式。

Docker 只作为后续可选集成测试或分发形态，不作为默认运行依赖。

源码目录：

```text
/Users/repairman/opt/native-db-bridge
```

运行态文件放在源码目录内的 `var/` 下，保持服务内聚：

```text
/Users/repairman/opt/native-db-bridge/
  cmd/
  internal/
  pkg/
  configs/
    config.example.yaml
  docs/
    superpowers/
      specs/
  var/
    config.yaml
    audit.db
    logs/
      server.log
    run/
      native-db-bridge.pid
  .gitignore
```

`var/` 必须整体加入 `.gitignore`，避免真实配置、审计库和日志误提交。

launchd plist 仍需放在：

```text
~/Library/LaunchAgents/com.repairman.native-db-bridge.plist
```

plist 只保存启动路径、`--home` 路径和日志重定向，不保存敏感信息。

### 2.3 运行态根目录

服务支持：

```bash
native-db-bridge-mcp serve --home /Users/repairman/opt/native-db-bridge/var
```

也支持：

```bash
NATIVE_DB_BRIDGE_HOME=/Users/repairman/opt/native-db-bridge/var native-db-bridge-mcp serve
```

相对路径都基于 `--home` 解析。

## 3. 架构

服务分为四层：

```text
Codex MCP Client
  -> MCP Tool Layer
  -> Policy & Confirmation Layer
  -> Execution Backend Layer
  -> Native Driver Backends
```

### 3.1 MCP Tool Layer

负责暴露 MCP 工具、校验输入、调用策略层和执行层。

该层不直接执行写操作。

### 3.2 Policy & Confirmation Layer

负责：

- 区分只读与写操作。
- 拒绝生产环境。
- 拒绝 Redis `SELECT`。
- 生成写操作确认记录。
- 校验确认状态。
- 生成风险摘要和影响范围预估。

所有写操作必须经过该层。

### 3.3 Execution Backend Layer

定义统一执行接口，屏蔽 SQL、Redis、MongoDB 驱动差异。

首版只实现原生驱动后端。

### 3.4 Native Driver Backends

首版支持：

- SQL：MySQL，后续可加 PostgreSQL。
- Redis：按物理实例连接，按逻辑 namespace 选择 db。
- MongoDB：按物理连接和逻辑 datasource 访问。

## 4. 配置模型

配置文件：

```text
/Users/repairman/opt/native-db-bridge/var/config.yaml
```

可提交示例：

```text
/Users/repairman/opt/native-db-bridge/configs/config.example.yaml
```

启动时必须校验 `var/config.yaml` 权限，建议为 `0600`。权限过宽时服务拒绝启动。

### 4.1 配置草案

```yaml
server:
  listen: "127.0.0.1:38987"
  mcp_path: "/mcp"
  client_token: "change-me"
  request_timeout: "60s"
  query_timeout: "30s"
  max_result_rows: 1000

policy:
  allowed_environments:
    - dev
    - support
  production_enabled: false
  confirm_all_writes: true
  reject_write_without_confirmation: true
  confirmation_ttl: "10m"

connection_lifecycle:
  defaults:
    lazy_connect: true
    idle_ttl: "10m"
    close_scan_interval: "1m"
    connect_timeout: "5s"
  sql:
    idle_ttl: "10m"
  redis:
    idle_ttl: "5m"
  mongo:
    idle_ttl: "10m"

storage:
  sqlite_path: "./audit.db"
  log_path: "./logs/server.log"

connections:
  sql:
    - name: "mysql-dev-main"
      environment: "dev"
      driver: "mysql"
      dsn: "user:password@tcp(host-dev:3306)/?parseTime=true&charset=utf8mb4"
      pool:
        max_open_conns: 5
        max_idle_conns: 2
        conn_max_lifetime: "30m"

    - name: "mysql-support-main"
      environment: "support"
      driver: "mysql"
      dsn: "user:password@tcp(host-support:3306)/?parseTime=true&charset=utf8mb4"

  redis:
    - name: "redis-support-main"
      environment: "support"
      address: "host-a:6379"
      username: ""
      password: "password"
      tls: false

  mongo:
    - name: "mongo-support-main"
      environment: "support"
      uri: "mongodb://user:password@host:27017"

datasources:
  sql:
    - name: "saas_dev"
      environment: "dev"
      connection: "mysql-dev-main"
      default_database: "saas_dev"

    - name: "saas_support"
      environment: "support"
      connection: "mysql-support-main"
      default_database: "saas_support"

    - name: "channel_msg_support"
      environment: "support"
      connection: "mysql-support-main"
      default_database: "channel_msg_support"

  redis:
    - name: "saas-auth-support"
      environment: "support"
      connection: "redis-support-main"
      db: 0
      service: "saas-auth"

    - name: "saas-web-bff-support"
      environment: "support"
      connection: "redis-support-main"
      db: 1
      service: "saas-web-bff"

    - name: "channel-msg-support"
      environment: "support"
      connection: "redis-support-main"
      db: 2
      service: "channel-msg"

  mongo:
    - name: "saas-support-mongo"
      environment: "support"
      connection: "mongo-support-main"
      default_database: "saas_support"
```

### 4.2 配置规则

- `connections` 保存物理连接和凭据。
- `datasources` 保存 agent 可见的业务入口。
- `datasource_list` 返回 `datasources`，不返回底层密码。
- Redis datasource 是逻辑 namespace，绑定 `connection + db + service`。
- Redis 禁止执行 `SELECT`，db 只能由 datasource 映射决定。
- SQL datasource 绑定默认 database，物理 MySQL connection 不固定单库。
- 生产环境数据源不允许加载。

## 5. 连接生命周期

业务数据源连接采用懒加载与空闲回收。

启动时：

- 不连接任何 SQL、Redis、MongoDB 数据源。
- 只加载配置。
- 校验配置权限。
- 初始化 SQLite 审计库。
- 启动 MCP endpoint。

首次访问时：

- 根据 datasource 找到 physical connection。
- 如果连接池或客户端不存在，则创建。
- 执行完成后更新 `last_used_at`。

空闲回收：

- 后台定时扫描连接池。
- 当 `in_flight == 0` 且 `now - last_used_at > idle_ttl` 时关闭连接池或客户端。
- Redis 多 namespace 共享同一个 physical connection，按 physical connection 聚合活跃时间。
- SQL 和 MongoDB 同样按 physical connection 回收。

SQLite 审计库保持常驻打开，因为它是服务自身状态基础，且资源占用很低。

默认 healthcheck 只校验配置和驱动加载，不主动连接所有数据源。显式 `healthcheck --connect` 才执行真实连接测试。

## 6. MCP 工具接口

### 6.1 Metadata 工具

- `datasource_list`
- `datasource_healthcheck`
- `sql_schema_list`
- `sql_object_type_list`
- `sql_object_list`
- `sql_object_describe`
- `sql_table_preview`
- `redis_key_scan`
- `redis_key_describe`
- `mongo_database_list`
- `mongo_collection_list`
- `mongo_collection_describe`

### 6.2 Execution 工具

- `sql_query`
- `sql_prepare_change`
- `redis_command`
- `redis_prepare_change`
- `mongo_find`
- `mongo_prepare_change`
- `execute_confirmation`

### 6.3 Control / Audit 工具

- `operation_list`
- `cancel_operation`
- `audit_recent`
- `confirmation_get`

### 6.4 工具规则

`sql_query` 只允许：

- `SELECT`
- `SHOW`
- `DESC`
- `DESCRIBE`
- `EXPLAIN`

SQL 写操作必须使用 `sql_prepare_change`。

Redis 读命令使用 `redis_command`。Redis 写命令必须使用 `redis_prepare_change`。

MongoDB 读操作使用 `mongo_find`。MongoDB 写操作必须使用 `mongo_prepare_change`。

`execute_confirmation` 是唯一真正执行写操作的 MCP 工具。

## 7. 写操作确认状态机

只读流程：

```text
agent -> *_read tool -> policy classify read -> execute -> audit_events
```

写操作 prepare 流程：

```text
agent -> *_prepare_change -> classify write -> risk summary -> persist confirmation -> audit_events
```

写操作 execute 流程：

```text
user confirms -> agent -> execute_confirmation(confirmation_id)
  -> SQLite transaction
  -> check pending and not expired
  -> mark executing
  -> execute frozen payload
  -> mark executed or failed
  -> audit_events
```

状态机：

```text
pending -> executing -> executed
pending -> executing -> failed
pending -> expired
pending -> cancelled
```

强规则：

- `execute_confirmation` 只接受 `confirmation_id`。
- `execute_confirmation` 不接受新的 SQL、Redis 命令或 MongoDB payload。
- confirmation 记录冻结 prepare 时的 payload。
- 同一个 `confirmation_id` 只能执行一次。
- 过期后不能执行。
- 非 `pending` 状态不能执行。
- 所有状态迁移写入 SQLite。
- 底层写执行函数不对 MCP 直接暴露。

影响范围预估：

- SQL `UPDATE` / `DELETE` 尽量生成 `SELECT COUNT(*)` 预估。
- SQL DDL 标记为 schema-changing 和 high risk。
- Redis `DEL`、`FLUSH*`、`SET`、`HSET`、`EXPIRE` 按 key 和命令标记风险。
- Redis `FLUSH*` 标记为极高风险。
- MongoDB `deleteMany`、`updateMany`、`drop` 标记为高风险。
- 能估算 filter count 时估算，不能估算时明确 `impact: unknown`。

服务端不解析用户自然语言确认。agent 必须在调用 `execute_confirmation` 前拿到用户明确的“确认/继续/是”。

## 8. 审计与存储

审计主存储使用 SQLite，不使用 JSONL 作为 source of truth。

文件：

```text
/Users/repairman/opt/native-db-bridge/var/audit.db
```

运行日志：

```text
/Users/repairman/opt/native-db-bridge/var/logs/server.log
```

JSONL 仅作为后续可选导出格式。

### 8.1 表结构

`confirmations`：

- `id`
- `kind`
- `datasource`
- `payload_json`
- `payload_hash`
- `risk_level`
- `impact_json`
- `status`
- `expires_at`
- `executed_at`
- `error_summary`

`audit_events`：

- `id`
- `event_type`
- `datasource`
- `operation_id`
- `confirmation_id`
- `summary`
- `status`
- `elapsed_ms`
- `row_count`
- `error_code`
- `created_at`

`operations`：

- `id`
- `kind`
- `datasource`
- `status`
- `started_at`
- `finished_at`
- `cancel_requested_at`

SQLite 启用 WAL，使用短事务，并为 `created_at`、`datasource`、`confirmation_id`、`status` 建索引。

## 9. 错误处理

所有 MCP 工具返回结构化错误。

示例：

```json
{
  "code": "POLICY_WRITE_REQUIRES_CONFIRMATION",
  "category": "policy",
  "message": "写操作必须先调用 prepare 工具生成确认记录",
  "datasource": "saas_support",
  "operation_id": "",
  "retryable": false,
  "details": {}
}
```

错误分类：

- `config`：配置缺失、权限过宽、数据源不存在。
- `policy`：生产环境拒绝、写操作未确认、Redis `SELECT` 被拒绝。
- `connection`：连接失败、认证失败、网络超时。
- `syntax`：SQL、MongoDB filter、Redis command 语法错误。
- `timeout`：查询或执行超时。
- `driver`：底层驱动错误。
- `internal`：服务内部错误。

## 10. 测试策略

### 10.1 单元测试

- SQL 读写分类。
- Redis 命令读写分类。
- Redis `SELECT` 拒绝。
- Redis `DEL`、`FLUSH*` 风险识别。
- MongoDB operation 分类。
- confirmation 状态迁移。
- 配置加载和权限校验。
- 错误归一化。
- 连接生命周期懒加载和空闲回收。

### 10.2 集成测试

Docker 可以用于本地或 CI 集成测试，但不作为运行部署方式。

覆盖：

- MySQL 查询、DDL/DML prepare、execute。
- Redis 多 namespace 映射到同一实例不同 db。
- MongoDB find 和 write prepare。
- SQLite 审计落库。
- `cancel_operation` 对长 SQL 查询生效。

### 10.3 并发测试

- 两个会话同时执行同一个 `confirmation_id`，只能成功一次。
- MCP client 重试不能重复写。
- 过期 confirmation 不能执行。
- 空闲连接回收时不关闭 in-flight 操作。

## 11. 验收标准

硬门禁：

- 无确认不能执行任何 SQL、Redis、MongoDB 写操作。
- 同一 confirmation 不能重复执行。
- 过期 confirmation 不能执行。
- `production_enabled: false` 时生产数据源拒绝加载。
- Redis `SELECT` 永远拒绝。
- `var/config.yaml` 权限过宽时拒绝启动。
- `datasource_list` 不泄露密码。
- 启动时不连接任何业务数据源。
- 首次使用某个 datasource 时才懒加载底层连接。
- 空闲超过 `idle_ttl` 后关闭无 in-flight 的底层连接。

功能验收：

- Codex 多会话连接同一个本地 MCP endpoint。
- 能列出固定全局数据源。
- 能浏览 SQL schema、对象和表预览。
- 能执行 SQL 只读查询。
- 能 prepare 并确认执行 SQL DDL/DML。
- 能执行 Redis 读命令。
- 能 prepare 并确认执行 Redis 写命令。
- 能执行 MongoDB 读操作。
- 能 prepare 并确认执行 MongoDB 写操作。
- 能查看最近审计记录。
- 能取消可取消的长 SQL 查询。

## 12. 后续阶段

首版完成后可考虑：

- PostgreSQL 支持。
- 1Password CLI 或环境变量插值。
- JSONL 审计导出。
- Docker 分发镜像。
- 更强 SQL AST 分析。
- 更丰富的 MongoDB aggregate 支持。
- 更细的 role-based policy。
