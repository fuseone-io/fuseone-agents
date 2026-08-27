package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// ErrMovedMeanwhile means the memory changed between being read and being
// decided about. The decision was made about a row that no longer exists in
// that form, and applying it would overwrite whoever moved it.
var ErrMovedMeanwhile = errors.New("memory: the memory changed while this was being decided")

/*
ReactivateInput is one decision to make a disabled memory readable again.

An options struct rather than six parameters, and the resolver stays outside it:
the ledger is what this is proved against, not something the caller is choosing.
*/
type ReactivateInput struct {
	ID     string
	Scope  domain.Scope
	By     domain.UserID
	Reason string
	Now    time.Time
}

func (in ReactivateInput) validate() error {
	// The event carries the reason, and recordEvent trims and accepts an empty
	// one — so without this the trail would hold a memory that came back to
	// life beside a blank where the justification should be.
	if strings.TrimSpace(in.Reason) == "" {
		return fmt.Errorf("%w: a reactivation needs a reason", ErrInvalid)
	}
	return nil
}

/*
reactivable is every reason a disabled memory may not come back, in one place
and in the order a person needs to hear them.

The snapshot is what was read before the ledger was consulted, and current is
what the row is now that the identity is held. They are compared rather than
trusted because the proof was about the snapshot: a citation that changed in
between belongs to a writer whose version is newer than this decision, and
reactivating on the strength of the old one would vouch for evidence nobody
looked at.
*/
func reactivable(
	snapshot, current domain.MemoryAssertion, covering *domain.MemoryAssertion, got proof,
) error {
	if current.Status != domain.MemoryDisabled {
		return fmt.Errorf("%w: it is %s", ErrMemoryTerminal, current.Status)
	}
	if !sameEvidence(current.Evidence, snapshot.Evidence) {
		return ErrMovedMeanwhile
	}
	if got != proofProved {
		return fmt.Errorf("%w: its citations no longer prove it", ErrEvidenceInvalid)
	}
	if covering != nil {
		return coveredBy(*covering)
	}
	return nil
}

/*
reactivated is what the row becomes, and deliberately little.

Status, the expiry of this decision, and who decided it. Not the claim, not the
counters, not the creation stamp, and not the citations — a memory brought back
is the same memory, and rewriting its provenance here would be a hydration with
a person's name on it. The evidence was proved, not corrected: repairing what a
citation says is the other door.

The expiry is this decision's rather than the old one's, because a memory that
has just been vouched for should not disappear again on a date set the day it
was switched off.
*/
func reactivated(current domain.MemoryAssertion, in ReactivateInput) domain.MemoryAssertion {
	out := current
	out.Status = domain.MemoryActive
	renewed := in.Now.UTC().Add(DefaultMemoryTTL)
	out.ExpiresAt = &renewed
	out.UpdatedBy, out.UpdatedAt = in.By, in.Now.UTC()
	return out
}

// sourceGone is the answer when the run a memory cites is no longer in the
// ledger. It names the status the row has just been moved to, so a second
// attempt gets the same answer for a reason that is now recorded.
func sourceGone() error {
	return fmt.Errorf("%w: it is %s", ErrMemoryTerminal, domain.MemorySourceErased)
}

/*
Reactivate makes a disabled memory readable again, having proved it once more.

The ledger is read before the identity is held, because holding it across that
I/O would block every writer of the same fact for as long as a read takes. What
that costs is the possibility of somebody moving the row in between, which is
why the snapshot is compared against the row under the lock rather than assumed
still current.
*/
func (m *Memory) Reactivate(
	ctx context.Context, r *Resolver, in ReactivateInput,
) (domain.MemoryAssertion, error) {
	snapshot, err := m.readForReactivation(ctx, in)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	_, got, err := resolveEvidence(ctx, r, snapshot.Scope, snapshot.Evidence)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current, held := m.values[in.ID]
	if !held {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	if got == proofSourceAbsent {
		// The platform already lost the proof; this is only what noticed. The
		// row says so from now on, and the reactivation is refused for a reason
		// the next person can read off the memory itself.
		if current.Status == domain.MemoryDisabled && sameEvidence(current.Evidence, snapshot.Evidence) {
			current.Status = domain.MemorySourceErased
			m.values[in.ID] = cloneAssertion(current)
		}
		return domain.MemoryAssertion{}, sourceGone()
	}
	covering, err := m.coveringActive(current, in.Now)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := reactivable(snapshot, current, covering, got); err != nil {
		return domain.MemoryAssertion{}, err
	}

	next := reactivated(current, in)
	m.values[in.ID] = cloneAssertion(next)
	return next, nil
}

func (m *Memory) readForReactivation(
	ctx context.Context, in ReactivateInput,
) (domain.MemoryAssertion, error) {
	if err := ctx.Err(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := in.validate(); err != nil {
		return domain.MemoryAssertion{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	held, ok := m.values[in.ID]
	if !ok || !in.Scope.Contains(held.Scope) {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	// Refused before the ledger is read rather than only after it. The answer is
	// the same either way, and a row that is already active has no reason to
	// cost a read of every step of its run.
	if held.Status != domain.MemoryDisabled {
		return domain.MemoryAssertion{}, fmt.Errorf("%w: it is %s", ErrMemoryTerminal, held.Status)
	}
	return cloneAssertion(held), nil
}

// coveringActive is the shared memory that already answers this identity, if
// there is one. Only for an agent-scoped row: shared memory is what every agent
// reads, and nothing covers it but itself.
func (m *Memory) coveringActive(
	a domain.MemoryAssertion, now time.Time,
) (*domain.MemoryAssertion, error) {
	if a.AgentID == "" {
		return nil, nil
	}
	shared := a
	shared.AgentID = ""
	shared.ID = domain.MemoryAssertionID(shared)

	found, err := m.byIdentity(shared)
	if err != nil || found == nil {
		return nil, err
	}
	if found.Status != domain.MemoryActive || expired(*found, nowOrWall(now)) {
		return nil, nil
	}
	return found, nil
}
