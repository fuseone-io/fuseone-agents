package ledger

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// AgentActivity derives the same figures the SQL side aggregates.
func (m *Memory) AgentActivity(ctx context.Context, filter domain.RunFilter) ([]domain.AgentActivity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var (
		byAgent = map[domain.AgentID]domain.AgentActivity{}
		// The id of the run each agent's LastPhase came from, so a tie is
		// broken the same way the SQL side breaks it.
		lastRun = map[domain.AgentID]domain.RunID{}
	)

	for _, steps := range m.runs {
		if len(steps) == 0 || !matches(steps[0], filter) {
			continue
		}
		summary := summarise(steps)

		a := byAgent[summary.AgentID]
		a.AgentID = summary.AgentID
		a.Runs++
		a.CostMicros += summary.Cost.Micros
		switch summary.Phase {
		case "finished":
			a.Finished++
		case "awaiting_approval", "parked":
			a.Waiting++
		}

		if newer(summary, a.LastRunAt, lastRun[summary.AgentID]) {
			a.LastRunAt = summary.StartedAt
			a.LastPhase = summary.Phase
			lastRun[summary.AgentID] = summary.RunID
		}
		byAgent[summary.AgentID] = a
	}

	out := make([]domain.AgentActivity, 0, len(byAgent))
	for _, a := range byAgent {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b domain.AgentActivity) int {
		return strings.Compare(string(a.AgentID), string(b.AgentID))
	})
	return out, nil
}

// newer reports whether this run is the most recent seen, with the id breaking
// a tie — two runs opened in the same tick must not reorder between reads.
func newer(s domain.RunSummary, best time.Time, bestID domain.RunID) bool {
	if s.StartedAt.After(best) {
		return true
	}
	return s.StartedAt.Equal(best) && s.RunID > bestID
}

// SpentSince derives the same total the SQL side sums.
func (m *Memory) SpentSince(ctx context.Context, scope domain.Scope, since time.Time) (domain.Consumption, error) {
	if err := ctx.Err(); err != nil {
		return domain.Consumption{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	filter := domain.RunFilter{Scope: scope, Since: since}
	var out domain.Consumption
	for _, steps := range m.runs {
		if len(steps) == 0 || !matches(steps[0], filter) {
			continue
		}
		summary := summarise(steps)
		out.Micros += summary.Cost.Micros
		out.Tokens += summary.Cost.TotalTokens()
		out.ToolCalls += summary.ToolCalls
		out.Steps += summary.Seq
	}
	return out, nil
}

// Agreement counts what people decided about each agent's proposals.
//
// The same fold the durable store does in SQL. It exists so a test that passes
// here cannot fail in production over a store that counted differently — the
// number decides whether an agent is trusted to act alone.
func (m *Memory) Agreement(ctx context.Context, since time.Time) ([]domain.Agreement, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	counted := map[domain.AgentID]*domain.Agreement{}
	for _, steps := range m.runs {
		if isSimulated(steps) {
			// Nobody was really asked in a simulation.
			continue
		}
		for _, step := range steps {
			if step.Kind != domain.StepApprovalDecided || step.At.Before(since) {
				continue
			}
			var decided domain.ApprovalDecidedPayload
			if err := json.Unmarshal(step.Payload, &decided); err != nil {
				continue
			}
			at, seen := counted[step.AgentID]
			if !seen {
				at = &domain.Agreement{Agent: step.AgentID}
				counted[step.AgentID] = at
			}
			if decided.Approved {
				at.Approved++
			} else {
				at.Refused++
			}
		}
	}

	out := make([]domain.Agreement, 0, len(counted))
	for _, a := range counted {
		out = append(out, *a)
	}
	slices.SortFunc(out, func(a, b domain.Agreement) int {
		return strings.Compare(string(a.Agent), string(b.Agent))
	})
	return out, nil
}
