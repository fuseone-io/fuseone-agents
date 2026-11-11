package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// errJudgeOutsideExperiment is a judge on a run that is not simulated or names no
	// experiment. The opener refuses one; this is the second door.
	errJudgeOutsideExperiment = errors.New("a judge is honoured only on a simulated run of an experiment")
	// errJudgeNotRunHere is a judge run reaching a pool with no judges wired: the real
	// pool, or one a test built without them.
	errJudgeNotRunHere = errors.New("this pool does not run judges")
)

// Judges resolves a judge run, declared here by the consumer (NT-016).
type Judges interface {
	ResolveJudge(ctx context.Context, run domain.RunID, started domain.RunStartedPayload) (Resolution, error)
}

// WithJudges wires how the simulation pool resolves a judge run. The real pool never gets
// it: a judge runs only on a simulated run.
func (w *Worker) WithJudges(j Judges) *Worker {
	w.judges = j
	return w
}

// resolveJudge is what a judge run plans with: refused outside an experiment, parked
// where no judges are wired, and otherwise the judge's own resolution.
func (w *Worker) resolveJudge(ctx context.Context, run domain.RunID, started domain.RunStartedPayload) (Resolution, error) {
	if err := judgeRefusal(run, started); err != nil && !errors.Is(err, errJudgeNotRunHere) {
		return Resolution{}, err
	}
	if w.judges == nil {
		return Resolution{}, fmt.Errorf("%s: %w", run, errJudgeNotRunHere)
	}
	return w.judges.ResolveJudge(ctx, run, started)
}

/*
judgeRefusal is why a judge run does not plan here (NT-016 § Rollout).

Its input is a case and its version the judged one, so resolving it as anything else plans
the judged agent itself on the case: a wrong run, paid for. It parks instead, where a
person can see it and resume it on a worker that runs judges.
*/
func judgeRefusal(run domain.RunID, started domain.RunStartedPayload) error {
	if !started.Simulated || started.Experiment == "" {
		return fmt.Errorf("%s: %w", run, errJudgeOutsideExperiment)
	}
	return fmt.Errorf("%s: %w", run, errJudgeNotRunHere)
}
