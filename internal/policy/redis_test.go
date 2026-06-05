package policy

import "testing"

func TestRedisCommandPolicy(t *testing.T) {
	if !IsRedisReadAllowed("GET") {
		t.Fatalf("GET should be read")
	}
	if IsRedisReadAllowed("SET") {
		t.Fatalf("SET must not be read")
	}
	if !IsRedisAlwaysRejected("SELECT") {
		t.Fatalf("SELECT must be always rejected")
	}
	if !IsRedisAlwaysRejected("EVAL") {
		t.Fatalf("EVAL must be always rejected")
	}
}

func TestRedisReadAllowed(t *testing.T) {
	readCommands := []string{
		"GET", "MGET", "TTL", "PTTL", "EXISTS", "TYPE", "STRLEN",
		"HGET", "HMGET", "HGETALL", "HLEN", "HEXISTS", "HKEYS", "HVALS",
		"LRANGE", "LLEN", "SCARD", "SMEMBERS", "SISMEMBER",
		"ZRANGE", "ZREVRANGE", "ZRANK", "ZREVRANK", "ZSCORE", "ZCARD",
		"SCAN", "HSCAN", "SSCAN", "ZSCAN",
	}
	for _, cmd := range readCommands {
		if !IsRedisReadAllowed(cmd) {
			t.Errorf("IsRedisReadAllowed(%q) = false, want true", cmd)
		}
	}
}

func TestRedisWriteCommand(t *testing.T) {
	writeCommands := []string{
		"SET", "DEL", "EXPIRE", "PEXPIRE", "PERSIST",
		"HSET", "HDEL", "LPUSH", "RPUSH", "LPOP", "RPOP",
		"SADD", "SREM", "ZADD", "ZREM", "INCR", "DECR",
		"FLUSHDB", "FLUSHALL",
	}
	for _, cmd := range writeCommands {
		if !IsRedisWriteCommand(cmd) {
			t.Errorf("IsRedisWriteCommand(%q) = false, want true", cmd)
		}
	}
}

func TestRedisAlwaysRejected(t *testing.T) {
	rejected := []string{
		"SELECT", "EVAL", "EVALSHA", "SCRIPT", "DEBUG", "MONITOR",
		"SUBSCRIBE", "PSUBSCRIBE", "CLIENT", "CONFIG", "SHUTDOWN",
	}
	for _, cmd := range rejected {
		if !IsRedisAlwaysRejected(cmd) {
			t.Errorf("IsRedisAlwaysRejected(%q) = false, want true", cmd)
		}
	}
}

func TestRedisCaseInsensitive(t *testing.T) {
	if !IsRedisReadAllowed("get") {
		t.Fatalf("IsRedisReadAllowed('get') should be true")
	}
	if !IsRedisReadAllowed("Get") {
		t.Fatalf("IsRedisReadAllowed('Get') should be true")
	}
}
