package simulate

import (
	"context"

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

func (DryTools) Reserve(context.Context, engine.Call) error { return nil }

func (DryTools) Invoke(context.Context, engine.Call) (engine.ToolResult, error) {
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

Everything else is the production collaborator — the same Gate, the same
ledger, the same clock, the same planner. A simulation that decided differently
would answer a question nobody asked.
*/
func Deps(deps engine.Deps) engine.Deps {
	deps.Tools = DryTools{}
	return deps
}
