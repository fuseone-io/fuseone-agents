package simulate

import (
	"context"
	"sync"

	"github.com/fuseone/agents/internal/engine"
)

/*
DryTools stands in for the tool layer during a simulation.

A dry run is a real run with exactly one thing missing: the call itself. The
Gate still decides, the ledger still records, the budget still reserves —
because what a simulation is for is showing where the policy would have stopped
the agent, and one that skipped the Gate would answer a question nobody asked.

It answers rather than refusing. A refusal would end the run at the first
write, and the case would report "stopped at step 4" when what happened is that
nobody let it try.

And it answers with nothing rather than with something invented. A fabricated
customer would make the rest of the case a story about data that does not
exist, and the author would review it as though it were real.
*/
type DryTools struct {
	mu    sync.Mutex
	calls []engine.Call
}

func NewDryTools() *DryTools { return &DryTools{} }

func (d *DryTools) Invoke(_ context.Context, call engine.Call) (engine.ToolResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, call)

	// No reference and no labels: nothing was read, so nothing can be tainted
	// by it. Inventing a label would make the next decision a decision about
	// data the run never saw.
	return engine.ToolResult{}, nil
}

// Calls is what the agent would have done, in order.
//
// Not the report. What the Gate decided, what each call cost and where the run
// stopped are already in the ledger the simulated run wrote, and the report is
// a fold of it — like every other projection here. A second account kept in
// this struct would be a second thing to hold in step with the first.
func (d *DryTools) Calls() []engine.Call {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]engine.Call(nil), d.calls...)
}
