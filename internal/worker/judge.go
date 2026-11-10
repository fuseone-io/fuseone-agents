package worker

import (
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// errJudgeOutsideExperiment is a judge on a run that is not simulated or names no
	// experiment. The opener refuses one; this is the second door.
	errJudgeOutsideExperiment = errors.New("a judge is honoured only on a simulated run of an experiment")
	// errJudgeNotRunHere is a judge run reaching a worker that cannot run judges yet.
	errJudgeNotRunHere = errors.New("this worker does not run judges")
)

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
