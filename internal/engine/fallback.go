package engine

import (
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNoFallbackPlanner means the ledger says the run switched to its fallback
// and this worker was handed nothing to plan with on that side.
var ErrNoFallbackPlanner = errors.New("engine: the run fell back and no fallback planner is wired")

// FallbackUnrehearsedError means an autonomous run's ledger records a switch
// to a fallback pair it was never rehearsed against.
type FallbackUnrehearsedError struct{}

func (FallbackUnrehearsedError) Error() string {
	return "engine: an autonomous run does not plan with an unrehearsed fallback"
}

// Summary parks the run at once: no number of retries rehearses the pair.
func (FallbackUnrehearsedError) Summary() domain.FailureSummary {
	return domain.FailureSummary{Code: domain.ParkFallbackUnrehearsed}
}

/*
applyFallback folds a switch away from the declared pair.

No phase change: the run goes on exactly where it was, asking someone else.
What changes is who every later planning call goes to, and that is read from
here rather than from the worker that made the switch — any worker may hold
the next turn.
*/
func (s *State) applyFallback(step domain.Step) error {
	var p domain.ModelFallbackPayload
	if err := decode(step, &p); err != nil {
		return err
	}
	if p.To.Provider == "" || p.To.Model == "" {
		return fmt.Errorf("engine: fallback at seq %d names no pair to plan with", step.Seq)
	}
	s.Fallback = &p
	return nil
}

/*
plannerFor is who plans this turn, and with which model.

The declared planner with the step's own model, until the run falls back.
After that the fallback, with no model override: a step's model names a model
of the declared provider, and the fallback pair is the whole of what the
author reviewed for this case.
*/
func (r *Runner) plannerFor(state State, start Start, stepModel string) (Planner, string, error) {
	if state.Fallback == nil {
		return r.deps.Planner, stepModel, nil
	}
	// Here and not only where the switch is made: the switch is sticky and
	// the stage is not. An agent promoted after its run fell back would
	// otherwise act alone on a pair nobody rehearsed it against.
	if start.Stage == domain.StageAutonomous {
		return nil, "", FallbackUnrehearsedError{}
	}
	if r.deps.Fallback == nil {
		return nil, "", ErrNoFallbackPlanner
	}
	return r.deps.Fallback, "", nil
}
