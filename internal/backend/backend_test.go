package backend

import (
	"testing"
	"time"

	"native-db-bridge-mcp/internal/config"
)

// testConfigWithInvalidHost returns a config with an unreachable host
// to verify that constructors do NOT attempt to connect.
func testConfigWithInvalidHost() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			MaxResultRows:     1000,
			RedisScanCountMax: 500,
			QueryTimeout:      config.Duration{Duration: 30 * time.Second},
		},
		ConnectionLifecycle: config.ConnectionLifecycleConfig{
			Defaults: config.LifecycleDefaults{
				LazyConnect: true,
				IdleTTL:     config.Duration{Duration: 10 * time.Minute},
			},
		},
		Connections: config.ConnectionsConfig{
			SQL: []config.SQLConnectionConfig{
				{
					Name:   "invalid-sql",
					Driver: "mysql",
					DSN:    "root:invalid@tcp(192.0.2.1:99999)/?parseTime=true",
				},
			},
			Redis: []config.RedisConnectionConfig{
				{
					Name:    "invalid-redis",
					Address: "192.0.2.1:99999",
				},
			},
			Mongo: []config.MongoConnectionConfig{
				{
					Name: "invalid-mongo",
					URI:  "mongodb://192.0.2.1:99999",
				},
			},
		},
		Datasources: config.DatasourcesConfig{
			SQL: []config.SQLDatasourceConfig{
				{
					Name:            "test_sql",
					Connection:      "invalid-sql",
					DefaultDatabase: "testdb",
				},
			},
			Redis: []config.RedisDatasourceConfig{
				{
					Name:       "test_redis",
					Connection: "invalid-redis",
					DB:         0,
				},
			},
			Mongo: []config.MongoDatasourceConfig{
				{
					Name:            "test_mongo",
					Connection:      "invalid-mongo",
					DefaultDatabase: "testdb",
				},
			},
		},
	}
}

func TestSQLBackendDoesNotConnectOnCreate(t *testing.T) {
	b := NewSQLDriverBackend(testConfigWithInvalidHost())
	if b == nil {
		t.Fatalf("backend nil")
	}
	_ = b.Close()
}

func TestRedisBackendDoesNotConnectOnCreate(t *testing.T) {
	b := NewRedisDriverBackend(testConfigWithInvalidHost())
	if b == nil {
		t.Fatalf("backend nil")
	}
	_ = b.Close()
}

func TestMongoBackendDoesNotConnectOnCreate(t *testing.T) {
	b := NewMongoDriverBackend(testConfigWithInvalidHost())
	if b == nil {
		t.Fatalf("backend nil")
	}
	_ = b.Close()
}

func TestSQLBackendPingFailsWithInvalidHost(t *testing.T) {
	b := NewSQLDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	err := b.Ping(t.Context(), "test_sql")
	if err == nil {
		t.Fatal("expected ping to fail with invalid host")
	}
}

func TestRedisBackendPingFailsWithInvalidHost(t *testing.T) {
	b := NewRedisDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	err := b.Ping(t.Context(), "test_redis")
	if err == nil {
		t.Fatal("expected ping to fail with invalid host")
	}
}

func TestMongoBackendPingFailsWithInvalidHost(t *testing.T) {
	b := NewMongoDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	err := b.Ping(t.Context(), "test_mongo")
	if err == nil {
		t.Fatal("expected ping to fail with invalid host")
	}
}

func TestSQLBackendQueryFailsWithInvalidHost(t *testing.T) {
	b := NewSQLDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	_, err := b.Query(t.Context(), "test_sql", "SELECT 1", 10)
	if err == nil {
		t.Fatal("expected query to fail with invalid host")
	}
}

func TestRedisBackendCommandFailsWithInvalidHost(t *testing.T) {
	b := NewRedisDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	_, err := b.Command(t.Context(), "test_redis", "GET", []string{"key"})
	if err == nil {
		t.Fatal("expected command to fail with invalid host")
	}
}

