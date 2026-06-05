// Package app wires all components into a runnable application.
package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"native-db-bridge-mcp/internal/audit"
	"native-db-bridge-mcp/internal/backend"
	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/ops"
	"native-db-bridge-mcp/internal/server"
	"native-db-bridge-mcp/internal/tools"
)

// App holds all wired components of the native-db-bridge application.
type App struct {
	cfg    *config.Config
	store  *audit.Store
	sql    *backend.SQLDriverBackend
	redis  *backend.RedisDriverBackend
	mongo  *backend.MongoDriverBackend
	ops    *ops.Tracker
	srv    *server.Server
	cancel context.CancelFunc // cancels background goroutines
}

// New loads configuration from cfgPath and creates all application
// components. Database connections (SQL, Redis, Mongo) are NOT opened
// at startup -- they are managed lazily by the lifecycle manager.
func New(cfgPath string) (*App, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("app: load config: %w", err)
	}
	return newFromConfig(cfg)
}

// NewForTest creates an App from the given config path. It is identical
// to New but named to signal test-only usage.
func NewForTest(cfgPath string) (*App, error) {
	return New(cfgPath)
}

// newFromConfig constructs an App from an already-loaded Config.
func newFromConfig(cfg *config.Config) (*App, error) {
	store, err := audit.Open(cfg.Storage.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("app: open audit store: %w", err)
	}

	sqlBackend := backend.NewSQLDriverBackend(*cfg)
	redisBackend := backend.NewRedisDriverBackend(*cfg)
	mongoBackend := backend.NewMongoDriverBackend(*cfg)
	opsTracker := ops.NewTracker()

	handlers := tools.NewHandlers(tools.Deps{
		Config: *cfg,
		Audit:  store,
		SQL:    sqlBackend,
		Redis:  redisBackend,
		Mongo:  mongoBackend,
		Ops:    opsTracker,
	})

	srv := server.NewServer(cfg.Server, handlers)

	ctx, cancel := context.WithCancel(context.Background())

	a := &App{
		cfg:    cfg,
		store:  store,
		sql:    sqlBackend,
		redis:  redisBackend,
		mongo:  mongoBackend,
		ops:    opsTracker,
		srv:    srv,
		cancel: cancel,
	}

	// Start background confirmation expiry scanner if configured.
	if cfg.Policy.ConfirmationExpireScanInterval.Duration > 0 {
		go a.runExpiryScanner(ctx, cfg.Policy.ConfirmationExpireScanInterval.Duration)
	}

	return a, nil
}

// runExpiryScanner periodically marks expired pending confirmations.
// It stops when ctx is cancelled.
func (a *App) runExpiryScanner(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := a.store.MarkExpiredConfirmations(time.Now())
			if err != nil {
				log.Printf("expiry scanner: %v", err)
				continue
			}
			if rows > 0 {
				log.Printf("expiry scanner: marked %d confirmation(s) as expired", rows)
			}
		}
	}
}

// BusinessConnectionCount returns the total number of lazily-created
// backend connections currently tracked. After construction this is
// always zero because no backend connects until the first Acquire.
func (a *App) BusinessConnectionCount() int {
	return a.sql.ActiveConnections() +
		a.redis.ActiveConnections() +
		a.mongo.ActiveConnections()
}

// Server returns the HTTP/MCP server.
func (a *App) Server() *server.Server {
	return a.srv
}

// Config returns the loaded configuration.
func (a *App) Config() *config.Config {
	return a.cfg
}

// Close gracefully shuts down all components in reverse creation order.
// Background goroutines are stopped first via context cancellation.
func (a *App) Close() error {
	a.cancel() // stop background goroutines

	var firstErr error

	setFirst := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	setFirst(a.srv.Close())
	setFirst(a.mongo.Close())
	setFirst(a.redis.Close())
	setFirst(a.sql.Close())
	setFirst(a.store.Close())

	return firstErr
}
