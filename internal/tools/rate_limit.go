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
	_ = ctx
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, known := c.entries[call.Tool]
	if !known || !entry.OnSurface {
		return fmt.Errorf("%w: %s", ErrUnknownTool, call.Tool)
	}
	if _, connected := c.sessions[entry.Server]; !connected {
		return fmt.Errorf("%w: %s", ErrUnknownServer, entry.Server)
	}
	limiter := c.limiters[entry.Server]
	if wait, ok := limiter.allow(time.Now()); !ok {
		return &ServerRateLimitError{Server: entry.Server, RetryAfter: wait}
	}
	return nil
}
