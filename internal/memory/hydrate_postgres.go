package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Hydrate completes what the platform can now derive, one page at a time.
//
// The ledger is read outside the lock and the row is re-read inside it: a
// citation that changed between the two belongs to a writer whose version is
// newer than this repair's snapshot, and the next pass will look again.
func (p *Postgres) Hydrate(
	ctx context.Context, r *Resolver, page HydratePage,
) (HydrateResult, error) {
	candidates, err := p.hydrationPage(ctx, page)
	if err != nil {
		return HydrateResult{}, err
	}

	var out HydrateResult
	for _, stored := range candidates {
		out.Scanned++
		out.Cursor = stored.ID

		resolved, ok, err := resolveFor(ctx, r, stored)
		if err != nil {
			return out, err
		}
		if !ok {
			out.Skipped++
			continue
		}
		repaired, err := p.hydrateOne(ctx, stored, resolved, nowOr(page.Now))
		if err != nil {
			return out, err
		}
		if repaired {
			out.Repaired++
		}
	}
	if out.Scanned < page.limit() {
		out.Cursor = ""
	}
	return out, nil
}

func (p *Postgres) hydrateOne(
	ctx context.Context, snapshot domain.MemoryAssertion,
	resolved []domain.MemoryEvidence, now time.Time,
) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("memory: begin hydrate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockIdentity(ctx, tx, snapshot); err != nil {
		return false, err
	}
	current, err := byIdentityTx(ctx, tx, snapshot)
	if err != nil {
		return false, err
	}
	// Gone, or written by somebody else since the snapshot. Either way this
	// repair is describing a row that no longer exists as it was.
	if current == nil || !sameEvidence(current.Evidence, snapshot.Evidence) {
		return false, nil
	}

	next, changed := hydrated(*current, resolved, true)
	if !changed {
		return false, nil
	}
	if err := writeDerived(ctx, tx, next); err != nil {
		return false, err
	}
	// In the same transaction, or the log stops being able to reconstruct the
	// evidence the projection now shows.
	if err := recordEvent(ctx, tx, next, "system:memory", "hydrated from the ledger", "hydrated"); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("memory: commit hydrate: %w", err)
	}
	return true, nil
}

// writeDerived touches only what the platform derives. updated_at is not in the
// list: a repair is not somebody changing the memory, and moving the timestamp
// would make every screen that sorts by it report a change nobody made.
func writeDerived(ctx context.Context, tx db, a domain.MemoryAssertion) error {
	evidence, err := json.Marshal(a.Evidence)
	if err != nil {
		return fmt.Errorf("memory: encode hydrated evidence: %w", err)
	}
	_, err = tx.Exec(ctx, `
		update memory_assertions
		set evidence = $2, labels = $3, canonical_identity_key = $4
		where assertion_id = $1`,
		a.ID, evidence, []string(a.Labels), domain.CanonicalIdentityKey(a))
	if err != nil {
		return fmt.Errorf("memory: hydrate assertion %s: %w", a.ID, err)
	}
	return nil
}

// hydrationPage is the rows that may still be missing something derivable: no
// canonical identity, or a citation that does not say which step it names.
func (p *Postgres) hydrationPage(
	ctx context.Context, page HydratePage,
) ([]domain.MemoryAssertion, error) {
	rows, err := p.pool.Query(ctx, `
		select `+columns+` from memory_assertions
		where assertion_id > $1
		  and (canonical_identity_key is null
		       or exists (select 1 from jsonb_array_elements(evidence) ev
		                  where coalesce((ev->>'seq')::bigint, 0) = 0))
		order by assertion_id
		limit $2`, page.After, page.limit())
	if err != nil {
		return nil, fmt.Errorf("memory: read hydration page: %w", err)
	}
	return scan(rows, nowOr(page.Now))
}
