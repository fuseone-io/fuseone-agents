package ledger

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// SimulationRuns lists the runs one simulation opened, oldest first.
//
// Read from the opening step rather than from a set kept beside it, so the
// fake answers from the same place the projection does: a store that knew this
// from somewhere else could disagree with the ledger it stands in for.
func (m *Memory) SimulationRuns(ctx context.Context, simulation string) ([]domain.RunID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Real runs carry no simulation, and treating "" as "everything unmarked"
	// would report production as a simulation.
	if simulation == "" {
		return nil, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []domain.RunID
	for id, steps := range m.runs {
		if len(steps) > 0 && simulationOf(steps[0]) == simulation {
			out = append(out, id)
		}
	}
	// In the order the cases were run, so the report reads case one as case
	// one. The id breaks a tie, because a fast set opens several in the same
	// instant and a report whose rows shuffle between reads is unreadable.
	slices.SortFunc(out, func(a, b domain.RunID) int {
		if c := m.runs[a][0].At.Compare(m.runs[b][0].At); c != 0 {
			return c
		}
		return strings.Compare(string(a), string(b))
	})
	return out, nil
}

// HasSimulation reports whether an agent has ever been simulated. The gate on
// leaving Draft (FU-10), and the same fold the durable store does in SQL.
func (m *Memory) HasSimulation(ctx context.Context, agent domain.AgentID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, steps := range m.runs {
		if len(steps) > 0 && steps[0].AgentID == agent && isSimulated(steps) {
			return true, nil
		}
	}
	return false, nil
}

// isSimulated reads the mark off a run's opening step.
func isSimulated(steps []domain.Step) bool {
	return len(steps) > 0 && startedPayload(steps[0]).Simulated
}

func simulationOf(opening domain.Step) string {
	return startedPayload(opening).Simulation
}

func startedPayload(opening domain.Step) domain.RunStartedPayload {
	var started domain.RunStartedPayload
	if opening.Kind != domain.StepRunStarted {
		return started
	}
	if err := json.Unmarshal(opening.Payload, &started); err != nil {
		return domain.RunStartedPayload{}
	}
	return started
}
