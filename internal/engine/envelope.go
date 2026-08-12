package engine

import (
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
envelopeOf is what a run may reach, given the steps it has already used.

A specification can declare steps: ordered stages, each naming the tools
reachable while a run sits in it (NT-003 §8). This narrows the frozen pack to
the stage the run has got to, and it does so without anything having to judge
whether a stage is finished — the proposal itself moves the run forward.

Forward stays open, and going back is refused. Forbidding the author's
second-favourite ordering would describe their first draft rather than their
process; letting a run that has already replied look the customer up again
would make the order decorative. So the envelope is every step from the
furthest one already used onwards.

A specification with no steps keeps its whole pack, which is how every agent
behaved before steps existed.
*/
// EnvelopeFor is envelopeOf, exported so the end-to-end suite can state the
// guarantee in the vocabulary an operator would use.
func EnvelopeFor(start Start, called []domain.ToolID) gate.Pack {
	return envelopeOf(start, called)
}

func envelopeOf(start Start, called []domain.ToolID) gate.Pack {
	if len(start.Steps) == 0 {
		return start.Pack
	}

	from := 0
	for _, tool := range called {
		if at, ok := stepOf(start.Steps, tool); ok && at > from {
			from = at
		}
	}

	var reachable []domain.ToolID
	for _, step := range start.Steps[from:] {
		reachable = append(reachable, step...)
	}
	// Built from the steps rather than intersected with the pack: a tool
	// granted and then left out of every step was never placed anywhere, and
	// the safe reading of that is that nobody meant it to run.
	return gate.NewPack(reachable...)
}

// stepOf reports which step first reaches a tool.
func stepOf(steps [][]domain.ToolID, tool domain.ToolID) (int, bool) {
	for i, step := range steps {
		for _, t := range step {
			if t == tool {
				return i, true
			}
		}
	}
	return 0, false
}
