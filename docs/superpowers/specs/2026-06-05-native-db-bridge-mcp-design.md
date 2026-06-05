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
- Redis：按逻辑 namespace 建立独立客户端，客户端启动时固定 db。
- MongoDB：按物理连接和逻辑 datasource 访问。

### 3.5 MCP 传输协议

首版使用 Streamable HTTP，不使用 SSE 或 stdio。

原因：

- stdio 无法满足多个 Codex 会话共享同一个本地服务的核心目标。
- SSE 已不是首选方向。
- Streamable HTTP 更符合当前 MCP 传输演进方向，也适合本机常驻服务。

服务只监听 `127.0.0.1`，不开放局域网访问。

认证采用 HTTP header：

```text
Authorization: Bearer <client_token>
```

`client_token` 从 `var/config.yaml` 读取。首版只支持单 token；轮换 token 后需要重启服务。Codex 侧 MCP 配置只保存本地 endpoint 和 token，不保存数据库凭据。

## 4. 配置模型

配置文件：

```text
/Users/repairman/opt/native-db-bridge/var/config.yaml
```

可提交示例：

```text
/Users/repairman/opt/native-db-bridge/configs/config.example.yaml
```

启动时必须校验 `var/config.yaml` 权限。最大允许权限为 `0600`；如果 group 或 others 有任何权限，服务拒绝启动。

### 4.1 配置草案