func TestMongoBackendFindFailsWithInvalidHost(t *testing.T) {
	b := NewMongoDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	_, err := b.Find(t.Context(), MongoFindRequest{
		Datasource: "test_mongo",
		Collection: "test",
		Operation:  "find",
	})
	if err == nil {
		t.Fatal("expected find to fail with invalid host")
	}
}

func TestSQLBackendUnknownDatasource(t *testing.T) {
	b := NewSQLDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	err := b.Ping(t.Context(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown datasource")
	}
}

func TestRedisBackendUnknownDatasource(t *testing.T) {
	b := NewRedisDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	err := b.Ping(t.Context(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown datasource")
	}
}

func TestMongoBackendUnknownDatasource(t *testing.T) {
	b := NewMongoDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	err := b.Ping(t.Context(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown datasource")
	}
}

func TestSQLBackendObjectTypeList(t *testing.T) {
	b := NewSQLDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	types, err := b.ObjectTypeList(t.Context(), "test_sql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 4 {
		t.Fatalf("expected 4 object types, got %d", len(types))
	}
}

func TestMongoListDatabasesReturnsDefault(t *testing.T) {
	b := NewMongoDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	dbs, err := b.ListDatabases(t.Context(), "test_mongo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dbs) != 1 || dbs[0] != "testdb" {
		t.Fatalf("expected [testdb], got %v", dbs)
	}
}

func TestMongoListDatabasesUnknownDatasource(t *testing.T) {
	b := NewMongoDriverBackend(testConfigWithInvalidHost())
	defer b.Close()

	_, err := b.ListDatabases(t.Context(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown datasource")
	}
}

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		database string
		want     string
	}{
		{
			name:     "empty database",
			base:     "root:pass@tcp(host:3306)/?parseTime=true",
			database: "",
			want:     "root:pass@tcp(host:3306)/?parseTime=true",
		},
		{
			name:     "with trailing slash and query params",
			base:     "root:pass@tcp(host:3306)/?parseTime=true",
			database: "mydb",
			want:     "root:pass@tcp(host:3306)/mydb?parseTime=true",
		},
		{
			name:     "without trailing slash",
			base:     "root:pass@tcp(host:3306)?parseTime=true",
			database: "mydb",
			want:     "root:pass@tcp(host:3306)/mydb?parseTime=true",
		},
		{
			name:     "without query params",
			base:     "root:pass@tcp(host:3306)/",
			database: "mydb",
			want:     "root:pass@tcp(host:3306)/mydb",
		},
		{
			name:     "without trailing slash and no params",
			base:     "root:pass@tcp(host:3306)",
			database: "mydb",
			want:     "root:pass@tcp(host:3306)/mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDSN(tt.base, tt.database)
			if got != tt.want {
				t.Fatalf("buildDSN(%q, %q) = %q, want %q", tt.base, tt.database, got, tt.want)
			}
		})
	}
}

func TestApplyLimit(t *testing.T) {
	tests := []struct {
		name           string
		sql            string
		limit          int
		wantSQL        string
		wantHasParam   bool
	}{
		{
			name:         "no existing limit",
			sql:          "SELECT * FROM users",
			limit:        100,
			wantSQL:      "SELECT * FROM (SELECT * FROM users) AS ndb_limited LIMIT ?",
			wantHasParam: true,
		},
		{
			name:         "already has limit",
			sql:          "SELECT * FROM users LIMIT 50",
			limit:        100,
			wantSQL:      "SELECT * FROM users LIMIT 50",
			wantHasParam: false,
		},
		{
			name:         "lowercase limit",
			sql:          "select * from users limit 10",
			limit:        100,
			wantSQL:      "select * from users limit 10",
			wantHasParam: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotHasParam := applyLimit(tt.sql, tt.limit)
			if gotSQL != tt.wantSQL {
				t.Fatalf("applyLimit(%q, %d) sql = %q, want %q", tt.sql, tt.limit, gotSQL, tt.wantSQL)
			}
			if gotHasParam != tt.wantHasParam {
				t.Fatalf("applyLimit(%q, %d) hasParam = %v, want %v", tt.sql, tt.limit, gotHasParam, tt.wantHasParam)
			}
		})
	}
}
