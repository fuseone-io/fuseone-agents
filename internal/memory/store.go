// Package memory stores governed assertions that agents may recall between runs.
package memory

import (
	"context"
	"errors"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var (
	ErrNotFound = errors.New("memory: assertion not found")
	/*
		ErrInvalid means the caller's own input is wrong, as opposed to the state
		it arrived at or the database being unreachable.

		Three kinds of no came out of this package as the same untyped error, and
		the edge answered all of them with 400: a missing subject, a memory
		somebody had disabled, and Postgres not answering. A person correcting a
		fact was told their input was invalid while the truth was that two rows
		claim that identity — and the console could show nothing better, because
		the difference was in prose it was not allowed to parse.
	*/
	ErrInvalid = errors.New("memory: the input is not valid")
)

type Store interface {
	Find(ctx context.Context, q domain.MemoryQuery) ([]domain.MemoryAssertion, error)
	List(ctx context.Context, f Filter) ([]domain.MemoryAssertion, error)
	Assert(ctx context.Context, a domain.MemoryAssertion, by domain.UserID, reason string, now time.Time) (domain.MemoryAssertion, error)
	Disable(ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time) error
	Suggest(ctx context.Context, s domain.MemorySuggestion, policy domain.MemoryLearningPolicy,
		by domain.UserID, now time.Time) (domain.MemorySuggestionOutcome, error)
	ListSuggestions(ctx context.Context, f SuggestionFilter) ([]domain.MemorySuggestion, error)
	AcceptSuggestion(ctx context.Context, in AcceptInput) (domain.MemoryAssertion, error)
	DismissSuggestion(ctx context.Context, id string, scope domain.Scope, by domain.UserID,
		reason string, now time.Time) error
}

type Filter struct {
	Scopes  []domain.Scope
	AgentID domain.AgentID
	Status  domain.MemoryStatus
	Search  string
	Limit   int
	Now     time.Time
}

type SuggestionFilter struct {
	Scopes  []domain.Scope
	AgentID domain.AgentID
	Status  domain.MemorySuggestionStatus
	Search  string
	Limit   int
	Now     time.Time
}
