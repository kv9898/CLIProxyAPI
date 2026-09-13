// Package clientkeys tracks ephemeral token totals, never request bodies or raw secrets.
package clientkeys

import (
	"crypto/sha256"
	"encoding/hex"
	coreusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"sync"
	"time"
)

func ID(key string) string { sum := sha256.Sum256([]byte(key)); return hex.EncodeToString(sum[:]) }

type Snapshot struct {
	Since time.Time        `json:"since"`
	Total int64            `json:"total_tokens"`
	Keys  map[string]int64 `json:"keys"`
}

type Counter struct {
	mu    sync.Mutex
	since time.Time
	total int64
	keys  map[string]int64
}

func NewCounter() *Counter { return &Counter{since: time.Now().UTC(), keys: make(map[string]int64)} }

var Default = NewCounter()

func (c *Counter) Record(record coreusage.Record) {
	if record.APIKey == "" {
		return
	}
	detail := coreusage.EnsureTokenBreakdownForProvider(record.Detail, record.Provider, record.ExecutorType)
	tokens := detail.TotalTokens
	if tokens <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += tokens
	c.keys[ID(record.APIKey)] += tokens
}

func (c *Counter) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := Snapshot{Since: c.since, Total: c.total, Keys: make(map[string]int64, len(c.keys))}
	for id, tokens := range c.keys {
		out.Keys[id] = tokens
	}
	return out
}
