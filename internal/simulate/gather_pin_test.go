package simulate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

type pinnedRuns map[domain.RunID]int64

func (p pinnedRuns) SimulationRuns(context.Context, string) ([]domain.RunID, error) {
	out := make([]domain.RunID, 0, len(p))
	for id := range p {
		out = append(out, id)
	}
	return out, nil
}

func (p pinnedRuns) Read(_ context.Context, run domain.RunID, _ int64) ([]domain.Step, error) {
	payload, err := json.Marshal(domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "sim-1", Case: string(run),
		DatasetVersion: p[run],
	})
	if err != nil {
		return nil, err
	}
	return []domain.Step{{RunID: run, Seq: 1, Kind: domain.StepRunStarted, AgentID: "triage", Payload: payload}}, nil
}

func TestGather_pinsTheReportToTheVersionItsRunsName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		runs pinnedRuns
		want int64
	}{
		{"every run names it", pinnedRuns{"a": 3, "b": 3}, 3},
		{"recorded before versions", pinnedRuns{"a": 0, "b": 0}, 0},
		// Two workers opened one window on either side of a correction. The battery
		// did not run one version, so saying it did would be a guess.
		{"its runs disagree", pinnedRuns{"a": 3, "b": 4}, 0},
		{"only some name one", pinnedRuns{"a": 3, "b": 0}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			report, err := Gather(t.Context(), tc.runs, "sim-1")
			if err != nil {
				t.Fatalf("Gather: %v", err)
			}
			if report.DatasetVersion != tc.want {
				t.Errorf("dataset version = %d, want %d", report.DatasetVersion, tc.want)
			}
		})
	}
}
