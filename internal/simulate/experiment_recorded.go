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

/*
Recorded is the intention an experiment's runs were opened under, and false when it has
opened none.

Read from the ledger rather than kept anywhere else, for the reason a report is: it is
what every process reads the same. Any run answers, because every run of an experiment is
opened from one intention, and the version is the one the opener pinned, not the one the
request named — which may have been none.
*/
func Recorded(ctx context.Context, steps Steps, id string) (Intention, bool, error) {
	ids, err := steps.SimulationRuns(ctx, id)
	if err != nil {
		return Intention{}, false, fmt.Errorf("simulate: runs of %s: %w", id, err)
	}
	if len(ids) == 0 {
		return Intention{}, false, nil
	}
	first, err := steps.Read(ctx, ids[0], domain.FirstSeq)
	if err != nil {
		return Intention{}, false, fmt.Errorf("simulate: read %s: %w", ids[0], err)
	}
	if len(first) == 0 || first[0].Kind != domain.StepRunStarted {
		return Intention{}, false, fmt.Errorf("simulate: %s has no opening step", ids[0])
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(first[0].Payload, &started); err != nil {
		return Intention{}, false, fmt.Errorf("simulate: decode the opening of %s: %w", ids[0], err)
	}
	out := Intention{
		Experiment: Experiment{
			Version: first[0].VersionID, Repetitions: started.ExperimentRepetitions,
		},
		DatasetVersion: started.DatasetVersion,
	}
	if started.ExperimentConfig != nil {
		out.Config = *started.ExperimentConfig
	}
	return out, true, nil
}
