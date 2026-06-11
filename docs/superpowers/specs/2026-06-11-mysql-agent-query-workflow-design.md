# MySQL Agent Query Workflow 改进设计

## 1. 背景

`native-db-bridge-mcp` 已作为本机常驻 MCP 服务替代 DataGrip MCP，承担 dev/support 环境数据库入口。近一周真实使用记录显示，当前 MySQL 链路已经可用，但 agent 使用时仍存在明显摩擦：

- 字段和表名需要人工或 agent 猜测，导致大量 `Unknown column` 和 `Table doesn't exist`。
- 宽范围 `LIKE`、URL 扫描和 Base64/文本字段处理容易触发慢查询或超时。
- `sql_query` 的自动 `LIMIT` 包装对 `SHOW`、尾部分号等场景不兼容。
- MySQL 文本列以 `[]byte` 形式进入 JSON 后会变成 Base64，影响直接阅读和二次处理。
- 错误码过粗，语法错、未知字段、缺表、超时都容易表现为 `DRIVER_ERROR`。
- 审计复盘需要直接查询 SQLite 并手工 join，不利于后续持续改进。

本设计聚焦 MySQL/SQL 工具，不扩展 Redis、MongoDB，不改变生产隔离策略。

## 2. 目标与非目标

### 2.1 目标

- 降低 MySQL 查询失败率，尤其是 schema 猜测造成的失败。
- 提供 agent 友好的字段发现、文本列扫描计划和受控扫描能力。
- 修正 `sql_query` 基础兼容问题，让只读查询工具行为稳定。
- 让 SQL 查询结果对人和 agent 都可直接阅读，避免文本列 Base64 绕路。
- 细化 MySQL 错误分类，让失败后能明确选择下一步。
- 在后续阶段提供审计摘要能力，支持按时间、数据源、工具和错误类型复盘。

### 2.2 非目标

- 不做生产数据库入口。
- 不把 native-db-bridge 做成通用数据库平台或远程共享服务。
- 不自动理解业务语义，不替 agent 编写最终业务 SQL。
- 不在本阶段改 Redis/MongoDB 工具。
- 不在 P0 引入任务线程关联模型；MCP 当前没有稳定会话标识。

## 3. 总体方案

采用 **MySQL Agent Query Workflow**：保留现有 `tools -> backend -> policy/audit -> driver` 分层，在 MySQL 工具面增加 schema 探索和文本扫描辅助能力，并修正查询执行与错误输出。

推荐 agent 工作流：

1. 使用 `datasource_list` 确认目标数据源。
2. 使用 `sql_column_search` 或 `sql_text_column_plan` 查真实表字段。
3. 使用 `sql_text_scan` 做受控 count 扫描，或使用 `sql_query` 查询明细。
4. 根据结构化错误调整查询，不再对不可重试错误盲目重试。

## 4. 工具契约设计

### 4.1 `sql_column_search`

用途：按 schema、表名、字段名和字段类型搜索 MySQL 元数据，减少字段猜测。

输入：

- `datasource`：SQL 数据源名称。
- `schema`：数据库/schema 名称。
- `table_pattern`：可选，表名 `LIKE` 模式。
- `column_pattern`：可选，字段名 `LIKE` 模式。
- `data_types`：可选，字段类型过滤，例如 `varchar`、`text`、`json`。
- `limit`：可选，最多返回条数，受全局上限限制。

输出：

- `schema`
- `table`
- `column`
- `data_type`
- `column_type`
- `nullable`
- `column_key`
- `comment`

约束：

- 只查询 `information_schema.COLUMNS`。
- 不扫描业务表数据。
- `schema` 必填，避免误扫所有库。

### 4.2 `sql_text_column_plan`

用途：生成文本列扫描计划，帮助 agent 先知道哪些列值得扫描。

输入：

- `datasource`
- `schema`
- `table_pattern`：可选，限制候选表。
- `column_pattern`：可选，限制候选列。
- `keywords`：关键词列表，用于说明扫描目标。
- `max_tables`：最多候选表数。
- `max_columns`：最多候选列数。

输出：

- 候选表字段列表。
- 建议的扫描批次，每批包含表、字段和关键词。
- 风险提示，包括候选列数量、可能慢查询原因和建议先缩小范围的条件。

约束：

- P0 只生成计划，不查询业务表。
- 候选列限定为文本类字段：`char`、`varchar`、`text`、`mediumtext`、`longtext`、`json` 等。
- 不自动执行宽范围扫描。

### 4.3 `sql_text_scan`

用途：受控执行文本列 count 扫描，替代一次性拼接巨大 `UNION ALL`。

输入：

- `datasource`
- `schema`
- `targets`：待扫描字段列表，每项包含 `table` 和 `column`。
- `keywords`：关键词列表。
- `mode`：P0 只支持 `count`。
- `max_columns_per_query`：每组最多字段数。
- `timeout`：可选，但不能超过全局查询超时。

输出：

