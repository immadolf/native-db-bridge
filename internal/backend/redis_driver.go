package backend

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/lifecycle"
)

// redisResource wraps a *redis.Client to satisfy lifecycle.Resource.
type redisResource struct {
	client *redis.Client
}

func (r *redisResource) Close() error {
	return r.client.Close()
}

// RedisDriverBackend implements RedisBackend using go-redis/v9.
// Each datasource namespace maps to a single redis.Client with the
// DB index fixed to the datasource's configured DB. SELECT is never
// sent. Connections are managed lazily via lifecycle.Manager.
type RedisDriverBackend struct {
	cfg     config.Config
	manager *lifecycle.Manager[string]
}

// NewRedisDriverBackend creates a Redis backend that defers connection
// until the first operation. The constructor never opens a connection.
func NewRedisDriverBackend(cfg config.Config) *RedisDriverBackend {
	idleTTL := cfg.ConnectionLifecycle.Defaults.IdleTTL.Duration
	if cfg.ConnectionLifecycle.Redis.IdleTTL.Duration > 0 {
		idleTTL = cfg.ConnectionLifecycle.Redis.IdleTTL.Duration
	}
	if idleTTL == 0 {
		idleTTL = 5 * time.Minute
	}

	factory := func(ctx context.Context, datasource string) (lifecycle.Resource, error) {
		connCfg, dsCfg, err := findRedisConnectionAndDatasource(cfg, datasource)
		if err != nil {
			return nil, err
		}

		opts := &redis.Options{
			Addr: connCfg.Address,
			DB:   dsCfg.DB,
		}
		if connCfg.Username != "" {
			opts.Username = connCfg.Username
		}
		if connCfg.Password != "" {
			opts.Password = connCfg.Password
		}
		if connCfg.TLS {
			opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}

		client := redis.NewClient(opts)

		if err := client.Ping(ctx).Err(); err != nil {
			client.Close()
			return nil, fmt.Errorf("redis ping %s: %w", datasource, err)
		}

		return &redisResource{client: client}, nil
	}

	return &RedisDriverBackend{
		cfg:     cfg,
		manager: lifecycle.NewManager(idleTTL, factory),
	}
}

// Close shuts down the lifecycle manager and all managed connections.
func (b *RedisDriverBackend) Close() error {
	return b.manager.Close()
}

// Ping verifies connectivity to the given datasource.
func (b *RedisDriverBackend) Ping(ctx context.Context, datasource string) error {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return err
	}
	release()
	return nil
}

// Command executes a single Redis command and returns the raw result.
func (b *RedisDriverBackend) Command(ctx context.Context, datasource, command string, args []string) (RedisResult, error) {
	start := time.Now()

	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return RedisResult{}, err
	}
	defer release()

	client, err := b.getClient(datasource)
	if err != nil {
		return RedisResult{}, err
	}

	cmdArgs := make([]interface{}, 0, len(args)+1)
	cmdArgs = append(cmdArgs, command)
	for _, a := range args {
		cmdArgs = append(cmdArgs, a)
	}

	cmd := redis.NewCmd(ctx, cmdArgs...)
	client.Process(ctx, cmd)

	if cmd.Err() != nil {
		return RedisResult{}, fmt.Errorf("redis command %s: %w", datasource, cmd.Err())
	}

	return RedisResult{
		Result:  cmd.Val(),
		Elapsed: time.Since(start),
	}, nil
}

