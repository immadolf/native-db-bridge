package policy

import "strings"

// redisReadCommands is the whitelist of safe read-only Redis commands.
var redisReadCommands = map[string]bool{
	"GET":      true,
	"MGET":     true,
	"TTL":      true,
	"PTTL":     true,
	"EXISTS":   true,
	"TYPE":     true,
	"STRLEN":   true,
	"HGET":     true,
	"HMGET":    true,
	"HGETALL":  true,
	"HLEN":     true,
	"HEXISTS":  true,
	"HKEYS":    true,
	"HVALS":    true,
	"LRANGE":   true,
	"LLEN":     true,
	"SCARD":    true,
	"SMEMBERS": true,
	"SISMEMBER": true,
	"ZRANGE":   true,
	"ZREVRANGE": true,
	"ZRANK":    true,
	"ZREVRANK": true,
	"ZSCORE":   true,
	"ZCARD":    true,
	"SCAN":     true,
	"HSCAN":    true,
	"SSCAN":    true,
	"ZSCAN":    true,
}

// redisWriteCommands is the set of known write Redis commands.
var redisWriteCommands = map[string]bool{
	"SET":      true,
	"DEL":      true,
	"EXPIRE":   true,
	"PEXPIRE":  true,
	"PERSIST":  true,
	"HSET":     true,
	"HDEL":     true,
	"LPUSH":    true,
	"RPUSH":    true,
	"LPOP":     true,
	"RPOP":     true,
	"SADD":     true,
	"SREM":     true,
	"ZADD":     true,
	"ZREM":     true,
	"INCR":     true,
	"DECR":     true,
	"FLUSHDB":  true,
	"FLUSHALL": true,
}

// redisAlwaysRejectedCommands is the set of Redis commands that must always
// be rejected regardless of read/write classification.
var redisAlwaysRejectedCommands = map[string]bool{
	"SELECT":    true,
	"EVAL":      true,
	"EVALSHA":   true,
	"SCRIPT":    true,
	"DEBUG":     true,
	"MONITOR":   true,
	"SUBSCRIBE": true,
	"PSUBSCRIBE": true,
	"CLIENT":    true,
	"CONFIG":    true,
	"SHUTDOWN":  true,
}

// IsRedisReadAllowed reports whether the Redis command is a safe read-only
// operation. The command is compared case-insensitively.
func IsRedisReadAllowed(command string) bool {
	return redisReadCommands[strings.ToUpper(strings.TrimSpace(command))]
}

// IsRedisWriteCommand reports whether the Redis command is a known write
// operation. The command is compared case-insensitively.
func IsRedisWriteCommand(command string) bool {
	return redisWriteCommands[strings.ToUpper(strings.TrimSpace(command))]
}

// IsRedisAlwaysRejected reports whether the Redis command must always be
// rejected for security reasons. The command is compared case-insensitively.
func IsRedisAlwaysRejected(command string) bool {
	return redisAlwaysRejectedCommands[strings.ToUpper(strings.TrimSpace(command))]
}
