package memory

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Hydration is the repair door, and it is deliberately narrow.

The write door is Merge: a person corrected this, a suggestion was accepted, a
policy confirmed observations. Those assert something. Hydration asserts
nothing — it completes the description of a citation that was already there,
with information that was always in the ledger and that the projection could not
carry when it was written.

So it may change exactly three things: the canonical identity, the derived
fields of a citation, and the labels derived from them. Claim, counts, status,
authorship, expiry and updated_at are what somebody decided, and a repair that
moved them would be reinterpreting the past rather than finishing writing it
down.

Routing this through Merge was the obvious idea and is wrong: a legacy citation
and its hydrated form have different keys — seq and digest both change — so the
fold would keep both and double every record it repaired.
*/
type HydratePage struct {
	// After resumes from the last assertion of a previous page. Empty starts.
	After string
	Limit int
	Now   time.Time
}

type HydrateResult struct {
	Scanned int
	// Repaired counts rows something was written to, whatever it was.
	Repaired int
	// Unproved counts rows whose citations the ledger would not vouch for. It
	// is not the opposite of Repaired and both can count the same row: the
	// canonical identity derives from the row's own fields, so it is written
	// even when the evidence cannot be proved. Otherwise a citation that was
	// erased, or was never true, would keep its row in the unkeyed index for
	// ever.
	Unproved int
	// Cursor is where the next page starts, empty when the sweep is done.
	Cursor string
}

func (p HydratePage) limit() int {
	// The same page a list is bounded to, and for the same reason: a sweep that
	// read everything would hold a transaction open across the whole table.
	return domain.MemoryListLimit(p.Limit)
}

/*
hydrated is what a row becomes, or false when nothing derivable is missing.

The resolver does the reading, which is why this takes its answer rather than
the resolver itself: the ledger is read outside the lock, and holding an
identity across that I/O would block every writer of the same fact for as long
as a database takes to answer.
*/
/*
hydration is what one row becomes and how much of it has to be written.

Provenance and identity are separated because they are answerable from different
places. The canonical key derives from the row's own fields, so it can always be
written; the evidence has to be proved against the ledger, and sometimes cannot
be. A row whose citation was erased still gets its key — otherwise it sits in
the unkeyed index for ever, waiting for a proof that will never come.
*/
type hydration struct {
	next domain.MemoryAssertion
	// provenance is true when the citations or the labels changed, which is
	// what the trail has to be able to reconstruct.
	provenance bool
	// key is true when the canonical identity still has to be stored. It needs
	// no event: it is recalculable from the row and does not appear in one.
	key bool
}

func (h hydration) writes() bool { return h.provenance || h.key }

func hydrated(
	stored domain.MemoryAssertion, resolved []domain.MemoryEvidence,
	proved, keyMissing bool,
) hydration {
	out := hydration{next: stored, key: keyMissing}
	if !proved {
		return out
	}
	out.next.Evidence = dedupeEvidence(resolved)

	var derived domain.Labels
	for _, ev := range out.next.Evidence {
		derived = derived.Union(ev.Labels)
	}
	out.next.Labels = stored.Labels.Union(derived)

	out.provenance = !sameEvidence(stored.Evidence, out.next.Evidence) ||
		!slices.Equal(stored.Labels, out.next.Labels)
	return out
}

// sameEvidence compares citations field by field, because hydration changes the
// fields inside a record and the keys along with them.
func sameEvidence(a, b []domain.MemoryEvidence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key() != b[i].Key() || !slices.Equal(a[i].Labels, b[i].Labels) {
			return false
		}
	}
	return true
}

// dedupeEvidence folds the hydrated citations, which is where two legacy records
// that named the same bytes by different truncations become one.
func dedupeEvidence(in []domain.MemoryEvidence) []domain.MemoryEvidence {
	return boundedEvidenceOf(in)
}

/*
resolveFor reads the ledger for one row's citations, outside any lock.

A citation the ledger will not vouch for leaves the row alone: skipped, not
rewritten. An unreachable ledger is a different answer and stops the sweep, so
the next pass tries again instead of recording that it checked.
*/
func resolveFor(
	ctx context.Context, r *Resolver, a domain.MemoryAssertion,
) ([]domain.MemoryEvidence, bool, error) {
	resolved, err := r.Resolve(ctx, a.Scope, a.Evidence)
	if errors.Is(err, ErrEvidenceInvalid) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return resolved, true, nil
}

func nowOr(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

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
		resolved, proved, err := resolveFor(ctx, r, stored)
		if err != nil {
			return out, err
		}
		if !proved {
			out.Unproved++
		}

		m.mu.Lock()
		current, held := m.values[stored.ID]
		// Somebody may have written between the read and here. Their version is
		// newer than this repair's snapshot, so it is left alone and the next
		// pass will look again.
		// The canonical key is a column the durable store keeps; this one
		// computes it when it looks, so it can never be the thing missing.
		if held && sameEvidence(current.Evidence, stored.Evidence) {
			if h := hydrated(current, resolved, proved, false); h.writes() {
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
