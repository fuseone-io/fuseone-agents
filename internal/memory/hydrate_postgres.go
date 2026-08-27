package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

		resolved, proved, err := resolveFor(ctx, r, stored)
		if err != nil {
			return out, err
		}
		if !proved {
			out.Unproved++
		}
		repaired, err := p.hydrateOne(ctx, stored, resolved, proved)
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
	resolved []domain.MemoryEvidence, proved bool,
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
	if current == nil {
		return false, nil
	}
	// Written by somebody else since the snapshot: their version is newer than
	// this repair's, so the citations are left alone. The canonical identity
	// still derives from the row's own fields and is written anyway.
	if !sameEvidence(current.Evidence, snapshot.Evidence) {
		proved = false
	}

	h := hydrated(*current, resolved, proved, true)
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
		if err := recordEvent(ctx, tx, h.next, "system:memory",
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

/*
HydrateSuggestions completes pending proposals, on a cursor of their own.

The suggestion lock is taken first and nothing else is, keeping the order the
write paths already use: auto-confirm goes suggestion then identity, so a repair
that went identity then suggestion would be the pair of orders a deadlock needs.

No event. See the note on the in-memory implementation for why hydrated in
memory_assertion_events would be ambiguous rather than merely redundant.
*/
func (p *Postgres) HydrateSuggestions(
	ctx context.Context, r *Resolver, page HydratePage,
) (HydrateResult, error) {
	candidates, err := p.suggestionHydrationPage(ctx, page)
	if err != nil {
		return HydrateResult{}, err
	}

	var out HydrateResult
	for _, stored := range candidates {
		out.Scanned++
		out.Cursor = stored.ID

		resolved, proved, err := resolveEvidence(ctx, r, stored.Scope, stored.Evidence)
		if err != nil {
			return out, err
		}
		if !proved {
			out.Unproved++
		}
		repaired, err := p.hydrateSuggestion(ctx, stored, resolved, proved)
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

func (p *Postgres) hydrateSuggestion(
	ctx context.Context, snapshot domain.MemorySuggestion,
	resolved []domain.MemoryEvidence, proved bool,
) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("memory: begin hydrate suggestion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockSuggestion(ctx, tx, snapshot.ID); err != nil {
		return false, err
	}
	current, err := readSuggestionTx(ctx, tx, snapshot.ID, snapshot.Scope, false)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Overtaken: another observation merged into this proposal while the ledger
	// was being read, and its citations are newer than this snapshot's.
	if !sameEvidence(current.Evidence, snapshot.Evidence) {
		proved = false
	}

	evidence, labels := current.Evidence, current.Labels
	changed := false
	if proved {
		evidence, labels, changed = hydratedProvenance(current.Evidence, current.Labels, resolved)
	}
	// Every row this sweep sees is one the page found missing something — a
	// canonical key, or a citation that does not say which step it names — so
	// there is always something to write. The second pass will not see it.
	_ = changed
	key := domain.CanonicalIdentityKey(assertionOf(current))

	// Narrow on purpose: status, covered_by, claim, observations, expiry,
	// authorship and updated_at are what somebody decided or when they decided
	// it, and a repair is neither.
	body, err := json.Marshal(evidence)
	if err != nil {
		return false, fmt.Errorf("memory: encode hydrated suggestion evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update memory_suggestions
		set evidence = $2, labels = $3, canonical_identity_key = $4
		where suggestion_id = $1`,
		current.ID, body, []string(labels), key); err != nil {
		return false, fmt.Errorf("memory: hydrate suggestion %s: %w", current.ID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("memory: commit hydrate suggestion: %w", err)
	}
	return true, nil
}

func (p *Postgres) suggestionHydrationPage(
	ctx context.Context, page HydratePage,
) ([]domain.MemorySuggestion, error) {
	rows, err := p.pool.Query(ctx, `
		select `+suggestionColumns+` from memory_suggestions
		where suggestion_id > $1
		  and (canonical_identity_key is null
		       or exists (select 1 from jsonb_array_elements(evidence) ev
		                  where coalesce((ev->>'seq')::bigint, 0) = 0))
		order by suggestion_id
		limit $2`, page.After, page.limit())
	if err != nil {
		return nil, fmt.Errorf("memory: read suggestion hydration page: %w", err)
	}
	return scanSuggestions(rows)
}

// assertionOf is the identity a suggestion would become, which is what its
// canonical key is about.
func assertionOf(s domain.MemorySuggestion) domain.MemoryAssertion {
	return domain.MemoryAssertion{
		Scope: s.Scope, AgentID: s.AgentID,
		Kind: s.Kind, Subject: s.Subject, Signature: s.Signature,
	}
}
