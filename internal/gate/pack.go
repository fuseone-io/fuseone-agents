package gate

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

// Pack is the capability set a run may draw on: the tools of the capability
// pack its author selected, resolved and frozen when the run starts.
//
// The author picks a pack, never individual tools, and never sees the tool
// catalogue (PRD SE-03). Anything outside the pack is not denied at call time
// — it is never offered to the model, so it cannot be proposed at all.
type Pack struct {
	tools []domain.ToolID
}

func NewPack(tools ...domain.ToolID) Pack {
	if len(tools) == 0 {
		return Pack{}
	}
	out := slices.Clone(tools)
	slices.Sort(out)
	return Pack{tools: slices.Compact(out)}
}

func (p Pack) Allows(tool domain.ToolID) bool {
	_, found := slices.BinarySearch(p.tools, tool)
	return found
}

func (p Pack) Tools() []domain.ToolID {
	return slices.Clone(p.tools)
}

func (p Pack) Empty() bool { return len(p.tools) == 0 }

// Narrow intersects two sets. Sub-agents inherit their parent's pack through
// this, which is what makes the guarantee hold recursively: a delegated run
// can only ever hold a subset of what delegated to it.
func (p Pack) Narrow(other Pack) Pack {
	kept := make([]domain.ToolID, 0, min(len(p.tools), len(other.tools)))
	for _, t := range p.tools {
		if other.Allows(t) {
			kept = append(kept, t)
		}
	}
	return Pack{tools: kept}
}

// Request is one proposed action arriving at the Gate.
//
// It carries plain domain values rather than a live run object: the Gate must
// be evaluable from a recorded ledger alone, which is what makes
// counterfactual replay possible (PRD AU-08).
type Request struct {
	Scope   domain.Scope
	RunID   domain.RunID
	AgentID domain.AgentID
	Seq     int64

	Tool   domain.ToolID
	Effect domain.Effect
	Args   []byte
	// ArgLabels is the taint of the arguments: the union of the labels of
	// every value they were derived from (PRD SE-05).
	ArgLabels domain.Labels

	Pack Pack

	Budget    domain.Budget
	Committed domain.Consumption
	// Estimate is what this call would reserve. The Gate checks
	// Committed+Estimate before the spend, never a total accumulated after it.
	Estimate domain.Consumption

	IdemKey string
	// AlreadyExecuted is set when the ledger already records IdemKey, meaning
	// the effect happened and a resume must not repeat it.
	AlreadyExecuted bool

	// ApprovalGranted is set when a human has already cleared this action and
	// the Gate is being re-evaluated to let it through.
	ApprovalGranted bool
}
