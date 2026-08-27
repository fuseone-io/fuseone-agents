package memory

import (
	"context"
	"errors"
	"slices"
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
	// SourceGone counts rows whose run the ledger no longer holds. They are
	// marked source_erased rather than left readable: the memory is not wrong,
	// its source was taken.
	SourceGone int
	// Conflicted counts rows the repair refused to touch because more than one
	// row is that identity. Nothing can be derived for either of them until a
	// person says which is the fact, and the sweep carries on: duplicate
	// spellings are exactly what the rows this job walks are made of, so
	// stopping on the first pair would abandon the population it exists for.
	Conflicted int
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
	out.next.Evidence, out.next.Labels, out.provenance =
		hydratedProvenance(stored.Evidence, stored.Labels, resolved)
	return out
}

/*
hydratedProvenance completes one record's citations and the labels they derive.

Shared by assertions and suggestions because it is the same repair: the evidence
a row carries, described as fully as the ledger now allows, and the labels that
follow from it. What differs between the two is what else may be written, which
is the caller's business and deliberately not this function's.
*/
func hydratedProvenance(
	stored []domain.MemoryEvidence, labels domain.Labels, resolved []domain.MemoryEvidence,
) ([]domain.MemoryEvidence, domain.Labels, bool) {
	evidence := dedupeEvidence(resolved)

	var derived domain.Labels
	for _, ev := range evidence {
		derived = derived.Union(ev.Labels)
	}
	next := labels.Union(derived)

	changed := !sameEvidence(stored, evidence) || !slices.Equal(labels, next)
	return evidence, next, changed
}

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
) ([]domain.MemoryEvidence, proof, error) {
	return resolveEvidence(ctx, r, a.Scope, a.Evidence)
}

/*
proof is what the ledger had to say about one row's citations.

Three answers, not two. Proved is the ordinary one. Unproved means the citation
does not match what the ledger holds — a mistake to leave alone. SourceAbsent
means the run itself is gone, which the platform already has a word for: the
memory is not wrong, its source was taken, and it stops being readable.

Collapsing the last two would leave active memory whose source we know does not
exist. Collapsing either into an error would stop the sweep on the population it
exists to repair, because a purged run is exactly what old rows cite.
*/
/*
proven is what the ledger answered about one row's citations, and when it was
asked.

Together because they travel together: the moment matters only to the
transitions the answer causes, and a transition dated by the row it changes is
how a discovery made today gets filed under a decision from months ago.
*/
type proven struct {
	evidence []domain.MemoryEvidence
	got      proof
	now      time.Time
}

// systemMemory is the author of everything nobody decided. A run was taken and
// something noticed; there is no person to name.
const systemMemory domain.UserID = "system:memory"

type proof int

const (
	proofUnproved proof = iota
	proofProved
	proofSourceAbsent
)

func resolveEvidence(
	ctx context.Context, r *Resolver, scope domain.Scope, in []domain.MemoryEvidence,
) ([]domain.MemoryEvidence, proof, error) {
	resolved, err := r.Resolve(ctx, scope, in)
	switch {
	case errors.Is(err, ErrEvidenceSourceAbsent):
		return nil, proofSourceAbsent, nil
	case errors.Is(err, ErrEvidenceInvalid):
		return nil, proofUnproved, nil
	case err != nil:
		return nil, proofUnproved, err
	}
	return resolved, proofProved, nil
}

func nowOr(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
