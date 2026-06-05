package app

import (
	"os"
	"path/filepath"
	"testing"
)

// validConfigPath creates a temporary config file with 0600 permissions
// that passes validation. The SQLite path is set to a file in the same
// temp directory so the audit store can be opened.
func validConfigPath(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cfg := `
server:
  listen: "127.0.0.1:9090"
  mcp_path: "/mcp"
  transport: "streamable_http"
  client_token: "test-token-123"
  request_timeout: "60s"
  query_timeout: "30s"
  max_result_rows: 1000
  redis_scan_count_max: 100

policy:
  allowed_environments:
    - "dev"
    - "support"
  production_enabled: false
  confirm_all_writes: true
  reject_write_without_confirmation: true
  confirmation_ttl: "10m"
  confirmation_expire_scan_interval: "30m"

connection_lifecycle:
  defaults:
    lazy_connect: true
    idle_ttl: "5m"
    close_scan_interval: "1m"
    connect_timeout: "10s"
  sql:
    idle_ttl: "10m"
  redis:
    idle_ttl: "3m"
  mongo:
    idle_ttl: "5m"

storage:
  sqlite_path: "` + filepath.Join(dir, "audit.db") + `"
  log_path: "` + filepath.Join(dir, "app.log") + `"

connections:
  sql:
    - name: "dev-pg"
      environment: "dev"
      driver: "postgres"
      dsn: "host=localhost port=5432 user=test dbname=testdb sslmode=disable"
      pool:
        max_open_conns: 10
        max_idle_conns: 5
        conn_max_lifetime: "30m"
  redis:
    - name: "dev-redis"
      environment: "dev"
      address: "localhost:6379"
      username: ""
      password: "fake-password"
      tls: false
  mongo:
    - name: "dev-mongo"
      environment: "dev"
      uri: "mongodb://localhost:27017"

datasources:
  sql:
    - name: "dev-postgres"
      environment: "dev"
      connection: "dev-pg"
      default_database: "testdb"
  redis:
    - name: "dev-redis-cache"
      environment: "dev"
      connection: "dev-redis"
      db: 0
      service: "cache"
  mongo:
    - name: "dev-mongo-db"
      environment: "dev"
      connection: "dev-mongo"
      default_database: "analytics"
`

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

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

func TestNewAppLoadsConfig(t *testing.T) {
	app, err := NewForTest(validConfigPath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	cfg := app.Config()
	if cfg == nil {
		t.Fatal("config is nil")
	}
	if cfg.Server.Transport != "streamable_http" {
		t.Fatalf("transport=%q, want streamable_http", cfg.Server.Transport)
	}
}

func TestNewAppRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("invalid: true\n"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := NewForTest(path)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestNewAppRejectsMissingConfig(t *testing.T) {
	_, err := NewForTest("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}
