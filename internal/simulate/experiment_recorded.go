package simulate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Intention is everything an experiment pins: the agent version, the configuration, the
// repetitions and the dataset version its cases were read from.
type Intention struct {
	Experiment
	DatasetVersion int64
}

// Record is what an experiment has opened: the intention it was opened under, its runs,
// and how many of its cases they cover.
type Record struct {
	Intention
	// Agent and Scope are what the runs belong to, as the opener resolved them. The scope
	// is the one recorded, not the agent's today: an agent republished elsewhere keeps its
	// old runs where they were recorded.
	Agent domain.AgentID
	Scope domain.Scope
	Runs  []domain.RunID
	Cases int
}

/*
Recorded is what an experiment's runs were opened under, and false when it has opened none
or the runs under that id are not that experiment's — a battery's, say.

Read from the ledger rather than kept anywhere else, for the reason a report is: it is
what every process reads the same, and it does not move when the corpus or the providers
do. Any run answers for the intention, because every run of an experiment is opened from
one, and the version is the one the opener pinned, not the one the request named — which
may have been none.
*/
func Recorded(ctx context.Context, steps Steps, id string) (Record, bool, error) {
	ids, err := steps.SimulationRuns(ctx, id)
	if err != nil {
		return Record{}, false, fmt.Errorf("simulate: runs of %s: %w", id, err)
	}
	if len(ids) == 0 {
		return Record{}, false, nil
	}
	out := Record{Runs: ids}
	cases := make(map[string]bool, len(ids))
	for i, run := range ids {
		step, started, err := opening(ctx, steps, run)
		if err != nil {
			return Record{}, false, err
		}
		if started.Experiment != id {
			return Record{}, false, nil
		}
		cases[started.Case] = true
		if i == 0 {
			out.Intention, out.Agent, out.Scope = recordedIntention(step, started), step.AgentID, step.Scope
		}
	}
	out.Cases = len(cases)
	return out, true, nil
}

// opening is a run's first step, decoded.
func opening(ctx context.Context, steps Steps, run domain.RunID) (domain.Step, domain.RunStartedPayload, error) {
	var started domain.RunStartedPayload
	first, err := steps.Read(ctx, run, domain.FirstSeq)
	if err != nil {
		return domain.Step{}, started, fmt.Errorf("simulate: read %s: %w", run, err)
	}
	if len(first) == 0 || first[0].Kind != domain.StepRunStarted {
		return domain.Step{}, started, fmt.Errorf("simulate: %s has no opening step", run)
	}
	if err := json.Unmarshal(first[0].Payload, &started); err != nil {
		return domain.Step{}, started, fmt.Errorf("simulate: decode the opening of %s: %w", run, err)
	}
	return first[0], started, nil
}

func recordedIntention(step domain.Step, started domain.RunStartedPayload) Intention {
	out := Intention{
		Experiment: Experiment{
			Version: step.VersionID, Repetitions: started.ExperimentRepetitions,
		},
		DatasetVersion: started.DatasetVersion,
	}
	if started.ExperimentConfig != nil {
		out.Config = *started.ExperimentConfig
	}
	return out
}
