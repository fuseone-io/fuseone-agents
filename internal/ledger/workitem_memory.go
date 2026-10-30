package ledger

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

// judgeWorkItem is the in-memory claim judgement, made under m.mu — the same
// serialisation the append itself runs under, which is what the Postgres
// store's advisory lock buys. Callers hold m.mu.
//
// Both halves are read from the steps rather than kept beside them: the claim
// a run holds is its first work_item_claimed step, and whether it is open is
// the phase its steps project. So this store cannot release a claim by
// writing anything either.
func (m *Memory) judgeWorkItem(s domain.Step) error {
	if s.Kind != domain.StepWorkItemClaimed {
		return nil
	}
	ref, err := claimedRef(s)
	if err != nil {
		return err
	}
	own, _, _ := claimOfRun(m.runs[s.RunID])
	heldElsewhere := false
	for id, steps := range m.runs {
		if id == s.RunID {
			continue
		}
		held, agent, ok := claimOfRun(steps)
		if ok && agent == s.AgentID && held == ref && !terminalPhase(phaseOfRun(steps)) {
			heldElsewhere = true
			break
		}
	}
	return judgeClaim(ref, own, heldElsewhere)
}

// claimOfRun is the card a run holds and the agent that claimed it.
func claimOfRun(steps []domain.Step) (domain.WorkItemRef, domain.AgentID, bool) {
	for _, st := range steps {
		if st.Kind != domain.StepWorkItemClaimed {
			continue
		}
		if ref, err := claimedRef(st); err == nil {
			return ref, st.AgentID, true
		}
	}
	return "", "", false
}

// phaseOfRun folds a run's steps into the phase the runs projection holds.
func phaseOfRun(steps []domain.Step) string {
	phase := ""
	for _, st := range steps {
		if p, _ := projectPhase(st); p != nil {
			phase = *p
		}
	}
	return phase
}

// WorkItemCost is what every run that claimed the card cost, whatever became
// of it — the sum of every step's cost, as the runs projection keeps it.
func (m *Memory) WorkItemCost(
	ctx context.Context, ref domain.WorkItemRef,
) (domain.WorkItemCost, error) {
	if err := ctx.Err(); err != nil {
		return domain.WorkItemCost{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := domain.WorkItemCost{Ref: ref}
	for _, steps := range m.runs {
		if held, _, ok := claimOfRun(steps); !ok || held != ref {
			continue
		}
		out.Runs++
		for _, st := range steps {
			out.Micros += st.Cost.Micros
		}
	}
	return out, nil
}
