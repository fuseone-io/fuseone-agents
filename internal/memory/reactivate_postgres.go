package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// Reactivate makes a disabled memory readable again, having proved it once
// more. See the in-memory implementation for why the ledger is read before the
// identity is held, and why the snapshot is compared rather than trusted.
func (p *Postgres) Reactivate(
	ctx context.Context, r *Resolver, in ReactivateInput,
) (domain.MemoryAssertion, error) {
	if err := in.validate(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	snapshot, err := readAssertionTx(ctx, p.pool, in.ID, in.Scope)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if snapshot.Status != domain.MemoryDisabled {
		return domain.MemoryAssertion{}, fmt.Errorf(
			"%w: it is %s", ErrMemoryTerminal, snapshot.Status)
	}
	_, got, err := resolveEvidence(ctx, r, snapshot.Scope, snapshot.Evidence)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: begin reactivate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockIdentity(ctx, tx, snapshot); err != nil {
		return domain.MemoryAssertion{}, err
	}
	current, err := readAssertionTx(ctx, tx, in.ID, in.Scope)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if got == proofSourceAbsent {
		return domain.MemoryAssertion{}, p.recordSourceGone(ctx, tx, in, snapshot, current)
	}

	covering, err := coveringActiveTx(ctx, tx, current, in.Now)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := reactivable(snapshot, current, covering, got); err != nil {
		return domain.MemoryAssertion{}, err
	}

	next := reactivated(current, in)
	if err := writeReactivated(ctx, tx, next); err != nil {
		return domain.MemoryAssertion{}, err
	}
	// In the same transaction as the projection. A memory that came back with
	// no event beside it is a row whose current state the trail cannot explain,
	// and the trail is the whole reason this is an act rather than an update.
	if err := recordEvent(ctx, tx, next, in.By, in.Reason, "reactivated"); err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: commit reactivate: %w", err)
	}
	return next, nil
}

// recordSourceGone commits the state the row was already in and refuses. The
// platform lost the proof when the run was erased; this only noticed, and
// saying so once means the next attempt reads the reason off the memory.
func (p *Postgres) recordSourceGone(
	ctx context.Context, tx pgx.Tx, in ReactivateInput,
	snapshot, current domain.MemoryAssertion,
) error {
	if current.Status != domain.MemoryDisabled ||
		!sameEvidence(current.Evidence, snapshot.Evidence) {
		return sourceGone()
	}
	if err := markSourceErased(ctx, tx, current, in.Now); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("memory: commit source erased: %w", err)
	}
	return sourceGone()
}

// writeReactivated touches the three columns this decision is about and no
// others. An upsert here would rewrite the claim, the counters and the
// citations from a value read before the lock — correct today only because
// nothing else may hold the identity, and wrong the first time that changes.
func writeReactivated(ctx context.Context, tx db, a domain.MemoryAssertion) error {
	_, err := tx.Exec(ctx, `
		update memory_assertions
		set status = $2, expires_at = $3, updated_by = $4, updated_at = $5
		where assertion_id = $1`,
		a.ID, string(a.Status), a.ExpiresAt, string(a.UpdatedBy), a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("memory: reactivate %s: %w", a.ID, err)
	}
	return nil
}

// coveringActiveTx is the shared memory that already answers this identity, if
// there is one. Read under the lock that holds both keys, which lockIdentity
// took for an agent-scoped row precisely so this question has a stable answer.
func coveringActiveTx(
	ctx context.Context, tx db, a domain.MemoryAssertion, now time.Time,
) (*domain.MemoryAssertion, error) {
	if a.AgentID == "" {
		return nil, nil
	}
	shared := a
	shared.AgentID = ""
	shared.ID = domain.MemoryAssertionID(shared)

	found, err := byIdentityTx(ctx, tx, shared)
	if err != nil || found == nil {
		return nil, err
	}
	if found.Status != domain.MemoryActive || expired(*found, nowOrWall(now)) {
		return nil, nil
	}
	return found, nil
}
