package engine

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// The ports the loop needs. All declared here, by the consumer, so no adapter
// package ever imports engine.

// Planner asks the model what to do next. It is the only non-deterministic
// component in the loop, which is why its output is a Proposal — something the
// Gate rules on — and never an instruction the loop obeys.
type Planner interface {
	Plan(ctx context.Context, in PlanInput) (Proposal, error)
}

// PlanInput is what the model sees. It deliberately excludes the capability
// pack's full tool list: the model is offered only the tools it may call, so
// an unavailable tool cannot be proposed in the first place.
type PlanInput struct {
	State State
	// Transcript is the conversation rebuilt from the ledger — the model's
	// entire view of the run. It is reconstructed every turn rather than kept
	// in memory, so a worker that picks up a half-finished run sees exactly
	// what its predecessor saw.
	Transcript []Turn
	Budget     domain.Budget
	// Remaining is shown to the model so it can pace itself rather than being
	// cut off mid-thought.
	Remaining domain.Consumption
	Tools     []domain.ToolID
}

// Proposal is what the model wants to do next. Nothing here has happened.
type Proposal struct {
	Tool domain.ToolID
	Args []byte
	// Estimate is the worst-case consumption of the call, used to reserve
	// budget before spending it.
	Estimate domain.Consumption
	// Cost is what the planning call itself consumed.
	Cost domain.Cost
	// Done reports that the agent considers the run complete.
	Done    bool
	Outcome string
	Node    string
}

// Tools invokes a registered tool, normally an MCP server.
type Tools interface {
	Invoke(ctx context.Context, call Call) (ToolResult, error)
}

type Call struct {
	// RunID and Seq locate the call in the ledger. The tool layer needs them
	// to file bulky results in the content store under a stable key.
	RunID domain.RunID
	Seq   int64

	Tool domain.ToolID
	Args []byte
	// IdemKey is carried through to the tool so an adapter that supports
	// idempotency natively can deduplicate on its own side too.
	IdemKey string
}

type ToolResult struct {
	// ResultRef points at the payload in object storage. Results routinely
	// carry personal data and never belong inline in the ledger (PRD AU-04).
	ResultRef string
	// Labels is the taint of the result. A tool reading external content
	// returns LabelUntrusted, and it propagates from here.
	Labels    domain.Labels
	Cost      domain.Cost
	Failed    bool
	ErrorCode string
}

// Catalog resolves a tool's effect classification, set centrally by the
// Curator at registration time (PRD DE-12).
type Catalog interface {
	Effect(domain.ToolID) (domain.Effect, bool)
}

// Gate is the deterministic checkpoint every action crosses.
type Gate interface {
	Evaluate(ctx context.Context, r gate.Request) (domain.Decision, error)
}

// Clock is injectable so runs are reproducible in tests. Business logic never
// calls time.Now directly.
type Clock interface {
	Now() time.Time
}

// SystemClock is the one every real deployment uses. It lives here so the
// wiring in cmd and in tests does not each redeclare the same three lines.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// Start is everything needed to begin or resume a run. It is passed on every
// Advance because a worker picking up an existing run has no memory of it —
// the ledger holds the state, the caller holds the configuration.
type Start struct {
	RunID      domain.RunID
	Scope      domain.Scope
	AgentID    domain.AgentID
	VersionID  domain.VersionID
	OnBehalfOf domain.UserID
	Pack       gate.Pack
	Budget     domain.Budget
	Trigger    string
}

// Status is the outcome of one Advance.
type Status struct {
	Phase Phase
	Seq   int64
	Done  bool
}
