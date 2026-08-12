package simulate

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Steps is the read side a report needs, declared here by the consumer.
type Steps interface {
	SimulationRuns(ctx context.Context, simulation string) ([]domain.RunID, error)
	Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error)
}

/*
Gather folds the runs a simulation opened into its report.

Read from the ledger every time rather than accumulated anywhere, so a report
outlives the process that produced it and reads the same from any of them —
including while the simulation is still going, when the cases that have settled
are rows and the rest say so.
*/
func Gather(ctx context.Context, steps Steps, id string) (Report, error) {
	ids, err := steps.SimulationRuns(ctx, id)
	if err != nil {
		return Report{}, fmt.Errorf("simulate: runs of %s: %w", id, err)
	}

	report := Report{ID: id, Cases: make([]Case, 0, len(ids))}
	for _, runID := range ids {
		read, err := steps.Read(ctx, runID, domain.FirstSeq)
		if err != nil {
			return Report{}, fmt.Errorf("simulate: read %s: %w", runID, err)
		}
		if report.Agent == "" && len(read) > 0 {
			report.Agent, report.Version = read[0].AgentID, read[0].VersionID
		}

		folded, err := Fold(read)
		if err != nil {
			return Report{}, err
		}
		// A case that has not settled is a case still being advanced by the
		// pool. There is nothing else a simulation could be waiting on.
		report.Running = report.Running || folded.Settled == SettledUnsettled
		report.Cases = append(report.Cases, folded)
	}
	return report, nil
}
