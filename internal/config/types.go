package config

import (
	"fmt"
	"time"
)

// Duration wraps time.Duration with YAML unmarshaling support for strings like "60s", "10m".
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return fmt.Errorf("invalid duration value: %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// Config represents the application configuration loaded from YAML.
type Config struct {
	Server              ServerConfig              `yaml:"server"`
	Policy              PolicyConfig              `yaml:"policy"`
	ConnectionLifecycle ConnectionLifecycleConfig `yaml:"connection_lifecycle"`
	Storage             StorageConfig             `yaml:"storage"`
	Connections         ConnectionsConfig         `yaml:"connections"`
	Datasources         DatasourcesConfig         `yaml:"datasources"`
}

// ServerConfig holds HTTP server and MCP settings.
type ServerConfig struct {
	Listen            string   `yaml:"listen"`
	MCPPath           string   `yaml:"mcp_path"`
	Transport         string   `yaml:"transport"`
	ClientToken       string   `yaml:"client_token"`
	RequestTimeout    Duration `yaml:"request_timeout"`
	QueryTimeout      Duration `yaml:"query_timeout"`
	MaxResultRows     int      `yaml:"max_result_rows"`
	RedisScanCountMax int      `yaml:"redis_scan_count_max"`
}

// PolicyConfig holds security and write-confirmation policies.
type PolicyConfig struct {
	AllowedEnvironments            []string `yaml:"allowed_environments"`
	ProductionEnabled              bool     `yaml:"production_enabled"`
	ConfirmAllWrites               bool     `yaml:"confirm_all_writes"`
	RejectWriteWithoutConfirmation bool     `yaml:"reject_write_without_confirmation"`
	ConfirmationTTL                Duration `yaml:"confirmation_ttl"`
	ConfirmationExpireScanInterval Duration `yaml:"confirmation_expire_scan_interval"`
}

// ConnectionLifecycleConfig holds connection pool lifecycle settings.
type ConnectionLifecycleConfig struct {
	Defaults LifecycleDefaults `yaml:"defaults"`
	SQL      LifecycleOverride `yaml:"sql"`
	Redis    LifecycleOverride `yaml:"redis"`
	Mongo    LifecycleOverride `yaml:"mongo"`
}

// LifecycleDefaults holds default lifecycle settings for all connection types.
type LifecycleDefaults struct {
	LazyConnect       bool     `yaml:"lazy_connect"`
	IdleTTL           Duration `yaml:"idle_ttl"`
	CloseScanInterval Duration `yaml:"close_scan_interval"`
	ConnectTimeout    Duration `yaml:"connect_timeout"`
}

// LifecycleOverride holds per-connection-type lifecycle overrides.
type LifecycleOverride struct {
	IdleTTL Duration `yaml:"idle_ttl"`
}

// StorageConfig holds file paths for SQLite and logging.
type StorageConfig struct {
	SQLitePath string `yaml:"sqlite_path"`
	LogPath    string `yaml:"log_path"`
}

// ConnectionsConfig groups connection definitions by type.
type ConnectionsConfig struct {
	SQL   []SQLConnectionConfig   `yaml:"sql"`
	Redis []RedisConnectionConfig `yaml:"redis"`
	Mongo []MongoConnectionConfig `yaml:"mongo"`
}

// SQLConnectionConfig defines a SQL database connection.
type SQLConnectionConfig struct {
	Name        string     `yaml:"name"`
	Environment string     `yaml:"environment"`
	Driver      string     `yaml:"driver"`
	DSN         string     `yaml:"dsn"`
	Pool        SQLPoolCfg `yaml:"pool"`
}

// SQLPoolCfg holds SQL connection pool settings.
type SQLPoolCfg struct {
	MaxOpenConns    int      `yaml:"max_open_conns"`
	MaxIdleConns    int      `yaml:"max_idle_conns"`
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime"`
}

// RedisConnectionConfig defines a Redis connection.
type RedisConnectionConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	Address     string `yaml:"address"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	TLS         bool   `yaml:"tls"`
}

// MongoConnectionConfig defines a MongoDB connection.
type MongoConnectionConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	URI         string `yaml:"uri"`
}

// DatasourcesConfig groups datasource definitions by type.
type DatasourcesConfig struct {
	SQL   []SQLDatasourceConfig   `yaml:"sql"`
	Redis []RedisDatasourceConfig `yaml:"redis"`
	Mongo []MongoDatasourceConfig `yaml:"mongo"`
}

// SQLDatasourceConfig defines a SQL datasource that references a connection.
type SQLDatasourceConfig struct {
	Name            string `yaml:"name"`
	Environment     string `yaml:"environment"`
	Connection      string `yaml:"connection"`
	DefaultDatabase string `yaml:"default_database"`
}

// RedisDatasourceConfig defines a Redis datasource that references a connection.
type RedisDatasourceConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	Connection  string `yaml:"connection"`
	DB          int    `yaml:"db"`
	Service     string `yaml:"service"`
}

// MongoDatasourceConfig defines a MongoDB datasource that references a connection.
type MongoDatasourceConfig struct {
	Name            string `yaml:"name"`
	Environment     string `yaml:"environment"`
	Connection      string `yaml:"connection"`
	DefaultDatabase string `yaml:"default_database"`
}
