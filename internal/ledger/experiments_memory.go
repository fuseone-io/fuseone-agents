package ledger

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Experiments answers as the Postgres store does: an agent's experiments, newest first by
// the moment each opened, at most limit of them.
func (m *Memory) Experiments(ctx context.Context, agent domain.AgentID, limit int) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	opened := map[string]time.Time{}
	for _, steps := range m.runs {
		if len(steps) == 0 || steps[0].AgentID != agent || steps[0].Kind != domain.StepRunStarted {
			continue
		}
		var started domain.RunStartedPayload
		if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
			continue
		}
		if !started.Simulated || started.Experiment == "" {
			continue
		}
		if at, seen := opened[started.Experiment]; !seen || steps[0].At.Before(at) {
			opened[started.Experiment] = steps[0].At
		}
	}

	out := make([]string, 0, len(opened))
	for experiment := range opened {
		out = append(out, experiment)
	}
	// Newest first, and by name where two opened in the same instant, as Postgres orders.
	sort.Slice(out, func(i, j int) bool {
		if !opened[out[i]].Equal(opened[out[j]]) {
			return opened[out[i]].After(opened[out[j]])
		}
		return out[i] > out[j]
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