- 每个表字段的命中数量。
- 每个扫描批次的耗时。
- 是否超时或被跳过。
- 若失败，返回对应字段和可行动错误。

约束：

- P0 不返回原始业务行，避免结果过大。
- 只生成 `COUNT` 聚合。
- 每批 SQL 必须受字段数和超时控制。

### 4.4 `sql_query` 修正

现有 `sql_query` 继续作为通用只读查询工具，但修正以下行为：

- 执行前去掉尾部分号。
- `SHOW`、`DESCRIBE`、`DESC`、`EXPLAIN` 不做子查询 `LIMIT` 包装。
- `SELECT` 和 `UNION` 已有 `LIMIT` 时不追加。
- 无 `LIMIT` 的 `SELECT`/`UNION` 再做安全包装。
- 结果行归一化：合法 UTF-8 `[]byte` 转字符串；非 UTF-8 保留为显式 binary/base64 结构。

## 5. 错误处理设计

MySQL 错误需要映射为更具体的结构化错误：

- `1054 Unknown column` -> `SQL_UNKNOWN_COLUMN`，不可重试，建议先使用 `sql_column_search`。
- `1146 Table doesn't exist` -> `SQL_UNKNOWN_TABLE`，不可重试，建议先使用 `sql_object_list`。
- `1064 syntax` -> `QUERY_SYNTAX_ERROR`，不可重试，保留 MySQL near 片段。
- `context deadline exceeded` -> `QUERY_TIMEOUT`，可重试，但建议缩小范围或改用 `sql_text_scan`。
- `broken pipe` / `connection reset by peer` -> `CONNECTION_FAILED` 或保留 `DRIVER_ERROR`，可重试。

`DRIVER_ERROR` 不再默认覆盖所有 SQL 失败。不可重试错误的 `retryable` 必须为 `false`，避免 agent 盲目重跑。

## 6. 审计分期

### P0

不改审计表结构。通过更准确的 `operation.error_code` 和工具返回错误码改善复盘基础。

### P1

新增 `audit_summary` 工具：

输入：

- `start_time`
- `end_time`
- `datasource`
- `event_type`
- `status`
- `group_by`
- `limit`

输出：

- 按日期、工具、数据源、状态、错误码聚合的数量。
- top error summaries。
- 慢查询数量分布，例如 `>=1s`、`>=5s`、`>=10s`。
- 写确认数量和执行结果摘要。

P1 目标是让复盘不再依赖手写 SQLite join。

### P2

考虑加入可选 `context_label`，用于让 agent 在调用工具时写入任务标签。该字段只作为审计辅助，不作为 MCP 会话强依赖。

## 7. 阶段拆分

### P0：MySQL 查询链路降摩擦

交付内容：

- `sql_column_search`
- `sql_text_column_plan`
- `sql_text_scan` 的 count 模式
- `sql_query` LIMIT/尾部分号/文本列归一化修正
- MySQL 错误分类和 retryable 语义修正

验收标准：

- `SHOW TABLES` 不再被包装成 `SELECT * FROM (...) AS ndb_limited LIMIT ?`。
- 尾部分号不会导致 `near ';) AS ndb_limited LIMIT ?'`。
- UTF-8 文本列不再以 Base64 展示。
- `1054`、`1146`、`1064`、超时分别映射到明确错误码。
- agent 能先用 `sql_column_search` 找字段，再执行业务查询。

### P1：审计复盘能力

交付内容：

- `audit_summary`
- README 增加审计复盘示例

验收标准：

- 不直接访问 SQLite，也能得到近 N 天失败率、top error 和慢查询分布。
- 能复现“按数据源/工具/错误码统计摩擦点”的结果。

### P2：慢查询与扫描增强

交付内容：

- `sql_text_scan` 支持更细的分批策略和部分结果返回。
- 对超时批次返回建议的缩小范围方式。

验收标准：

- 宽范围 URL/文本扫描不会一次性拼接巨大 SQL。
- 超时后能定位到具体慢字段或慢表。

## 8. 测试策略

- `backend` 单测覆盖 `applyLimit`：尾部分号、`SHOW`、`DESCRIBE`、`DESC`、`EXPLAIN`、已有 `LIMIT`、`UNION`。
- `backend` 单测覆盖结果归一化：UTF-8 `[]byte` 转字符串，非 UTF-8 返回 binary/base64 结构。
- `tools` 单测覆盖新增工具 schema、handler 入参校验、数量上限和边界值。
- `backend` 或 `tools` 单测覆盖 `information_schema` 查询参数构造。
- `nbderrors` 单测覆盖 MySQL 错误映射和 retryable 语义。
- 集成测试使用 testdata 或本地测试库，不依赖真实 support 数据。

## 9. 文档更新

README 增加：

- Agent 推荐 MySQL 查询流程。
- 新增工具说明。
- 常见错误与下一步建议。
- 文本列扫描和审计复盘示例。

实现计划需在本设计确认后单独生成，不在本设计文档中展开代码级步骤。

