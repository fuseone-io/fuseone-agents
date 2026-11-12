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
	// ToolScope is where the tool belongs: the runtime scope of the server or
	// connector instance that declares it, or the installation's for the
	// platform's own tools (NT-019). A run reaches a tool only when this
	// contains Scope. The zero value contains nothing, so a tool whose origin
	// nobody declared is stopped.
	ToolScope domain.Scope
	Args      []byte
	// ArgLabels is the taint of the arguments: the union of the labels of
	// every value they were derived from (PRD SE-05).
	ArgLabels domain.Labels

	Pack Pack

	// Stage is how much this agent is trusted to do on its own. An unset one
	// reads as draft: a request with no stage is a wiring mistake, and the
	// safe reading of a wiring mistake is the least trusted one.
	Stage domain.Stage

	// Compensating names the call this one undoes, when it is a compensation
	// (PRD SE-08).
	//
	// It is how a compensating tool crosses the capability check without
	// being in the pack: the author never chose it, and demanding they list
	// every undo alongside every do would make the pack a list of things
	// nobody meant to allow. The rule it stands on is narrow — you may undo
	// what you were allowed to do — and it is the Curator who decides what
	// undoes what, never the author.
	Compensating domain.ToolID

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

	// PendingReview is set when a memory suggestion can only create a review
	// item and cannot become active platform state without a later human
	// decision. The effect stays write for the trail and for authored denies;
	// this only prevents the Gate from asking for a second approval before the
	// review queue gets the item it exists to inspect.
	PendingReview bool

	// LinkProven is set by the engine when the run's own record proves a
	// $fuseone.work_item.link: the run holds a card, and its create_pull_request
	// returned an answer naming the pull request being linked. Only then is
	// the link a coordination operation; unproven, it is an ordinary write,
	// and a person's approval becomes its proof (#204). Derived from the
	// ledger, like every other input here, so a replay can derive it again.
	LinkProven bool
	// LinkRefusal is why a link can never be made, set by the engine from
	// the same record: the run holds no card, or the reference is not a pull
	// request of the card's repository. The Gate refuses such a link rather
	// than asking a person to approve something the ledger would refuse.
	LinkRefusal string
}
