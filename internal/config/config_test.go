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

func TestRejectProductionDatasource(t *testing.T) {
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
