package memory

import (
	"context"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// The repair, performed against the in-memory store.
//
// Both sweeps read the ledger outside the mutex and re-read the row under it,
// which is the same order the durable store keeps for the same reason: holding
// an identity across I/O would block every writer of the same fact, and a value
// read before the lock is a value somebody may have changed since.

/*
Hydrate completes what the platform can now derive, one page at a time.

Idempotent by construction: a row whose citations already carry their step,
their source and their labels, and whose canonical identity is stored, is
recognised as complete and not written. A repair that cannot be run twice cannot
be run on a schedule, and this one runs before and after a rollout.

Resumable by cursor rather than by offset, so a sweep interrupted halfway
carries on from where it stopped rather than from where the rows happen to be
now.
*/
func (m *Memory) Hydrate(
	ctx context.Context, r *Resolver, page HydratePage,
) (HydrateResult, error) {
	if err := ctx.Err(); err != nil {
		return HydrateResult{}, err
	}
	candidates := m.hydrationPage(page)

	var out HydrateResult
	for _, stored := range candidates {
		out.Scanned++
		out.Cursor = stored.ID

		// Outside the lock: reading the ledger is I/O, and holding an identity
		// across it would block every writer of the same fact.
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

		m.mu.Lock()
		current, held := m.values[stored.ID]
		// Somebody may have written between the read and here. Their version is
		// newer than this repair's snapshot, so it is left alone and the next
		// pass will look again.
		// The canonical key is a column the durable store keeps; this one
		// computes it when it looks, so it can never be the thing missing.
		if held && sameEvidence(current.Evidence, stored.Evidence) {
			if got == proofSourceAbsent && current.Status == domain.MemoryActive {
				current.Status = domain.MemorySourceErased
				current.UpdatedBy, current.UpdatedAt = systemMemory, nowOr(page.Now)
				m.values[stored.ID] = cloneAssertion(current)
				out.Repaired++
			} else if h := hydrated(current, resolved, got == proofProved, false); h.writes() {
				m.values[stored.ID] = cloneAssertion(h.next)
				out.Repaired++
			}
		}
		m.mu.Unlock()
	}
	if out.Scanned < page.limit() {
		out.Cursor = ""
	}
	return out, nil
}

func (m *Memory) hydrationPage(page HydratePage) []domain.MemoryAssertion {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []domain.MemoryAssertion
	for _, held := range m.values {
		if held.ID > page.After {
			out = append(out, cloneAssertion(held))
		}
	}
	slices.SortFunc(out, func(a, b domain.MemoryAssertion) int {
		return strings.Compare(a.ID, b.ID)
	})
	if len(out) > page.limit() {
		out = out[:page.limit()]
	}
	return out
}

/*
HydrateSuggestions completes pending proposals, on a cursor of their own.

Separate from the assertion sweep rather than sharing one, because a single
cursor cannot say how far two tables have got: a pass that finished assertions
and stopped halfway through suggestions has nothing to write down that would
resume both.

No event is recorded. hydrated in memory_assertion_events would be ambiguous —
the row carries no suggestion id, no status and no covered_by, so a reader could
not tell a repaired memory from a repaired proposal, and could not reconstruct
the suggestion projection from it either. The existing actions are acts:
suggested, accepted, dismissed, auto-confirmed. Completing the record of an act
is not another act, and when the proposal is accepted the merge writes the event
with the evidence already whole — which is the hole this closes.
*/
func (m *Memory) HydrateSuggestions(
	ctx context.Context, r *Resolver, page HydratePage,
) (HydrateResult, error) {
	if err := ctx.Err(); err != nil {
		return HydrateResult{}, err
	}
	var out HydrateResult
	for _, stored := range m.suggestionPage(page) {
		out.Scanned++
		out.Cursor = stored.ID

		resolved, got, err := resolveEvidence(ctx, r, stored.Scope, stored.Evidence)
		if err != nil {
			return out, err
		}
		if got == proofUnproved {
			out.Unproved++
			continue
		}

		m.mu.Lock()
		current, held := m.suggestions[stored.ID]
		if got == proofSourceAbsent {
			out.SourceGone++
			if held && current.Status == domain.MemorySuggestionPending {
				current.Status = domain.MemorySuggestionSourceErased
				current.UpdatedBy, current.UpdatedAt = systemMemory, nowOr(page.Now)
				m.suggestions[stored.ID] = cloneSuggestion(current)
				out.Repaired++
			}
		} else if held && sameEvidence(current.Evidence, stored.Evidence) {
			evidence, labels, changed := hydratedProvenance(
				current.Evidence, current.Labels, resolved)
			if changed {
				current.Evidence, current.Labels = evidence, labels
				m.suggestions[stored.ID] = cloneSuggestion(current)
				out.Repaired++
			}
		}
		m.mu.Unlock()
	}
	if out.Scanned < page.limit() {
		out.Cursor = ""
	}
	return out, nil
}

func (m *Memory) suggestionPage(page HydratePage) []domain.MemorySuggestion {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []domain.MemorySuggestion
	for _, held := range m.suggestions {
		if held.ID > page.After {
			out = append(out, cloneSuggestion(held))
		}
	}
	slices.SortFunc(out, func(a, b domain.MemorySuggestion) int {
		return strings.Compare(a.ID, b.ID)
	})
	if len(out) > page.limit() {
		out = out[:page.limit()]
	}
	return out
}

// sameEvidence compares citations field by field, because hydration changes the
// fields inside a record and the keys along with them.
