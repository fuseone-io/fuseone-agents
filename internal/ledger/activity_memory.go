package ledger

import (
	"context"
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
