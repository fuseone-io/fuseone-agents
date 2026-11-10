package ledger

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/fuseone/agents/internal/domain"
)

// Experiments answers as the Postgres store does: an agent's experiments, newest first by
// the moment each opened, at most limit of them, with their counts and first opening.
func (m *Memory) Experiments(
	ctx context.Context, agent domain.AgentID, limit int,
) ([]domain.ExperimentListing, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	found := map[string]*domain.ExperimentListing{}
	cases := map[string]map[string]bool{}
	for _, steps := range m.runs {
		if len(steps) == 0 || steps[0].AgentID != agent || steps[0].Kind != domain.StepRunStarted {
			continue
		}
		var started domain.RunStartedPayload
		if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
			continue
		}
		// A judge run names the experiment it judges and is not one of its runs (NT-016).
		if !started.Simulated || started.Experiment == "" || started.Judge != nil {
			continue
		}
		e, seen := found[started.Experiment]
		if !seen {
			e = &domain.ExperimentListing{ID: started.Experiment, OpenedAt: steps[0].At, Opening: steps[0]}
			found[started.Experiment], cases[started.Experiment] = e, map[string]bool{}
		}
		e.Runs++
		cases[started.Experiment][started.Case] = true
		if firstOf(steps[0], e.Opening) {
			e.OpenedAt, e.Opening = steps[0].At, steps[0]
		}
	}

	out := make([]domain.ExperimentListing, 0, len(found))
	for id, e := range found {
		e.Cases = len(cases[id])
		out = append(out, *e)
	}
	// Newest first, and by name where two opened in the same instant, as Postgres orders.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].OpenedAt.Equal(out[j].OpenedAt) {
			return out[i].OpenedAt.After(out[j].OpenedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// firstOf is whether a run opened before the one held as first, by time and then by id,
// as Postgres picks it.
func firstOf(run, held domain.Step) bool {
	if !run.At.Equal(held.At) {
		return run.At.Before(held.At)
	}
	return run.RunID < held.RunID
}
