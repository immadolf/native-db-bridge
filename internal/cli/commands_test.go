package cli

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestResolveHomeExplicitFlag(t *testing.T) {
	got := resolveHome("/explicit/path")
	if got != "/explicit/path" {
		t.Fatalf("resolveHome(explicit)=%q, want /explicit/path", got)
	}
}

func TestResolveHomeEnvVar(t *testing.T) {
	t.Setenv("NATIVE_DB_BRIDGE_HOME", "/env/path")
	got := resolveHome("")
	if got != "/env/path" {
		t.Fatalf("resolveHome(env)=%q, want /env/path", got)
	}
}

func TestResolveHomeDefault(t *testing.T) {
	t.Setenv("NATIVE_DB_BRIDGE_HOME", "")
	got := resolveHome("")
	if got != "./var" {
		t.Fatalf("resolveHome(default)=%q, want ./var", got)
	}
}

func TestResolveHomeFlagOverridesEnv(t *testing.T) {
	t.Setenv("NATIVE_DB_BRIDGE_HOME", "/env/path")
	got := resolveHome("/flag/path")
	if got != "/flag/path" {
		t.Fatalf("resolveHome(flag-overrides-env)=%q, want /flag/path", got)
	}
}

// writeTestConfig creates a valid config file in a temp directory and
// returns the directory path suitable for --home.
func writeTestConfig(t *testing.T) string {
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
  redis:
    - name: "dev-redis"
      environment: "dev"
      address: "localhost:6379"
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
  mongo:
    - name: "dev-mongo-db"
      environment: "dev"
      connection: "dev-mongo"
      default_database: "analytics"
`
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestHealthcheckConfigOK(t *testing.T) {
	home := writeTestConfig(t)
	err := Healthcheck([]string{"--home", home})
	if err != nil {
		t.Fatalf("Healthcheck() error = %v", err)
	}
}

func TestHealthcheckMissingConfig(t *testing.T) {
	err := Healthcheck([]string{"--home", "/nonexistent/path"})
	if err == nil {
		t.Fatal("Healthcheck() expected error for missing config")
	}
}

func TestHealthcheckConnectFailsGracefully(t *testing.T) {
	home := writeTestConfig(t)
	err := Healthcheck([]string{"--home", home, "--connect"})
	// Connect will fail because backends are unreachable, but it should
	// return an error rather than panic.
	if err == nil {
		t.Fatal("Healthcheck(connect) expected error for unreachable backends")
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	code := Dispatch([]string{"native-db-bridge-mcp", "unknown-cmd"})
	if code != 2 {
		t.Fatalf("Dispatch(unknown) = %d, want 2", code)
	}
}

func TestDispatchNoArgs(t *testing.T) {
	code := Dispatch([]string{"native-db-bridge-mcp"})
	if code != 2 {
		t.Fatalf("Dispatch(no-args) = %d, want 2", code)
	}
}

func TestDispatchHealthcheckOK(t *testing.T) {
	home := writeTestConfig(t)
	code := Dispatch([]string{"native-db-bridge-mcp", "healthcheck", "--home", home})
	if code != 0 {
		t.Fatalf("Dispatch(healthcheck) = %d, want 0", code)
	}
}
