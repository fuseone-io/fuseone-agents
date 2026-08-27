package memory

import (
	"context"
	"encoding/json"
	"errors"
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

		resolved, got, err := resolveFor(ctx, r, stored)
		if err != nil {
			return out, err
		}
		switch got {
		case proofUnproved:
			out.Unproved++
		case proofSourceAbsent:
			out.SourceGone++
		}
		repaired, err := p.hydrateOne(ctx, stored, proven{resolved, got, page.Now})
		// The row has a twin, so there is nothing to derive for it — but that
		// is a fact about one identity, not about the sweep. Ending here would
		// stop the repair on the very rows it walks.
		if errors.Is(err, ErrCanonicalConflict) {
			out.Conflicted++
			continue
		}
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
	ctx context.Context, snapshot domain.MemoryAssertion, said proven,
) (bool, error) {
	resolved, got := said.evidence, said.got
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
	if current == nil {
		return false, nil
	}
	// Written by somebody else since the snapshot: their version is newer than
	// this repair's, so the citations are left alone. The canonical identity
	// still derives from the row's own fields and is written anyway.
	if !sameEvidence(current.Evidence, snapshot.Evidence) {
		got = proofUnproved
	}

	// The run is gone, so the memory stops being readable. The platform already
	// has this state and the transition is the one it always was; hydration is
	// only the thing that noticed.
	if got == proofSourceAbsent && current.Status == domain.MemoryActive {
		if err := markSourceErased(ctx, tx, *current, said.now); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("memory: commit source erased: %w", err)
		}
		return true, nil
	}

	h := hydrated(*current, resolved, got == proofProved, true)
	if !h.writes() {
		return false, nil
	}
	if err := writeDerived(ctx, tx, h.next); err != nil {
		return false, err
	}
	// Only when the provenance moved, and in the same transaction: otherwise
	// the log stops being able to reconstruct the evidence the projection now
	// shows. A key-only write needs none — it is recalculable and does not
	// appear in the event detail.
	if h.provenance {
		if err := recordEvent(ctx, tx, h.next, systemMemory,
			"hydrated from the ledger", "hydrated"); err != nil {
			return false, err
		}
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

func assertionOf(s domain.MemorySuggestion) domain.MemoryAssertion {
	return domain.MemoryAssertion{
		Scope: s.Scope, AgentID: s.AgentID,
		Kind: s.Kind, Subject: s.Subject, Signature: s.Signature,
	}
}

/*
markSourceErased is the transition retention already performs, reached from the
other direction: retention knows which runs it erased, and a sweep or a
reactivation discovers a source that is no longer there. The state and the event
are the same ones, because what happened is the same thing.

Dated when it was discovered, not when the memory was last decided about. The
row kept its own updated_at and recordEvent writes that value as the moment the
event happened, so a sweep finding today that a run was taken filed the event
under the last human decision — months back, out of order in the trail, and old
enough for the retention of events to delete it before anybody read it.

Authored by the platform for the same reason. Nobody decided this; something
noticed it.

Unlike the repair in writeDerived, which deliberately leaves updated_at alone:
filling a field the platform can now derive is not a change to the memory, and a
status becoming terminal is.
*/
func markSourceErased(ctx context.Context, tx db, a domain.MemoryAssertion, now time.Time) error {
	a.Status = domain.MemorySourceErased
	a.UpdatedBy, a.UpdatedAt = systemMemory, nowOr(now)
	if _, err := tx.Exec(ctx, `
		update memory_assertions
		set status = $2, canonical_identity_key = $3, updated_by = $4, updated_at = $5
		where assertion_id = $1`,
		a.ID, string(a.Status), domain.CanonicalIdentityKey(a),
		string(a.UpdatedBy), a.UpdatedAt); err != nil {
		return fmt.Errorf("memory: mark %s source erased: %w", a.ID, err)
	}
	return recordEvent(ctx, tx, a, a.UpdatedBy,
		"the source the evidence names is no longer there", "source_erased")
}
