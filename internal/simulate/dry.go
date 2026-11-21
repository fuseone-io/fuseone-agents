package simulate

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/engine"
)

/*
DryTools stands in for the tool layer during a simulation.

A simulated run is a real run with exactly one thing missing: the call itself.
The Gate still decides, the ledger still records, the budget still reserves —
because what a simulation is for is showing where the policy would have stopped
the agent, and one that skipped the Gate would answer a question nobody asked.

It answers rather than refusing. A refusal would end the run at the first
write, and the case would report "stopped at step 4" when what happened is that
nobody let it try.

And it answers with nothing rather than with something invented. A fabricated
customer would make the rest of the case a story about data that does not
exist, and the author would review it as though it were real.

It keeps no record of what it was asked. Every call is already a tool_called
step in the ledger, and the report is a fold of that — a second account here
would be one more thing to hold in step with the first, in a process that
outlives every simulation it serves.
*/
type DryTools struct{}

// ErrRehearsalCall is a live rehearsal's call that reached a tool layer. The engine parks
// every such call before any layer sees it (NT-018 phase 4); answering one here, even
// with nothing, would hand the run an empty world it would read as an answer.
var ErrRehearsalCall = errors.New("simulate: a rehearsal's call reached the tool layer")

func (DryTools) Reserve(_ context.Context, call engine.Call) error {
	if call.Rehearsal {
		return ErrRehearsalCall
	}
	return nil
}

func (DryTools) Invoke(_ context.Context, call engine.Call) (engine.ToolResult, error) {
	if call.Rehearsal {
		return engine.ToolResult{}, ErrRehearsalCall
	}
	// No reference and no labels: nothing was read, so nothing can be tainted
	// by it. Inventing a label would make the next decision a decision about
	// data the run never saw.
	return engine.ToolResult{}, nil
}

/*
Deps is the only place a simulated run's dependencies are built.

It drops whatever tool layer it was handed. That is the property the whole
feature rests on: there is no argument, no flag and no second constructor by
which a run marked simulated could reach a real system.

It drops the standing approvals too. A grant is a real authority with a daily
ceiling: asking it spends one of its uses and writes a real approval in its
owner's name. A simulated run parks for a person where the grant would have
released the real one, and the Gate's verdict is recorded either way.

It drops the semantic dedupe store too (NT-018 phase 5). Real runs of the agent share it:
a record a simulated call confirmed would answer a real call as the duplicate of an effect
that never happened, and a reservation it held would make a real run wait on nothing.

Everything else is the production collaborator — the same Gate, the same
ledger, the same clock, the same planner. A simulation that decided differently
would answer a question nobody asked.
*/
func Deps(deps engine.Deps) engine.Deps {
	deps.Tools = DryTools{}
	deps.Standing = nil
	deps.Dedupe = nil
	return deps
}
