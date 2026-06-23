# native-db-bridge-mcp

[中文](README.md)

A locally resident MCP server that replaces DataGrip MCP as the database operation entry point for development and testing environments.

## Why

DataGrip MCP requires DataGrip to run persistently, incurring extra memory, CPU, and IDE lag. When agents already access databases through MCP, the value of keeping DataGrip resident drops significantly.

This project provides a low-resource, single-process MCP server built with Go, allowing multiple Codex/Claude sessions to share a single local instance.

## Features

- **SQL** — MySQL queries, DDL/DML preparation and execution
- **Redis** — Logical namespace isolation, read-only command whitelist, write operations require confirmation
- **MongoDB** — find, aggregate (stage whitelist), write operations require confirmation
- **Two-phase write confirmation** — All writes go through `prepare` to generate a confirmation, then `execute_confirmation` to actually execute
- **Audit** — All operations logged to a local SQLite audit database
- **Security** — Listens on `127.0.0.1` only, Bearer token auth, config file permission check (`0600`)
- **Production isolation** — Rejects production data sources, only allows dev/support environments

## Tech Stack

- Go 1.23+
- Official MCP Go SDK (Streamable HTTP)
- `modernc.org/sqlite` (pure Go SQLite)
- `github.com/go-sql-driver/mysql`
- `github.com/redis/go-redis/v9`
- `go.mongodb.org/mongo-driver/mongo`
- `github.com/xwb1989/sqlparser`

## Quick Start

### 1. Configure

```bash
mkdir -p var
cp configs/config.example.yaml var/config.yaml
chmod 600 var/config.yaml
# Edit var/config.yaml with your database connections and client_token
```

### 2. Build & Run

```bash
go build -o native-db-bridge-mcp ./cmd/native-db-bridge-mcp
./native-db-bridge-mcp serve --home ./var
```

Or using an environment variable:

```bash
NATIVE_DB_BRIDGE_HOME=./var ./native-db-bridge-mcp serve
```

### 3. Install as launchd Service (macOS)

```bash
./native-db-bridge-mcp install-service --home ./var
```

### 4. Health Check

```bash
# Config and driver check only
./native-db-bridge-mcp healthcheck

# Real connection test
./native-db-bridge-mcp healthcheck --connect
```

## MCP Client Configuration

Add to your Codex or Claude Code MCP configuration:

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

## MCP Tools

### Metadata

| Tool | Description |
|------|-------------|
| `datasource_list` | List available data sources |
| `datasource_healthcheck` | Data source health check |
| `sql_schema_list` | List SQL schemas |
| `sql_object_type_list` | List object types (table/view/procedure/function) |
| `sql_object_list` | List objects in a schema |
| `sql_object_describe` | Describe table/view/stored procedure structure |
| `sql_table_preview` | Preview table data |
| `sql_column_search` | Search MySQL column metadata |
| `sql_text_column_plan` | Build a text-column scan plan without reading business rows |
| `redis_key_scan` | SCAN Redis keys |
| `redis_key_describe` | Describe Redis key type, TTL, and length |
| `mongo_database_list` | List MongoDB databases |
| `mongo_collection_list` | List collections |
| `mongo_collection_describe` | Describe collection indexes and sample schema |

### Execution

| Tool | Description |
|------|-------------|
| `sql_query` | Read-only SQL queries (SELECT/SHOW/DESC/EXPLAIN) |
| `sql_text_scan` | Run count-only scans on selected text columns |
| `sql_prepare_change` | Prepare a SQL write operation, returns confirmation |
| `redis_command` | Redis read-only commands |
| `redis_prepare_change` | Prepare a Redis write operation, returns confirmation |
| `mongo_find` | MongoDB read operations (find/aggregate) |
| `mongo_prepare_change` | Prepare a MongoDB write operation, returns confirmation |
| `execute_confirmation` | Execute a confirmed write operation |

### Control & Audit

| Tool | Description |
|------|-------------|
| `operation_list` | List running and recent operations |
| `cancel_operation` | Cancel a running operation |
| `audit_recent` | Query recent audit events |
| `audit_summary` | Summarize audit events for failures and slow queries |
| `confirmation_get` | Query confirmation status |
| `cancel_confirmation` | Cancel a pending confirmation |

## Agent MySQL Query Workflow

Agents should query MySQL in this order:

1. `datasource_list`: confirm the target datasource and default database.
2. `sql_column_search`: search real columns by table, column, or data type before guessing names.
3. `sql_text_column_plan`: build candidate text columns and scan batches before searching URLs, images, or text content.
4. `sql_text_scan`: run count-only scans on confirmed fields.
5. `sql_query`: query details after fields and scope are clear.
6. `audit_summary`: review failures, slow queries, and error codes after a task.

Common error handling:

- `SQL_UNKNOWN_COLUMN`: use `sql_column_search` first.
- `SQL_UNKNOWN_TABLE`: use `sql_object_list` first.
- `QUERY_SYNTAX_ERROR`: fix the SQL; do not retry blindly.
- `QUERY_TIMEOUT`: narrow the scope, or split the search with `sql_text_column_plan` and `sql_text_scan`.

If system diagnostics such as `performance_schema`, `SHOW PROCESSLIST`, or `SHOW GLOBAL STATUS` fail because of permissions, agents should preserve the structured error and switch to ordinary schema metadata tools or controlled text scans. Higher-privilege diagnostics require explicit user confirmation; do not keep retrying the same permission-limited statement.

## Write Operation Flow

```
prepare_change → returns confirmation_id (pending)
                    ↓
         Agent displays risk summary, waits for user confirmation
                    ↓
execute_confirmation(confirmation_id) → actually executes
```

- Confirmations expire after 10 minutes by default, auto-marked as `expired`
- Pending confirmations can be cancelled anytime via `cancel_confirmation`
- All state transitions are written to the audit log

## Project Structure

```text
cmd/native-db-bridge-mcp/    Entry point
configs/                      Example configuration
internal/
  app/                        Application assembly
  audit/                      SQLite audit store
  backend/                    SQL/Redis/Mongo backend interfaces & implementations
  cli/                        CLI commands (serve/healthcheck/install-service)
  config/                     Config loading & validation
  lifecycle/                  Lazy connection loading & idle reclamation
  model/                      Shared data models
  nbderrors/                  Structured error codes
  ops/                        Operation tracker (cancel function management)
  policy/                     Read/write classification, safety whitelists, risk assessment
  server/                     Streamable HTTP MCP, authentication
  tools/                      MCP tool schemas & handlers
testdata/                     Test fixtures
var/                          Runtime files (gitignored)
```

## Security Design

- Config file permissions must be `0600`, otherwise the server refuses to start
- Audit database `audit.db` also enforces `0600` permissions
- Listens on `127.0.0.1` only, no network exposure
- SQL uses a parser to identify statement types, rejects `FOR UPDATE`, `LOAD_FILE`, `INTO OUTFILE`, etc.
- Redis rejects unknown commands by default, only allows whitelisted operations
- MongoDB aggregate only allows safe stages (`$match`/`$project`/`$sort`, etc.)
- DSN enforces `multiStatements` disabled
- Production data sources are rejected at load time

## Documentation

- [Design Document](docs/superpowers/specs/2026-06-05-native-db-bridge-mcp-design.md)
- [Implementation Plan](docs/superpowers/plans/2026-06-05-native-db-bridge-mcp-implementation.md)

## License

Private