// ScanKeys iterates keys matching the pattern using SCAN with cursor.
func (b *RedisDriverBackend) ScanKeys(ctx context.Context, datasource, match, cursor string, count int) (RedisScanResult, error) {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return RedisScanResult{}, err
	}
	defer release()

	client, err := b.getClient(datasource)
	if err != nil {
		return RedisScanResult{}, err
	}

	cursorVal, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		return RedisScanResult{}, fmt.Errorf("invalid cursor %q: %w", cursor, err)
	}

	if count <= 0 {
		count = 100
	}
	maxCount := b.cfg.Server.RedisScanCountMax
	if maxCount > 0 && count > maxCount {
		count = maxCount
	}

	keys, nextCursor, err := client.Scan(ctx, cursorVal, match, int64(count)).Result()
	if err != nil {
		return RedisScanResult{}, fmt.Errorf("redis scan %s: %w", datasource, err)
	}

	truncated := nextCursor != 0

	return RedisScanResult{
		Keys:       keys,
		NextCursor: strconv.FormatUint(nextCursor, 10),
		Truncated:  truncated,
	}, nil
}

// KeyDescribe returns metadata about a single Redis key by combining
// EXISTS, TYPE, PTTL and type-specific length commands.
func (b *RedisDriverBackend) KeyDescribe(ctx context.Context, datasource, key string) (RedisKeyDescription, error) {
	release, err := b.manager.Acquire(ctx, datasource)
	if err != nil {
		return RedisKeyDescription{}, err
	}
	defer release()

	client, err := b.getClient(datasource)
	if err != nil {
		return RedisKeyDescription{}, err
	}

	pipe := client.Pipeline()
	existsCmd := pipe.Exists(ctx, key)
	typeCmd := pipe.Type(ctx, key)
	ttlCmd := pipe.PTTL(ctx, key)

	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return RedisKeyDescription{}, fmt.Errorf("redis key describe pipeline %s: %w", datasource, err)
	}

	desc := RedisKeyDescription{
		Key:    key,
		Exists: existsCmd.Val() > 0,
		Type:   typeCmd.Val(),
		TTL:    ttlCmd.Val().Milliseconds(),
	}

	if desc.Exists {
		desc.Length = b.keyLength(ctx, client, key, desc.Type)
	}

	return desc, nil
}

// keyLength returns the type-specific length of a key.
func (b *RedisDriverBackend) keyLength(ctx context.Context, client *redis.Client, key, keyType string) int64 {
	switch keyType {
	case "string":
		return client.StrLen(ctx, key).Val()
	case "list":
		return client.LLen(ctx, key).Val()
	case "set":
		return client.SCard(ctx, key).Val()
	case "zset":
		return client.ZCard(ctx, key).Val()
	case "hash":
		return client.HLen(ctx, key).Val()
	case "stream":
		return client.XLen(ctx, key).Val()
	default:
		return 0
	}
}

// getClient returns the underlying *redis.Client for a datasource.
func (b *RedisDriverBackend) getClient(datasource string) (*redis.Client, error) {
	res, ok := b.manager.Get(datasource)
	if !ok {
		return nil, fmt.Errorf("datasource %q not found in lifecycle manager", datasource)
	}
	redisRes, ok := res.(*redisResource)
	if !ok {
		return nil, fmt.Errorf("unexpected resource type for datasource %q", datasource)
	}
	return redisRes.client, nil
}

// findRedisConnectionAndDatasource looks up the connection config and
// datasource config for a given datasource name.
func findRedisConnectionAndDatasource(cfg config.Config, datasource string) (config.RedisConnectionConfig, config.RedisDatasourceConfig, error) {
	var dsCfg config.RedisDatasourceConfig
	found := false
	for _, ds := range cfg.Datasources.Redis {
		if ds.Name == datasource {
			dsCfg = ds
			found = true
			break
		}
	}
	if !found {
		return config.RedisConnectionConfig{}, config.RedisDatasourceConfig{}, fmt.Errorf("datasource %q not found", datasource)
	}

	for _, conn := range cfg.Connections.Redis {
		if conn.Name == dsCfg.Connection {
			return conn, dsCfg, nil
		}
	}

	return config.RedisConnectionConfig{}, config.RedisDatasourceConfig{}, fmt.Errorf("connection %q not found for datasource %q", dsCfg.Connection, datasource)
}
