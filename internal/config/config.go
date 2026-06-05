package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// maxConfigPerm is the maximum allowed file permission for config files.
	// Only owner should have read access (0600); group and others must have no access.
	maxConfigPerm = os.FileMode(0600)

	allowedTransport = "streamable_http"
)

// allowedEnvironments defines the set of permitted datasource environments.
var allowedEnvironments = map[string]bool{
	"dev":     true,
	"support": true,
}

// Load reads, parses, and validates the configuration from the given path.
func Load(path string) (*Config, error) {
	if err := checkFilePermissions(path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// checkFilePermissions verifies the config file has restrictive permissions.
// The file must not be group-readable or world-readable (max 0600).
func checkFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	perm := info.Mode().Perm()
	if perm > maxConfigPerm {
		return fmt.Errorf(
			"config file %q has too-open permissions %04o; must be %04o or more restrictive",
			path, perm, maxConfigPerm,
		)
	}

	return nil
}

// validate checks all configuration rules.
func validate(cfg *Config) error {
	if err := validateTransport(cfg); err != nil {
		return err
	}
	if err := validatePolicy(cfg); err != nil {
		return err
	}
	if err := validateDatasources(cfg); err != nil {
		return err
	}
	if err := validateConnectionRefs(cfg); err != nil {
		return err
	}
	if err := validateSQLDSN(cfg); err != nil {
		return err
	}
	return nil
}

// validateTransport ensures the transport is "streamable_http".
func validateTransport(cfg *Config) error {
	if cfg.Server.Transport != allowedTransport {
		return fmt.Errorf(
			"unsupported transport %q; only %q is allowed",
			cfg.Server.Transport, allowedTransport,
		)
	}
	return nil
}

// validatePolicy ensures production mode is disabled.
func validatePolicy(cfg *Config) error {
	if cfg.Policy.ProductionEnabled {
		return fmt.Errorf("production_enabled must be false; production mode is not allowed")
	}
	return nil
}

// validateDatasources ensures all datasource environments are in the allowed set.
func validateDatasources(cfg *Config) error {
	for _, ds := range cfg.Datasources.SQL {
		if err := checkEnvironment(ds.Name, ds.Environment); err != nil {
			return err
		}
	}
	for _, ds := range cfg.Datasources.Redis {
		if err := checkEnvironment(ds.Name, ds.Environment); err != nil {
			return err
		}
	}
	for _, ds := range cfg.Datasources.Mongo {
		if err := checkEnvironment(ds.Name, ds.Environment); err != nil {
			return err
		}
	}
	return nil
}

// checkEnvironment verifies a datasource environment is in the allowed set.
func checkEnvironment(name, env string) error {
	if !allowedEnvironments[env] {
		return fmt.Errorf(
			"datasource %q has disallowed environment %q; allowed: dev, support",
			name, env,
		)
	}
	return nil
}

// validateConnectionRefs ensures each datasource references an existing connection.
func validateConnectionRefs(cfg *Config) error {
	sqlConns := make(map[string]bool, len(cfg.Connections.SQL))
	for _, c := range cfg.Connections.SQL {
		sqlConns[c.Name] = true
	}
	for _, ds := range cfg.Datasources.SQL {
		if !sqlConns[ds.Connection] {
			return fmt.Errorf(
				"datasource %q references unknown SQL connection %q",
				ds.Name, ds.Connection,
			)
		}
	}

	redisConns := make(map[string]bool, len(cfg.Connections.Redis))
	for _, c := range cfg.Connections.Redis {
		redisConns[c.Name] = true
	}
	for _, ds := range cfg.Datasources.Redis {
		if !redisConns[ds.Connection] {
			return fmt.Errorf(
				"datasource %q references unknown Redis connection %q",
				ds.Name, ds.Connection,
			)
		}
	}

	mongoConns := make(map[string]bool, len(cfg.Connections.Mongo))
	for _, c := range cfg.Connections.Mongo {
		mongoConns[c.Name] = true
	}
	for _, ds := range cfg.Datasources.Mongo {
		if !mongoConns[ds.Connection] {
			return fmt.Errorf(
				"datasource %q references unknown MongoDB connection %q",
				ds.Name, ds.Connection,
			)
		}
	}

	return nil
}

// validateSQLDSN ensures SQL DSN strings do not contain multiStatements=true.
func validateSQLDSN(cfg *Config) error {
	for _, conn := range cfg.Connections.SQL {
		if strings.Contains(strings.ToLower(conn.DSN), "multistatements=true") {
			return fmt.Errorf(
				"SQL connection %q DSN must not contain multiStatements=true",
				conn.Name,
			)
		}
	}
	return nil
}
