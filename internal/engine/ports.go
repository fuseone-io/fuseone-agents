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
	// Model and Effort are the step's, when it named its own. A planner that
	// ignores them runs the agent's, which is the correct reading of a
	// provider that cannot switch model per call.
	Model  string
	Effort string
	// Remaining is shown to the model so it can pace itself rather than being
	// cut off mid-thought.
	Remaining domain.Consumption
	Tools     []domain.ToolID

	// Step is where the run is, in the author's words, and StopsWhen is the
	// exception that step declared. Both empty for an agent with no steps.
	//
	// The exception is told to the model because somebody has to judge it and
	// nothing else can: it is a sentence about the world — "não encontrar o
	// cliente" — and the platform has no way to evaluate one. Stopping takes
	// no effect, so a run that stops early does nothing a run that carried on
	// would not have, which is what makes this safe to leave to the model
	// while every effect stays the Gate's.
	Step      string
	StopsWhen string
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
	// Prompt is the composition of the input the planner sent to the model.
	// It is measured by the planner because only the planner knows the final
	// wire shape: system text, tool schemas and transcript all meet there.
	Prompt domain.PromptInputBreakdown
	// Provider and Model are the pair the planner actually called, which the
	// step records so spend can be attributed to it rather than to whatever
	// the agent declares.
	Provider string
	Model    string
	// Price is the rate provenance for this planning call. Cost is accounting;
	// this is only the explanation for zero and precision.
	Price domain.ModelPriceUse
	// Done reports that the agent considers the run complete.
	Done    bool
	Outcome string
	// Artifacts are named pieces of the final answer the run publishes by
	// reference for listeners. Empty means the run only has its ordinary
	// closing answer.
	Artifacts map[string]string
	Node      string
	// StoppedBy is the step's declared exception, when that is why the run is
	// stopping. Recorded verbatim: the trail says the model asserted it, and
	// nobody should read it as having been verified.
	StoppedBy string
}

// Tools invokes a registered tool, normally an MCP server.
type Tools interface {
	// Reserve is called before a ToolCalled step is recorded. An operational
	// refusal here means the effect has not left the worker, so the run can
	// retry without the trail claiming a call happened.
	Reserve(ctx context.Context, call Call) error
	Invoke(ctx context.Context, call Call) (ToolResult, error)
}

type Call struct {
	// RunID and Seq locate the call in the ledger. The tool layer needs them
	// to file bulky results in the content store under a stable key.
	RunID domain.RunID
	Seq   int64
	Scope domain.Scope

	Tool domain.ToolID
	Args []byte
	// OnBehalfOf is the human delegation the run is using. Tool transports use
	// it only to choose the credential owned by that human; the Gate has
	// already decided whether the call may happen at all.
	OnBehalfOf domain.UserID
	// IdemKey is carried through to the tool so an adapter that supports
	// idempotency natively can deduplicate on its own side too.
	IdemKey string
	// ContextArtifacts is the event-supplied contract this run may retrieve
	// through the platform-owned context reader.
	ContextArtifacts []domain.ContextArtifact
}

type ToolResult struct {
	// ResultRef points at the payload in object storage. Results routinely
	// carry personal data and never belong inline in the ledger (PRD AU-04).
	ResultRef string
	// Labels is the taint of the result. A tool reading external content
	// returns LabelUntrusted, and it propagates from here.
	Labels        domain.Labels
	Cost          domain.Cost
	Failed        bool
	ErrorCode     string
	Cached        bool
	CachedFromRun domain.RunID
	CachedFromSeq int64
	Context       *domain.ContextArtifact
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

	// Steps are the envelopes a run advances through, in order, as data rather
	// than as spec types: the dependencies point inward, and engine cannot
	// import the package that parses definitions. Empty means one envelope
	// holding the whole pack.
	Steps  []Envelope
	Budget domain.Budget
	// Stage is how far this agent is trusted to act alone. It is state beside
	// the specification rather than in it — promotion is not a new version —
	// so it arrives with the run rather than with the definition.
	Stage   domain.Stage
	Trigger string
}

// Status is the outcome of one Advance.
type Status struct {
	Phase Phase
	Seq   int64
	Done  bool
}

// Envelope is one declared step: what it reaches, and what it is called.
//
// The name travels with it because the ledger records which step a proposal
// came from, and an auditor reading that trail in two years is better served
// by "Responder" than by "2".
type Envelope struct {
	Name    string
	Reaches []domain.ToolID

	// StopsWhen is the exception, in the author's own words: the condition on
	// which the run gives up here rather than carrying on. Nothing evaluates
	// it — it is told to the model, which may answer that it happened.
	StopsWhen string

	// Model and Effort are what this step is worth spending on. Empty means
	// the agent's own, which is what almost every step uses: the lever exists
	// because one step reasons and another classifies, and paying the
	// reasoning price for both is how an agent becomes expensive for nothing
	// (PRD FO-10, FO-11).
	Model  string
	Effort string
}