```yaml
server:
  listen: "127.0.0.1:38987"
  mcp_path: "/mcp"
  transport: "streamable_http"
  client_token: "change-me"
  request_timeout: "60s"
  query_timeout: "30s"
  max_result_rows: 1000
  redis_scan_count_max: 500

policy:
  allowed_environments:
    - dev
    - support
  production_enabled: false
  confirm_all_writes: true
  reject_write_without_confirmation: true
  confirmation_ttl: "10m"
  confirmation_expire_scan_interval: "1m"

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
- SQL datasource 绑定默认 database，并为每个 SQL datasource 建立独立连接池。
- SQL datasource 的实际 DSN 在运行时由 `connection.dsn + datasource.default_database` 组合得到。
- SQL 工具不支持通过参数覆盖 database；跨库访问必须在 SQL 中显式写限定名，且仍受当前 datasource 权限约束。
- 首版不在服务端解析跨库授权，依赖数据库用户权限隔离；配置中必须使用仅授权到目标开发/测试库的账号。
- 如果同一个 SQL connection 被 N 个 datasource 引用，运行时会创建 N 个独立连接池。
- Redis 每个 namespace 建立独立客户端，客户端初始化时固定 `db`。
- 生产环境数据源不允许加载。
- `var/config.yaml` 最大允许权限为 `0600`；如果 group 或 others 有任何权限，服务拒绝启动。

## 5. 连接生命周期

业务数据源连接采用懒加载与空闲回收。

启动时：

- 不连接任何 SQL、Redis、MongoDB 数据源。
- 只加载配置。
- 校验配置权限。
- 初始化 SQLite 审计库。
- 启动 MCP endpoint。

首次访问时：

- 根据 datasource 找到对应的连接配置。
- 如果连接池或客户端不存在，则创建。
- 执行完成后更新 `last_used_at`。

空闲回收：

- 后台定时扫描连接池。
- 当 `in_flight == 0` 且 `now - last_used_at > idle_ttl` 时关闭连接池或客户端。
- SQL 按 datasource 独立连接池回收。
- Redis 按 namespace 独立客户端回收。
- MongoDB 按 datasource 客户端回收。
- 连接管理器必须用互斥锁保护 `in_flight`、`last_used_at`、客户端指针和关闭操作。
- 关闭前必须在锁内再次确认 `in_flight == 0` 且 idle 超时，避免扫描器关闭刚被请求线程获取的连接。
- 获取连接时必须在锁内增加 `in_flight`，请求结束后递减并更新 `last_used_at`。

SQLite 审计库保持常驻打开，因为它是服务自身状态基础，且资源占用很低。

默认 healthcheck 只校验配置和驱动加载，不主动连接所有数据源。显式 `healthcheck --connect` 才执行真实连接测试。

confirmation 过期扫描使用 `policy.confirmation_expire_scan_interval`，不复用连接回收扫描间隔。

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
- `cancel_confirmation`

### 6.4 工具规则

`sql_query` 只允许：

- `SELECT`
- `SHOW`
- `DESC`
- `DESCRIBE`
- `EXPLAIN`

SQL 写操作必须使用 `sql_prepare_change`。

SQL 安全验证规则：

- DSN 必须禁用 multi statements；配置中出现 `multiStatements=true` 时拒绝启动。
- SQL 工具只接受单条语句。
- 首版使用 SQL parser 识别语句类型，不使用简单前缀匹配。
- `sql_query` 拒绝包含写语句、事务控制语句、`USE`、`LOCK`、`UNLOCK`、`CALL`、`SET` 的输入。
- `sql_query` 拒绝高风险函数或语法，包括 `LOAD_FILE`、`INTO OUTFILE`、`INTO DUMPFILE`。
- `sql_query` 拒绝 locking read，包括 `FOR UPDATE` 和 `LOCK IN SHARE MODE`。
- `SLEEP` 不作为语义白名单，仍受 `query_timeout` 和 `cancel_operation` 约束。

Redis 读命令使用 `redis_command`。Redis 写命令必须使用 `redis_prepare_change`。

Redis 命令策略：

- 默认拒绝未知命令。
- 首版只读白名单：`GET`、`MGET`、`TTL`、`PTTL`、`EXISTS`、`TYPE`、`STRLEN`、`HGET`、`HMGET`、`HGETALL`、`HLEN`、`HEXISTS`、`HKEYS`、`HVALS`、`LRANGE`、`LLEN`、`SCARD`、`SMEMBERS`、`SISMEMBER`、`ZRANGE`、`ZREVRANGE`、`ZRANK`、`ZREVRANK`、`ZSCORE`、`ZCARD`、`SCAN`、`HSCAN`、`SSCAN`、`ZSCAN`。
- 首版写命令通过 `redis_prepare_change`，包括但不限于 `SET`、`DEL`、`EXPIRE`、`PEXPIRE`、`PERSIST`、`HSET`、`HDEL`、`LPUSH`、`RPUSH`、`LPOP`、`RPOP`、`SADD`、`SREM`、`ZADD`、`ZREM`、`INCR`、`DECR`、`FLUSHDB`、`FLUSHALL`。
- `SELECT`、`EVAL`、`EVALSHA`、`SCRIPT`、`DEBUG`、`MONITOR`、`SUBSCRIBE`、`PSUBSCRIBE`、`CLIENT`、`CONFIG`、`SHUTDOWN` 永远拒绝。

MongoDB 读操作使用 `mongo_find`。MongoDB 写操作必须使用 `mongo_prepare_change`。

MongoDB aggregate 策略：

- 首版采用 stage 白名单，不采用黑名单。
- 允许读 stage：`$match`、`$project`、`$limit`、`$skip`、`$sort`、`$group`、`$count`、`$unwind`。
- 拒绝 `$out`、`$merge`、`$function`、`$accumulator`、`$where`、`$graphLookup`、`$lookup`。
- `mongo_find` 和 `mongo_prepare_change` 不接受 `database` 参数，固定使用 datasource 的 `default_database`。

`execute_confirmation` 是唯一真正执行写操作的 MCP 工具。

确认类型枚举：

- `sql_dml`
- `sql_ddl`
- `redis_write`
- `mongo_write`

### 6.5 工具输入输出 schema

除非特别说明，所有工具返回都包含：

- `ok`：布尔值。
- `operation_id`：操作 ID；metadata 工具可为空。
- `error`：结构化错误；成功时为空。

执行与诊断工具 timeout 规则：

- 带 `timeout` 字段的执行、prepare、connect healthcheck 工具可省略该字段。
- 省略时使用 `server.query_timeout`。
- 传入值超过 `server.query_timeout` 时按 `server.query_timeout` 截断。
- 纯 metadata 列表和描述工具默认不暴露 `timeout` 字段，由服务端统一使用 `server.query_timeout`。

#### `datasource_list`

输入：

```json
{
  "type": "sql|redis|mongo|null",
  "environment": "dev|support|null"
}
```

输出：

```json
{
  "ok": true,
  "datasources": [
    {
      "name": "saas_support",
      "type": "sql",
      "environment": "support",
      "writable": true,
      "default_database": "saas_support"
    }
  ]
}
```

不返回 host、password、token、dsn、uri。

#### `datasource_healthcheck`

输入：

```json
{
  "datasource": "saas_support",
  "mode": "config|connect"
}
```

`config` 只校验配置、驱动和策略，不创建业务连接。`connect` 显式创建连接并执行轻量探测。

输出：

```json
{
  "ok": true,
  "datasource": "saas_support",
  "mode": "config",
  "status": "healthy|unhealthy",
  "details": {}
}
```

#### `sql_query`

输入：

```json
{
  "datasource": "saas_support",
  "sql": "SELECT * FROM tc_org LIMIT 10",
  "limit": 100,
  "timeout": "30s"
}
```

规则：

- `datasource` 是 SQL datasource name。
- `sql` 必须是单条只读语句。
- `limit` 不能超过 `server.max_result_rows`。
- `limit` 省略时默认使用 `server.max_result_rows`。
- 不支持 `database` 覆盖参数。
- `timeout` 可省略；传入时不能超过 `server.query_timeout`，超过则按 `server.query_timeout` 截断。
- `SELECT ... FOR UPDATE` 和 `LOCK IN SHARE MODE` 被拒绝。

输出：

```json
{
  "ok": true,
  "operation_id": "op_...",
  "columns": [{"name": "id", "type": "BIGINT"}],
  "rows": [{"id": 1}],
  "row_count": 1,
  "elapsed_ms": 12
}
```

#### `sql_prepare_change`

输入：

```json
{
  "datasource": "saas_support",
  "sql": "UPDATE tc_org SET name = 'x' WHERE id = 1",
  "timeout": "30s"
}
```

规则：

- 只接受单条写语句。
- 接受 `INSERT`、`UPDATE`、`DELETE` 和 DDL。
- 拒绝 `SELECT`、`SHOW`、`DESC`、`DESCRIBE`、`EXPLAIN` 等只读语句。
- 拒绝事务控制语句、`USE`、`LOCK`、`UNLOCK`、`CALL`、`SET`。

输出：

```json
{
  "ok": true,
  "confirmation_id": "conf_...",
  "kind": "sql_dml",
  "datasource": "saas_support",
  "risk_level": "medium",
  "impact": {"mode": "estimated", "rows": 1},
  "summary": "UPDATE tc_org ... WHERE id = 1",
  "expires_at": "2026-06-05T12:00:00+08:00"
}
```

#### `redis_command`

输入：

```json
{
  "datasource": "saas-auth-support",
  "command": "GET",
  "args": ["redisson_token:dingtalk:corp_channel"],
  "timeout": "10s"
}
```

规则：

- `datasource` 是 Redis namespace name。
- `command` 必须在只读命令白名单中。
- `SELECT` 永远拒绝。
- 写命令必须改用 `redis_prepare_change`。

输出：

```json
{
  "ok": true,
  "operation_id": "op_...",
  "result": "value",
  "elapsed_ms": 5
}
```

#### `redis_prepare_change`

输入：

```json
{
  "datasource": "saas-auth-support",
  "command": "DEL",
  "args": ["some:key"],
  "timeout": "10s"
}
```

输出：

```json
{
  "ok": true,
  "confirmation_id": "conf_...",
  "kind": "redis_write",
  "datasource": "saas-auth-support",
  "risk_level": "high",
  "impact": {"mode": "best_effort", "keys": ["some:key"]},
  "summary": "DEL some:key",
  "expires_at": "2026-06-05T12:00:00+08:00"
}
```

#### `redis_key_scan`

输入：

```json
{
  "datasource": "saas-auth-support",
  "match": "redisson_token:*",
  "count": 100,
  "cursor": "0"
}
```

输出：

```json
{
  "ok": true,
  "keys": ["redisson_token:dingtalk:corp_channel"],
  "next_cursor": "0",
  "truncated": false
}
```

`count` 有上限，避免大 keyspace 一次返回过多结果。

#### `redis_key_describe`

输入：

```json
{
  "datasource": "saas-auth-support",
  "key": "some:key"
}
```

输出：

```json
{
  "ok": true,
  "key": "some:key",
  "type": "string",
  "ttl_ms": 60000,
  "length": 128,
  "exists": true
}
```

#### `sql_schema_list`

输入：

```json
{
  "datasource": "saas_support"
}
```

输出：

```json
{
  "ok": true,
  "schemas": ["saas_support"]
}
```

#### `sql_object_type_list`

输入：

```json
{
  "datasource": "saas_support"
}
```

输出：

```json
{
  "ok": true,
  "object_types": ["table", "view", "procedure", "function"]
}
```

#### `sql_object_list`

输入：

```json
{
  "datasource": "saas_support",
  "schema": "saas_support",
  "object_type": "table",
  "name_pattern": "tc_%"
}
```

输出：

```json
{
  "ok": true,
  "objects": [
    {"schema": "saas_support", "name": "tc_org", "type": "table"}
  ]
}
```

#### `sql_object_describe`

输入：

```json
{
  "datasource": "saas_support",
  "schema": "saas_support",
  "object_name": "tc_org",
  "object_type": "table"
}
```

输出：

```json
{
  "ok": true,
  "object": {"schema": "saas_support", "name": "tc_org", "type": "table"},
  "columns": [{"name": "id", "type": "BIGINT", "nullable": false}],
  "indexes": [{"name": "PRIMARY", "columns": ["id"], "unique": true}],
  "definition": ""
}
```

表返回字段、主键、索引。视图返回字段和 view definition 摘要。存储过程和函数返回参数与 definition 摘要。

#### `sql_table_preview`

输入：

```json
{
  "datasource": "saas_support",
  "schema": "saas_support",
  "table": "tc_org",
  "limit": 50
}
```

输出：

```json
{
  "ok": true,
  "columns": [{"name": "id", "type": "BIGINT"}],
  "rows": [{"id": 1}],
  "row_count": 1,
  "elapsed_ms": 8
}
```

#### `mongo_find`

输入：

```json
{
  "datasource": "saas-support-mongo",
  "collection": "users",
  "operation": "find",
  "filter": {"id": 1},
  "pipeline": [],
  "projection": {},
  "sort": {},
  "limit": 100,
  "timeout": "30s"
}
```

首版读操作支持 `find`、`findOne`、`countDocuments`、`distinct` 和轻量 `aggregate`。数据库固定为 datasource 的 `default_database`，不允许调用方覆盖。

字段规则：

- `operation=find|findOne|countDocuments|distinct` 时使用 `filter`、`projection`、`sort`、`limit`。
- `operation=aggregate` 时必须传 `pipeline`，并按 aggregate stage 白名单校验。
- `pipeline` 不用于非 aggregate 操作。

输出：

```json
{
  "ok": true,
  "operation_id": "op_...",
  "documents": [{"id": 1}],
  "count": 1,
  "elapsed_ms": 10
}
```

#### `mongo_prepare_change`

输入：

```json
{
  "datasource": "saas-support-mongo",
  "collection": "users",
  "operation": "updateOne",
  "filter": {"id": 1},
  "document": {"$set": {"name": "x"}},
  "timeout": "30s"
}
```

首版写操作支持 `insertOne`、`insertMany`、`updateOne`、`updateMany`、`deleteOne`、`deleteMany`、`dropCollection`。首版不承诺 MongoDB 多文档事务。

输出结构与其他 prepare 工具一致，必须包含 `confirmation_id`。

MongoDB 写操作字段矩阵：

| operation | collection | filter | document | documents |
| --- | --- | --- | --- | --- |
| `insertOne` | 必填 | 禁止 | 必填 | 禁止 |
| `insertMany` | 必填 | 禁止 | 禁止 | 必填 |
| `updateOne` | 必填 | 必填 | 必填 | 禁止 |
| `updateMany` | 必填 | 必填 | 必填 | 禁止 |
| `deleteOne` | 必填 | 必填 | 禁止 | 禁止 |
| `deleteMany` | 必填 | 必填 | 禁止 | 禁止 |
| `dropCollection` | 必填 | 禁止 | 禁止 | 禁止 |

#### `mongo_database_list`

输入：

```json
{
  "datasource": "saas-support-mongo"
}
```

输出固定 datasource 可见数据库。首版通常只返回 `default_database`。

```json
{
  "ok": true,
  "databases": ["saas_support"]
}
```

#### `mongo_collection_list`

输入：

```json
{
  "datasource": "saas-support-mongo",
  "name_pattern": "user*"
}
```

输出：

```json
{
  "ok": true,
  "collections": [{"name": "users", "type": "collection"}]
}
```

#### `mongo_collection_describe`

输入：

```json
{
  "datasource": "saas-support-mongo",
  "collection": "users"
}
```

输出：

```json
{
  "ok": true,
  "collection": "users",
  "estimated_count": 100,
  "indexes": [{"name": "_id_", "keys": {"_id": 1}, "unique": true}],
  "sample_schema": {}
}
```

#### `execute_confirmation`

输入：

```json
{
  "confirmation_id": "conf_..."
}
```

输出：

```json
{
  "ok": true,
  "operation_id": "op_...",
  "confirmation_id": "conf_...",
  "status": "executed",
  "affected_count": 1,
  "result_summary": "updated 1 row",
  "elapsed_ms": 20
}
```

失败时返回结构化错误，并将 confirmation 状态更新为 `failed`。

#### `operation_list`

输入：

```json
{
  "status": "running|cancel_requested|finished|null",
  "limit": 50
}
```

输出运行中和近期操作。`operations` 记录所有操作，包括读和写；只读长查询也会进入 `operations`。

输出：

```json
{
  "ok": true,
  "operations": [
    {
      "operation_id": "op_...",
      "kind": "sql_query",
      "datasource": "saas_support",
      "status": "running",
      "started_at": "2026-06-05T12:00:00+08:00",
      "finished_at": null,
      "cancel_requested_at": null,
      "confirmation_id": null,
      "error_code": null,
      "error_summary": null
    }
  ]
}
```

#### `cancel_operation`

输入：

```json
{
  "operation_id": "op_..."
}
```

首版取消范围：

- 支持 SQL 查询取消。
- Redis 和 MongoDB 取消仅尽力设置 context cancellation，不承诺驱动一定立即中断。
- 已提交的写操作不能回滚，除非驱动和事务边界明确支持。

取消机制：

- 每个 operation 在内存中绑定 `context.CancelFunc`。
- `cancel_operation` 设置 `operations.cancel_requested_at`，调用对应 cancel function。
- 执行函数收到 context cancellation 后更新 operation 状态。

输出：

```json
{
  "ok": true,
  "operation_id": "op_...",
  "status": "cancel_requested"
}
```

#### `cancel_confirmation`

输入：

```json
{
  "confirmation_id": "conf_..."
}
```

只能取消 `pending` 状态的 confirmation。已过期、已执行、执行中或失败的 confirmation 不能取消。

输出：

```json
{
  "ok": true,
  "confirmation_id": "conf_...",
  "status": "cancelled"
}
```

#### `audit_recent`

输入：

```json
{
  "datasource": "saas_support",
  "event_type": null,
  "status": null,
  "limit": 50
}
```

输出最近审计事件。

输出：

```json
{
  "ok": true,
  "events": [
    {
      "id": "evt_...",
      "event_type": "sql_query",
      "datasource": "saas_support",
      "status": "success",
      "summary": "SELECT ...",
      "created_at": "2026-06-05T12:00:00+08:00"
    }
  ]
}
```

#### `confirmation_get`

输入：

```json
{
  "confirmation_id": "conf_..."
}
```

输出 confirmation 当前状态和摘要，不返回敏感 payload 全量。

输出：

```json
{
  "ok": true,
  "confirmation": {
    "confirmation_id": "conf_...",
    "kind": "sql_dml",
    "datasource": "saas_support",
    "status": "pending",
    "risk_level": "medium",
    "summary": "UPDATE ...",
    "expires_at": "2026-06-05T12:00:00+08:00"
  }
}
```

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
- `confirmation_id` 来自 `*_prepare_change` 返回值。
- `confirmation_id` 过期清理采用被动检查和后台扫描结合：执行时发现过期会拒绝并标记 `expired`；后台任务定期把过期 `pending` 记录标记为 `expired`。
- `cancel_confirmation` 可以把 `pending` confirmation 标记为 `cancelled`。
- `expired`、`cancelled`、`executed`、`failed` 都是终态。
- confirmation 过期、执行、失败、取消都写入 `audit_events`。

影响范围预估：

- SQL `UPDATE` / `DELETE` 尽量生成 `SELECT COUNT(*)` 预估。
- SQL DDL 标记为 schema-changing 和 high risk。
- Redis `DEL`、`FLUSH*`、`SET`、`HSET`、`EXPIRE` 按 key 和命令标记风险。
- Redis `FLUSH*` 标记为极高风险。
- Redis 影响范围采用 best effort：可先检查 key 是否存在、类型和 TTL；无法精确预估时明确 `impact: unknown`。
- MongoDB `deleteMany`、`updateMany`、`drop` 标记为高风险。
- 能估算 filter count 时估算，不能估算时明确 `impact: unknown`。

服务端不解析用户自然语言确认。agent 必须在调用 `execute_confirmation` 前拿到用户明确的“确认/继续/是”。

## 8. 审计与存储

审计主存储使用 SQLite，不使用 JSONL 作为 source of truth。

SQLite 由 Go 依赖 `modernc.org/sqlite` 提供。`audit.db` 是本地文件，不是独立服务；运行时不依赖系统 `sqlite3` 命令。

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
- `summary`
- `risk_level`
- `impact_json`
- `status`
- `expires_at`
- `executed_at`
- `error_summary`
- `created_at`
- `updated_at`

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
- `confirmation_id`
- `started_at`
- `finished_at`
- `cancel_requested_at`
- `error_code`
- `error_summary`

关系规则：

- `operations` 记录所有操作，包括 metadata、读操作、prepare、execute。
- `audit_events.operation_id` 可为空；服务内部异常或启动事件没有 operation 时允许为空。
- `audit_events.confirmation_id` 可为空；只读操作和 metadata 操作通常为空。
- `operations.confirmation_id` 可为空；只读操作为空，执行写 confirmation 时填写。
- `confirmations` 与 execute operation 是 1:0..1；未执行时没有 execute operation，执行后最多一个 execute operation。

SQLite 启用 WAL，使用短事务，并为 `created_at`、`datasource`、`confirmation_id`、`status` 建索引。

SQLite 可用性规则：

- SQLite 文件不存在时自动创建。
- 新建 `audit.db` 时权限必须是 `0600`。
- 已存在的 `audit.db` 启动时必须校验权限；如果 group 或 others 有任何权限，服务拒绝启动。
- 启动时初始化 schema migration。
- 启动时执行 SQLite integrity check。
- 如果 `audit.db` 不可访问、文件损坏、integrity check 失败或 schema migration 失败，服务拒绝启动，并输出结构化错误。
- 运行中如果 SQLite 写入失败，写操作确认和执行全部拒绝；只读查询也返回 `internal` 错误，因为审计不可用时不能满足可追踪要求。
- 恢复方式是停止服务、备份损坏文件、人工修复或删除后重新初始化；服务不自动丢弃旧审计库。

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

首版错误码：

- `CONFIG_FILE_NOT_FOUND`
- `CONFIG_PERMISSION_TOO_OPEN`
- `CONFIG_DATASOURCE_NOT_FOUND`
- `CONFIG_PRODUCTION_DATASOURCE_REJECTED`
- `AUTH_MISSING_TOKEN`
- `AUTH_INVALID_TOKEN`
- `POLICY_WRITE_REQUIRES_CONFIRMATION`
- `POLICY_PRODUCTION_REJECTED`
- `POLICY_REDIS_SELECT_REJECTED`
- `POLICY_READONLY_TOOL_REJECTED_WRITE`
- `CONNECTION_FAILED`
- `CONNECTION_AUTH_FAILED`
- `QUERY_TIMEOUT`
- `QUERY_SYNTAX_ERROR`
- `QUERY_LOCKING_READ_REJECTED`
- `CONFIRMATION_NOT_FOUND`
- `CONFIRMATION_EXPIRED`
- `CONFIRMATION_ALREADY_EXECUTED`
- `CONFIRMATION_INVALID_STATE`
- `OPERATION_NOT_FOUND`
- `OPERATION_NOT_CANCELABLE`
- `DRIVER_ERROR`
- `INTERNAL_ERROR`

正式 MCP schema 生成规则：

- 本节 JSON 是契约示例；实现时必须用 Go 结构体生成 MCP tool schema。
- 每个字段必须声明 `required`、类型、枚举和最大长度或最大数量。
- 测试必须校验生成的 schema 与本文字段规则一致。

## 10. 测试策略

### 10.1 单元测试

- SQL 读写分类。
- Redis 命令读写分类。
- Redis `SELECT` 拒绝。
- Redis `DEL`、`FLUSH*` 风险识别。
- MongoDB operation 分类。
- confirmation 状态迁移。
- 配置加载和权限校验。
- SQL locking read 拒绝，包括 `FOR UPDATE` 和 `LOCK IN SHARE MODE`。
- 错误归一化。
- 连接生命周期懒加载和空闲回收。
- MCP 工具输入输出 schema 校验。
- client token 鉴权。
- confirmation 过期清理。
- pending confirmation 手工取消。
- `operations` 与 `confirmations` 关系。

### 10.2 集成测试

Docker 可以用于本地或 CI 集成测试，但不作为运行部署方式。

覆盖：

- MySQL 查询、DDL/DML prepare、execute。
- Redis 多 namespace 映射到同一实例不同 db，每个 namespace 独立客户端。
- MongoDB find 和 write prepare。
- SQLite 审计落库。
- SQLite integrity check 失败时拒绝启动。
- `cancel_operation` 对长 SQL 查询生效。
- Streamable HTTP MCP endpoint 鉴权。

单元测试通过接口 mock SQL、Redis、MongoDB 后端；集成测试可用 Docker Compose 或 testcontainers-go 启动 MySQL、Redis、MongoDB。测试配置 fixtures 放在 `testdata/`，不得包含真实凭据。

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
