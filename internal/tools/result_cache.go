package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const (
	defaultResultCacheEntries = 256
	resultCacheValueLimit     = 64 * 1024
)

type resultCacheKey struct {
	tool       domain.ToolID
	digest     string
	effect     domain.Effect
	untrusted  bool
	argsDigest string
	scope      string
	principal  string
}

type resultCacheEntry struct {
	body      []byte
	labels    domain.Labels
	sourceRun domain.RunID
	sourceSeq int64
	expiresAt time.Time
}

type resultCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	entries    map[resultCacheKey]resultCacheEntry
	order      []resultCacheKey
}

func newResultCache(config *domain.MCPResultCache) *resultCache {
	if config == nil || config.TTLSeconds <= 0 {
		return nil
	}
	maxEntries := config.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultResultCacheEntries
	}
	return &resultCache{
		ttl:        time.Duration(config.TTLSeconds) * time.Second,
		maxEntries: maxEntries,
		entries:    make(map[resultCacheKey]resultCacheEntry),
	}
}

func resultCacheable(
	entry Entry, call engine.Call, content engine.ContentStore, cache *resultCache,
) bool {
	return content != nil && cache != nil && entry.Effect == domain.EffectRead
}

func resultCacheKeyOf(entry Entry, call engine.Call) resultCacheKey {
	argsDigest := sha256.Sum256(call.Args)
	return resultCacheKey{
		tool:       entry.ID,
		digest:     entry.Digest,
		effect:     entry.Effect,
		untrusted:  entry.Untrusted,
		argsDigest: hex.EncodeToString(argsDigest[:]),
		scope:      fmt.Sprintf("%s/%s", call.Scope.Company, call.Scope.Area),
		principal:  string(call.OnBehalfOf),
	}
}

func (c *resultCache) has(key resultCacheKey, now time.Time) bool {
	_, ok := c.get(key, now)
	return ok
}

func (c *resultCache) get(key resultCacheKey, now time.Time) (resultCacheEntry, bool) {
	if c == nil {
		return resultCacheEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return resultCacheEntry{}, false
	}
	if !now.Before(entry.expiresAt) {
		delete(c.entries, key)
		return resultCacheEntry{}, false
	}
	entry.body = append([]byte(nil), entry.body...)
	entry.labels = entry.labels.Clone()
	return entry, true
}

func (c *resultCache) put(
	key resultCacheKey, body []byte, labels domain.Labels, sourceRun domain.RunID, sourceSeq int64, now time.Time,
) {
	if c == nil || len(body) == 0 || len(body) > resultCacheValueLimit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}
	c.entries[key] = resultCacheEntry{
		body:      append([]byte(nil), body...),
		labels:    labels.Clone(),
		sourceRun: sourceRun,
		sourceSeq: sourceSeq,
		expiresAt: now.Add(c.ttl),
	}
	for len(c.entries) > c.maxEntries && len(c.order) > 0 {
		victim := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, victim)
	}
}
