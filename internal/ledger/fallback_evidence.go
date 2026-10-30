package ledger

import "github.com/fuseone/agents/internal/domain"

/*
Leaving out runs that switched to their fallback pair.

A version is promoted on what it did with its declared provider and model. A
run that fell back produced its later decisions on a pair nobody rehearsed the
version against, so the evidence promotion reads — agreement, gate refusals,
the runs and spend beside them — leaves it out (#27). The run itself stays in
every listing: it happened and it spent.

Read from the trail like everything else here. The switch is a step, so a join
against it is the record itself rather than a flag on the projection that
could disagree with it.
*/
const (
	declaredPairRuns = "not exists (select 1 from run_steps f " +
		"where f.run_id = runs.run_id and f.kind = 'model_fallback')"
	declaredPairSteps = "not exists (select 1 from run_steps f " +
		"where f.run_id = run_steps.run_id and f.kind = 'model_fallback')"
)

// fellBack is the in-memory reading of the same join.
func fellBack(steps []domain.Step) bool {
	for _, s := range steps {
		if s.Kind == domain.StepModelFallback {
			return true
		}
	}
	return false
}
