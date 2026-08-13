package ledger

import (
	"context"
	"slices"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// The in-memory queue mirrors the Postgres one closely enough that the shared
// contract suite passes against both. Where it cannot mirror it — a single
// process has no SKIP LOCKED — it reproduces the observable behaviour: a run
// already leased is passed over, an expired lease is claimable again.

type leaseState struct {
	attempts      int
	nextAttemptAt time.Time
	// parkedAt is how many steps the run had when the supervisor withdrew it.
	// Postgres records parking by overwriting the phase column, which any
	// later append recomputes — so a step arriving from outside (an approval,
	// an abandonment) returns the run to the queue there. Holding the length
	// is how the same thing happens here, rather than a sticky flag that made
	// this fake refuse work the real store would hand out.
	parkedAt    int
	leasedUntil time.Time
}

func (m *Memory) Claim(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error) {
	return m.claim(ctx, owner, lease, false)
}

func (m *Memory) claimSimulated(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error) {
	return m.claim(ctx, owner, lease, true)
}

func (m *Memory) claim(
	ctx context.Context, owner string, lease time.Duration, simulated bool,
) (domain.Claim, error) {
	if err := ctx.Err(); err != nil {
		return domain.Claim{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	ids := make([]domain.RunID, 0, len(m.runs))
	for id := range m.runs {
		ids = append(ids, id)
	}
	// Deterministic order, then oldest-waiting first — the same ordering the
	// SQL claim uses, so a test cannot pass here and fail there.
	slices.Sort(ids)
	slices.SortStableFunc(ids, func(a, b domain.RunID) int {
		return m.leases[a].nextAttemptAt.Compare(m.leases[b].nextAttemptAt)
	})

	for _, id := range ids {
		st := m.leases[id]
		if st.parkedAt > 0 && len(m.runs[id]) <= st.parkedAt {
			continue
		}
		if st.nextAttemptAt.After(now) {
			continue
		}
		if !st.leasedUntil.IsZero() && st.leasedUntil.After(now) {
			continue
		}

		steps := m.runs[id]
		phase := phaseOf(steps)
		if !claimable(phase) {
			continue
		}
		// Which half of the queue this is. The fake enforced nothing here for
		// as long as it existed, so a worker on the in-memory ledger would
		// claim a simulated run and execute its proposals with the real tool
		// layer — exactly what the Postgres query has always refused.
		if isSimulated(steps) != simulated {
			continue
		}

		st.leasedUntil = now.Add(lease)
		m.leases[id] = st
		m.owners[id] = owner

		who := identityOf(steps)
		return domain.Claim{
			RunID:       id,
			Scope:       who.Scope,
			AgentID:     who.AgentID,
			VersionID:   who.VersionID,
			OnBehalfOf:  who.OnBehalfOf,
			Phase:       phase,
			Attempts:    st.attempts,
			LeasedUntil: st.leasedUntil,
		}, nil
	}
	return domain.Claim{}, domain.ErrNoClaimableRun
}

func (m *Memory) Release(ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	st := m.leases[runID]
	st.leasedUntil = time.Time{}
	if outcome.Failed() {
		st.attempts++
	} else {
		st.attempts = 0
	}
	if outcome.Parked {
		st.parkedAt = len(m.runs[runID])
	}
	st.nextAttemptAt = outcome.NextAttemptAt
	if st.nextAttemptAt.IsZero() {
		st.nextAttemptAt = m.now()
	}

	m.leases[runID] = st
	delete(m.owners, runID)
	m.lastError[runID] = outcome.Reason()
	return nil
}

// phaseOf replays the projection the Postgres queue reads from a column.
//
// It deliberately reuses projectPhase rather than folding through the engine:
// dependencies point inward, so the ledger may not import the interpreter that
// sits above it. Sharing the projection is also what stops the two queues
// disagreeing about which runs a worker may pick up.
func phaseOf(steps []domain.Step) string {
	phase := ""
	for _, s := range steps {
		if p, _ := projectPhase(s); p != nil {
			phase = *p
		}
	}
	return phase
}

// runIdentity is the part of a run that never changes once recorded.
type runIdentity struct {
	Scope      domain.Scope
	AgentID    domain.AgentID
	VersionID  domain.VersionID
	OnBehalfOf domain.UserID
}

// identityOf takes the first value recorded for each field, mirroring the
// projection's "keep what is already there" rule. A later step that omits the
// agent must not blank it.
func identityOf(steps []domain.Step) runIdentity {
	var out runIdentity
	for _, s := range steps {
		if out.Scope.Company == "" {
			out.Scope.Company = s.Scope.Company
		}
		if out.Scope.Area == "" {
			out.Scope.Area = s.Scope.Area
		}
		if out.AgentID == "" {
			out.AgentID = s.AgentID
		}
		if out.VersionID == "" {
			out.VersionID = s.VersionID
		}
		if out.OnBehalfOf == "" {
			out.OnBehalfOf = s.OnBehalfOf
		}
	}
	return out
}
