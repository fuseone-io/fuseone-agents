package tools

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const CodeMCPServerRateLimited = "mcp_server_rate_limited"

var ErrServerRateLimited = errors.New("tools: MCP server rate limited")

// ServerRateLimitError is the stable, retryable shape of a local operational
// refusal. The call has not left the worker, so the runner must not record a
// ToolCalled step for it.
type ServerRateLimitError struct {
	Server     string
	RetryAfter time.Duration
}

func (e *ServerRateLimitError) Error() string {
	if e.RetryAfter <= 0 {
		return fmt.Sprintf("%s: %s", ErrServerRateLimited, e.Server)
	}
	return fmt.Sprintf("%s: %s, retry after %s", ErrServerRateLimited, e.Server, e.RetryAfter.Round(time.Millisecond))
}

func (e *ServerRateLimitError) Unwrap() error { return ErrServerRateLimited }

func (e *ServerRateLimitError) Summary() domain.FailureSummary {
	return domain.FailureSummary{
		Code:      CodeMCPServerRateLimited,
		Provider:  e.Server,
		Retryable: true,
	}
}

type serverOptions struct {
	rateLimit *domain.MCPRateLimit
	cache     *domain.MCPResultCache
}

type ServerOption func(*serverOptions)

func WithRateLimit(limit *domain.MCPRateLimit) ServerOption {
	return func(options *serverOptions) {
		if limit == nil || limit.RatePerSecond <= 0 || limit.Burst <= 0 {
			options.rateLimit = nil
			return
		}
		options.rateLimit = &domain.MCPRateLimit{
			RatePerSecond: limit.RatePerSecond,
			Burst:         limit.Burst,
		}
	}
}

func WithResultCache(cache *domain.MCPResultCache) ServerOption {
	return func(options *serverOptions) {
		if cache == nil || cache.TTLSeconds <= 0 {
			options.cache = nil
			return
		}
		maxEntries := cache.MaxEntries
		if maxEntries <= 0 {
			maxEntries = defaultResultCacheEntries
		}
		options.cache = &domain.MCPResultCache{
			TTLSeconds: cache.TTLSeconds,
			MaxEntries: maxEntries,
		}
	}
}

type serverLimiter struct {
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newServerLimiter(limit *domain.MCPRateLimit, now time.Time) *serverLimiter {
	if limit == nil || limit.RatePerSecond <= 0 || limit.Burst <= 0 {
		return nil
	}
	burst := float64(limit.Burst)
	return &serverLimiter{
		rate:   limit.RatePerSecond,
		burst:  burst,
		tokens: burst,
		last:   now,
	}
}

func (l *serverLimiter) allow(now time.Time) (time.Duration, bool) {
	if l == nil {
		return 0, true
	}
	if now.Before(l.last) {
		l.last = now
	}
	elapsed := now.Sub(l.last).Seconds()
	l.last = now
	l.tokens = math.Min(l.burst, l.tokens+elapsed*l.rate)
	if l.tokens >= 1 {
		l.tokens--
		return 0, true
	}
	seconds := (1 - l.tokens) / l.rate
	wait := time.Duration(math.Ceil(seconds*1000)) * time.Millisecond
	if wait <= 0 {
		wait = time.Millisecond
	}
	return wait, false
}

// Reserve consumes one token for a tool call before the ledger records the
// effect. A rate-limited call did not reach the server, so it is a retryable
// worker failure rather than a ToolReturned failure.
func (c *Catalog) Reserve(ctx context.Context, call engine.Call) error {
	decision := c.reserve(call)
	if decision.healthCode != "" {
		recordMCPToolHealth(
			ctx,
			decision.health,
			decision.healthBy,
			decision.server,
			false,
			decision.healthCode,
			decision.observedAt,
		)
	}
	return decision.err
}

type reserveDecision struct {
	err        error
	health     ToolCallHealth
	healthBy   string
	server     string
	healthCode string
	observedAt time.Time
}

func (c *Catalog) reserve(call engine.Call) reserveDecision {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, known := c.entries[call.Tool]
	health := c.health
	healthBy := c.healthBy
	observedAt := clockNow(c.clock)
	if !known {
		c.recordMCPReservationRefused(CodeMCPUnknownTool)
		return reserveDecision{err: fmt.Errorf("%w: %s", ErrUnknownTool, call.Tool)}
	}
	if !entry.OnSurface {
		c.recordMCPReservationRefused(CodeMCPUnknownTool)
		return reserveFailure(
			fmt.Errorf("%w: %s", ErrUnknownTool, call.Tool),
			health, healthBy, entry.Server, CodeMCPUnknownTool, observedAt)
	}
	if _, connected := c.sessions[entry.Server]; !connected {
		c.recordMCPReservationRefused(CodeMCPUnknownServer)
		return reserveFailure(
			fmt.Errorf("%w: %s", ErrUnknownServer, entry.Server),
			health, healthBy, entry.Server, CodeMCPUnknownServer, observedAt)
	}
	if cache := c.caches[entry.Server]; resultCacheable(entry, call, c.content, cache) {
		if cache.has(resultCacheKeyOf(entry, call), observedAt) {
			return reserveDecision{}
		}
	}
	limiter := c.limiters[entry.Server]
	if wait, ok := limiter.allow(observedAt); !ok {
		c.recordMCPReservationRefused(CodeMCPServerRateLimited)
		return reserveFailure(
			&ServerRateLimitError{Server: entry.Server, RetryAfter: wait},
			health, healthBy, entry.Server, CodeMCPServerRateLimited, observedAt)
	}
	return reserveDecision{}
}

func reserveFailure(
	err error,
	health ToolCallHealth,
	healthBy, server, code string,
	observedAt time.Time,
) reserveDecision {
	return reserveDecision{
		err:        err,
		health:     health,
		healthBy:   healthBy,
		server:     server,
		healthCode: code,
		observedAt: observedAt,
	}
}

func (c *Catalog) recordMCPReservationRefused(code string) {
	if c.metrics != nil {
		c.metrics.MCPReservationRefused(code)
	}
}
