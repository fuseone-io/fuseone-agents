package memory

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// What one write leaves behind.
//
// Pure, and deliberately so: every path that writes an assertion comes through
// here, and three copies of these rules would be three places for one of them
// to drift.

// DefaultMemoryTTL is how long a memory lives when a renewal sets one.
//
// Fixed and documented for now rather than configurable: a setting nobody can
// see is a default with extra steps, and the screen says thirty days.
const DefaultMemoryTTL = 30 * 24 * time.Hour

var (
	// ErrMemoryTerminal means the stored assertion was disabled or its source
	// erased, and a write is not how either is undone. Reactivation is a
	// separate act with its own reason and its own event.
	ErrMemoryTerminal = errors.New("memory: the assertion is not active")
	// ErrCovered means an equivalent shared memory already answers this write.
	// Nothing was modified and nothing is consumed: correcting the shared
	// memory is a separate act, taken against the shared memory itself.
	ErrCovered = errors.New("memory: shared memory already covers this identity")
	// ErrMergeOrigin means the caller did not say which act is writing. The
	// field decides whether the expiry may be renewed, and its zero value is
	// the most permissive answer — so a caller that forgot it would change what
	// a memory's life means and nothing would complain.
	ErrMergeOrigin = errors.New("memory: the merge origin is not one of the three")
	// ErrEvidenceCannotExplain means the record budget cannot hold one citation
	// per label the assertion carries. Storing it anyway would keep a taint
	// while dropping the only citation that shows where it came from.
	ErrEvidenceCannotExplain = errors.New("memory: evidence cannot explain every label")
	// ErrCanonicalConflict means more than one row is this identity, and no
	// write may proceed until a person says which. The duplicates predate the
	// canonical key — the index that reveals them is deliberately not unique,
	// because a constraint would have refused the upgrade instead of surfacing
	// what was already in the table. Choosing one of them would correct half a
	// fact and leave the other half active, saying something else.
	ErrCanonicalConflict = errors.New("memory: more than one row is this identity")
)

/*
coveredBy is what a direct write gets when shared memory already answers it.

An outcome for a suggestion and a refusal for a person, which is not an
inconsistency: accepting a proposal that shared memory covers has an honest
ending — the proposal is closed and nothing is learned twice. A person
correcting a fact has no such ending. They were told the correction worked while
the row they meant to change was never touched, because the shared memory an
agent-scoped write covers is the memory every agent reads, and rewriting it from
one agent's context is not something to do quietly.

So the write says whose memory answers it, and the caller goes and improves that
one deliberately.
*/
func coveredBy(covering domain.MemoryAssertion) error {
	return fmt.Errorf("%w: %s", ErrCovered, covering.ID)
}

/*
oneOf is what both stores do with the rows they matched, in one place.

Two implementations of "is there exactly one" is one implementation and one
place for it to drift, and the store that drifts is whichever one has fewer
tests. Sorted by the caller before it gets here, so the pair named in the
refusal is the same pair whichever store answered.
*/
func oneOf(found []domain.MemoryAssertion, key string) (*domain.MemoryAssertion, error) {
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		return &found[0], nil
	}
	return nil, fmt.Errorf("%w: %s and %s are both %s",
		ErrCanonicalConflict, found[0].ID, found[1].ID, key)
}

// MergeOrigin is which act is writing, because the expiry rule differs and
// nothing else does. It is not a permission: the caller has already decided
// somebody may write.
type MergeOrigin string

const (
	OriginHuman       MergeOrigin = "human"
	OriginAccept      MergeOrigin = "accept"
	OriginAutoConfirm MergeOrigin = "auto_confirm"
)

func (o MergeOrigin) Valid() bool {
	switch o {
	case OriginHuman, OriginAccept, OriginAutoConfirm:
		return true
	default:
		return false
	}
}

// MergeOutcome says what happened, because "covered" and "merged" look the same
// from the outside and mean opposite things to the person who asked.
type MergeOutcome string

