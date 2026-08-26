// Package memory stores governed assertions that agents may recall between runs.
package memory

import (
	"context"
	"errors"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var ErrNotFound = errors.New("memory: assertion not found")

type Store interface {
	Find(ctx context.Context, q domain.MemoryQuery) ([]domain.MemoryAssertion, error)
	List(ctx context.Context, f Filter) ([]domain.MemoryAssertion, error)
	Assert(ctx context.Context, a domain.MemoryAssertion, by domain.UserID, reason string, now time.Time) (domain.MemoryAssertion, error)
	Disable(ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time) error
}

type Filter struct {
	Scopes  []domain.Scope
	AgentID domain.AgentID
	Status  domain.MemoryStatus
	Search  string
	Limit   int
	Now     time.Time
}
