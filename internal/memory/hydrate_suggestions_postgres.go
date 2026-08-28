package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// The pending queue's repair, on a cursor of its own.
//
// Apart from the assertions' sweep because the two share nothing but the
// decision: different table, different lock, different ending. A single cursor
// could not represent progress through both, which is what made them two
// methods in the first place.

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

		resolved, got, err := resolveEvidence(ctx, r, stored.Scope, stored.Evidence)
		if err != nil {
			return out, err
		}
		switch got {
		case proofUnproved:
			out.Unproved++
		case proofSourceAbsent:
			out.SourceGone++
		}
		repaired, err := p.hydrateSuggestion(ctx, stored, proven{resolved, got, page.Now})
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
	ctx context.Context, snapshot domain.MemorySuggestion, said proven,
) (bool, error) {
	resolved, got := said.evidence, said.got
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
		got = proofUnproved
	}
	if got == proofSourceAbsent && current.Status == domain.MemorySuggestionPending {
		current.Status = domain.MemorySuggestionSourceErased
		current.UpdatedBy, current.UpdatedAt = systemMemory, nowOr(said.now)
		if err := updateSuggestion(ctx, tx, current); err != nil {
			return false, err
		}
		// Repairing a proposal writes no event; ending one does. Without it the
		// proposal simply stopped being in the queue — no refusal, no
		// acceptance, nothing saying where it went — while the administrative
		// erasure recorded exactly this transition for exactly these rows.
		if err := recordSuggestionEnded(ctx, tx, current); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("memory: commit suggestion source erased: %w", err)
		}
		return true, nil
	}

	evidence, labels := current.Evidence, current.Labels
	changed := false
	if got == proofProved {
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