const (
	Inserted MergeOutcome = "inserted"
	Merged   MergeOutcome = "merged"
	Covered  MergeOutcome = "covered"
)

/*
MergeInput is one write and what is already there.

Stored is the row with the same canonical identity in the same namespace — the
one that may be merged into. Covering is a shared memory that answers an
agent-scoped creation without being its target: a run reads its own memory and
the shared memory, so an equivalent shared fact covers the need, and correcting
it from an agent's context would rewrite what every agent reads.
*/
type MergeInput struct {
	Incoming domain.MemoryAssertion
	Stored   *domain.MemoryAssertion
	Covering *domain.MemoryAssertion
	Origin   MergeOrigin
	Now      time.Time
}

/*
Merge decides what one write leaves behind. It touches nothing.

Deliberately pure: every path that writes an assertion — a person correcting
one, a suggestion being accepted, a policy confirming repeated observations —
goes through this, and three copies of these rules is three places for one of
them to drift. Keeping it out of the transaction is also what lets the
reconciliation job reuse it without reproducing the write path.

What it is not is a permission check. The caller has already decided somebody
may write; this decides what the row becomes.
*/
func Merge(in MergeInput) (domain.MemoryAssertion, MergeOutcome, error) {
	if !in.Origin.Valid() {
		return domain.MemoryAssertion{}, "", fmt.Errorf("%w: %q", ErrMergeOrigin, in.Origin)
	}
	if in.Stored == nil {
		if in.Covering != nil {
			return *in.Covering, Covered, nil
		}
		return in.Incoming, Inserted, nil
	}
	if in.Stored.Status != domain.MemoryActive {
		return domain.MemoryAssertion{}, "", fmt.Errorf(
			"%w: it is %s", ErrMemoryTerminal, in.Stored.Status)
	}

	out := *in.Stored
	out.Claim = in.Incoming.Claim
	out.Labels = out.Labels.Union(in.Incoming.Labels)
	out.Observations = max(out.Observations, in.Incoming.Observations)
	out.Confirmed = max(out.Confirmed, in.Incoming.Confirmed)
	out.UpdatedBy, out.UpdatedAt = in.Incoming.UpdatedBy, in.Now

	evidence, err := mergedEvidence(in.Stored.Evidence, in.Incoming.Evidence, out.Labels)
	if err != nil {
		return domain.MemoryAssertion{}, "", err
	}
	out.Evidence = evidence
	out.ExpiresAt = mergedExpiry(in, evidence)
	return out, Merged, nil
}

/*
mergedExpiry renews only when the memory learned something.

A correction that rewords a claim is not a reason to extend the life of a fact;
a citation nobody had before is. Renewal never shortens, because a suggestion
carries the learning policy's TTL and a memory somebody has been correcting for
months should not be cut back by accepting one.
*/
func mergedExpiry(in MergeInput, merged []domain.MemoryEvidence) *time.Time {
	stored := in.Stored.ExpiresAt
	if !learnedSomething(in, merged) {
		return stored
	}
	renewed := in.Now.Add(DefaultMemoryTTL)
	if stored != nil && stored.After(renewed) {
		return stored
	}
	return &renewed
}

/*
learnedSomething is true when this write brought a citation the stored row did
not have. An accept of the claim alone has taught the platform nothing it could
not already prove.

By key rather than by count: a memory already at the record cap takes a new
citation and evicts an old one, so the count is unchanged and what changed is
which citations these are.
*/
func learnedSomething(in MergeInput, merged []domain.MemoryEvidence) bool {
	if in.Origin == OriginAccept {
		return false
	}
	stored := make(map[string]bool, len(in.Stored.Evidence))
	for _, ev := range in.Stored.Evidence {
		stored[ev.Key()] = true
	}
	return slices.ContainsFunc(merged, func(ev domain.MemoryEvidence) bool {
		return !stored[ev.Key()]
	})
}
