package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// The queue of proposals, durably.
//
// Apart from the assertions the store serves because a proposal is not a
// memory: it is something an agent noticed and nobody has agreed to yet, and
// every path out of here that does agree goes through the merge rather than
// writing a row of its own.

func (p *Postgres) Suggest(
	ctx context.Context, s domain.MemorySuggestion, policy domain.MemoryLearningPolicy,
	by domain.UserID, now time.Time,
) (domain.MemorySuggestionOutcome, error) {
	policy = policy.Normalize()
	if !policy.Enabled() {
		return domain.MemorySuggestionOutcome{Result: domain.MemorySuggestIgnored}, nil
	}
	prepared, err := prepareSuggestion(s, by, now)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: begin suggest: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	flow := suggestionTx{tx: tx, by: by, now: now}

	// Suggestion first, then identity, and the duplicate check after both. The
	// order is the one every other path takes — auto-confirm reaches the
	// identity from inside this transaction — and asking whether memory already
	// covers this before holding the identity would be asking about rows another
	// writer is in the middle of changing.
	if err := lockSuggestion(ctx, tx, prepared.ID); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	identity := identitiesForSuggestion(prepared)[0]
	if err := lockIdentity(ctx, tx, identity); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if err := keyLegacyIdentities(ctx, tx, identity); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if out, done, err := flow.alreadyActive(ctx, prepared); done || err != nil {
		return out, err
	}
	stored, err := upsertSuggestion(ctx, tx, prepared)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if stored.Status != domain.MemorySuggestionPending {
		return flow.ignored(ctx, stored)
	}
	return flow.finish(ctx, stored, policy)
}

type suggestionTx struct {
	tx  pgx.Tx
	by  domain.UserID
	now time.Time
}

func (s suggestionTx) alreadyActive(
	ctx context.Context,
	prepared domain.MemorySuggestion,
) (domain.MemorySuggestionOutcome, bool, error) {
	var active *domain.MemoryAssertion
	for _, identity := range identitiesForSuggestion(prepared) {
		found, err := byIdentityTx(ctx, s.tx, identity)
		if err != nil {
			return domain.MemorySuggestionOutcome{}, false, err
		}
		if found != nil && found.Status == domain.MemoryActive && !expired(*found, s.now) {
			active = found
			break
		}
	}
	if active == nil {
		return domain.MemorySuggestionOutcome{}, false, nil
	}
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, true,
			fmt.Errorf("memory: commit already active suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{
		Suggestion: prepared, Assertion: active, Result: domain.MemorySuggestAlreadyActive,
	}, true, nil
}

func (s suggestionTx) ignored(
	ctx context.Context, stored domain.MemorySuggestion,
) (domain.MemorySuggestionOutcome, error) {
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit ignored suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{Suggestion: stored, Result: domain.MemorySuggestIgnored}, nil
}

func (s suggestionTx) recordSuggested(ctx context.Context, stored domain.MemorySuggestion) error {
	return recordEvent(ctx, s.tx, assertionFromSuggestion(stored, 0, s.by, s.now),
		s.by, "suggested by agent", "suggested")
}

func (s suggestionTx) finish(
	ctx context.Context,
	stored domain.MemorySuggestion,
	policy domain.MemoryLearningPolicy,
) (domain.MemorySuggestionOutcome, error) {
	if err := s.recordSuggested(ctx, stored); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if policy.AutoConfirms(stored.Labels) && stored.Observations >= policy.MinObservations {
		return s.autoConfirm(ctx, stored)
	}
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{Suggestion: stored, Result: domain.MemorySuggestPending}, nil
}

func (s suggestionTx) autoConfirm(
	ctx context.Context, stored domain.MemorySuggestion,
) (domain.MemorySuggestionOutcome, error) {
	assertion := assertionFromSuggestion(stored, stored.Observations, systemMemory, s.now)
	merged, outcome, err := mergeInto(ctx, s.tx, assertion, OriginAutoConfirm,
		assertion.UpdatedBy, "auto-confirmed repeated suggestions", "auto_confirmed")
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if outcome == Covered {
		if err := closeAsCovered(ctx, s.tx, stored, merged, assertion.UpdatedBy, s.now); err != nil {
			return domain.MemorySuggestionOutcome{}, err
		}
		if err := s.tx.Commit(ctx); err != nil {
			return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit suggestion: %w", err)
		}
		covered := merged
		return domain.MemorySuggestionOutcome{
			Suggestion: stored, Assertion: &covered, Result: domain.MemorySuggestAlreadyActive,
		}, nil
	}
	assertion = merged
	stored.Status = domain.MemorySuggestionAutoConfirmed
	stored.UpdatedBy, stored.UpdatedAt = assertion.UpdatedBy, assertion.UpdatedAt
	if err := updateSuggestion(ctx, s.tx, stored); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit auto-confirm suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{
		Suggestion: stored, Assertion: &assertion, Result: domain.MemorySuggestAutoConfirmed,
	}, nil
}

func (p *Postgres) ListSuggestions(ctx context.Context, f SuggestionFilter) ([]domain.MemorySuggestion, error) {
	where, args := suggestionWhere(f)
	args = append(args, domain.MemorySuggestLimit(f.Limit))
	rows, err := p.pool.Query(ctx, `
		select `+suggestionColumns+`
		from memory_suggestions `+where+`
		order by updated_at desc, suggestion_id
		limit $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("memory: list suggestions: %w", err)
	}
	return scanSuggestions(rows)
}

func (p *Postgres) AcceptSuggestion(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) (domain.MemoryAssertion, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: begin accept suggestion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	s, err := readSuggestionTx(ctx, tx, id, scope, true)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if s.Status != domain.MemorySuggestionPending {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	assertion := assertionFromSuggestion(s, s.Observations, by, now)
	stored, outcome, err := mergeInto(ctx, tx, assertion, OriginAccept, by, reason, "accepted")
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if outcome == Covered {
		// Shared memory already answers this and was not modified. The proposal
		// is finished all the same: leaving it pending would be a queue item
		// with no honest exit, and dismissing it would record a refusal nobody
		// made about a fact the platform already holds.
		if err := closeAsCovered(ctx, tx, s, stored, by, now); err != nil {
			return domain.MemoryAssertion{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.MemoryAssertion{}, fmt.Errorf("memory: commit accept suggestion: %w", err)
		}
		return stored, nil
	}
	// After the merge, in the same transaction: a suggestion marked accepted
	// beside an assertion that was never written is a queue that empties while
	// nothing is learned.
	s.Status, s.UpdatedBy, s.UpdatedAt = domain.MemorySuggestionAccepted, by, now.UTC()
	if err := updateSuggestion(ctx, tx, s); err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: commit accept suggestion: %w", err)
	}
	return stored, nil
}

func (p *Postgres) DismissSuggestion(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin dismiss suggestion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	s, err := readSuggestionTx(ctx, tx, id, scope, true)
	if err != nil {
		return err
	}
	if s.Status != domain.MemorySuggestionPending {
		return ErrNotFound
	}
	s.Status, s.UpdatedBy, s.UpdatedAt = domain.MemorySuggestionDismissed, by, now.UTC()
	if err := recordEvent(ctx, tx, assertionFromSuggestion(s, 0, by, now), by, reason, "dismissed"); err != nil {
		return err
	}
	if err := updateSuggestion(ctx, tx, s); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("memory: commit dismiss suggestion: %w", err)
	}
	return nil
}
