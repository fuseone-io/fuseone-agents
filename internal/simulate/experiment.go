package simulate

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

// MaxRepetitions is how many times an experiment may run each case (NT-015).
const MaxRepetitions = 5

/*
Experiment is what makes a batch an experiment: the configuration it runs the version
under, and how many times each case runs.

Repetitions are for structural certainty, not an average: a case broken in any repetition
is broken. Each repetition is its own run, so each one is recorded, gated and billed.
*/
type Experiment struct {
	// Version is the agent version the experiment runs. Empty is the one published now.
	Version     domain.VersionID
	Config      domain.ExperimentConfig
	Repetitions int
}

// draw is one run a batch opens: a case, and the intention that names it.
type draw struct {
	at         int
	occurrence Occurrence
	key        string
}

// drawsOf is every run a batch opens. A battery or an uploaded set runs each case once,
// under the key it always had; an experiment runs each case once per repetition.
func drawsOf(batch Batch) ([]draw, error) {
	if batch.Experiment == nil {
		out := make([]draw, 0, len(batch.Cases))
		for i, occurrence := range batch.Cases {
			out = append(out, draw{at: i, occurrence: occurrence, key: intentionOf(batch.ID, i, occurrence)})
		}
		return out, nil
	}
	repetitions := batch.Experiment.Repetitions
	if repetitions < 1 || repetitions > MaxRepetitions {
		return nil, fmt.Errorf("an experiment runs each case 1 to %d times, not %d", MaxRepetitions, repetitions)
	}
	out := make([]draw, 0, len(batch.Cases)*repetitions)
	for i, occurrence := range batch.Cases {
		for r := range repetitions {
			key := fmt.Sprintf("%s:rep:%d", intentionOf(batch.ID, i, occurrence), r+1)
			out = append(out, draw{at: i, occurrence: occurrence, key: key})
		}
	}
	return out, nil
}

// withExperiment marks an experiment's request with its id and the whole of its intention.
func withExperiment(req *trigger.Request, batch Batch) {
	if batch.Experiment == nil {
		return
	}
	config := batch.Experiment.Config
	req.Experiment = batch.ID
	req.ExperimentConfig = &config
	req.ExperimentRepetitions = batch.Experiment.Repetitions
	req.Version = batch.Experiment.Version
}
