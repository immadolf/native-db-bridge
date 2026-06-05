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

func TestRejectDisallowedEnvironment(t *testing.T) {
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

func TestRejectMultiStatementsInDSN(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := `
server:
  listen: "127.0.0.1:9090"
  mcp_path: "/mcp"
  transport: "streamable_http"
  client_token: "test-token"
  request_timeout: "60s"
  query_timeout: "30s"
  max_result_rows: 1000
  redis_scan_count_max: 100
policy:
  allowed_environments: ["dev"]
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
storage:
  sqlite_path: "/tmp/test.db"
  log_path: "/tmp/test.log"
connections:
  sql:
    - name: "bad-dsn"
      environment: "dev"
      driver: "mysql"
      dsn: "test:test@tcp(localhost:3306)/db?multiStatements=true"
datasources:
  sql:
    - name: "bad-ds"
      environment: "dev"
      connection: "bad-dsn"
`
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load() expected multiStatements rejection")
	}
}

func TestRejectMissingConnectionReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := `
server:
  listen: "127.0.0.1:9090"
  mcp_path: "/mcp"
  transport: "streamable_http"
  client_token: "test-token"
  request_timeout: "60s"
  query_timeout: "30s"
  max_result_rows: 1000
  redis_scan_count_max: 100
policy:
  allowed_environments: ["dev"]
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
storage:
  sqlite_path: "/tmp/test.db"
  log_path: "/tmp/test.log"
connections:
  sql:
    - name: "real-conn"
      environment: "dev"
      driver: "postgres"
      dsn: "host=localhost"
datasources:
  sql:
    - name: "orphan-ds"
      environment: "dev"
      connection: "nonexistent-conn"
`
	if err := os.WriteFile(path, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatalf("Load() expected missing connection reference error")
	}
}
