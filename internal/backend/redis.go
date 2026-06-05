package backend

import "time"

// RedisResult holds the outcome of a single Redis command.
type RedisResult struct {
	Result  interface{}   `json:"result"`
	Elapsed time.Duration `json:"elapsed"`
}

// RedisScanResult holds one page of a SCAN iteration.
type RedisScanResult struct {
	Keys        []string `json:"keys"`
	NextCursor  string   `json:"next_cursor"`
	Truncated   bool     `json:"truncated"`
}

// RedisKeyDescription holds metadata about a single Redis key.
type RedisKeyDescription struct {
	Key    string `json:"key"`
	Type   string `json:"type"`
	TTL    int64  `json:"ttl"`    // -1 = no expiry, -2 = key does not exist
	Length int64  `json:"length"`
	Exists bool   `json:"exists"`
}
