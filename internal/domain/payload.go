package domain

import "time"

// Typed payloads for each step kind. They are the ledger's on-disk contract:
// an auditor reading run_steps five years from now decodes these, so treat a
// change to any field as a schema migration, not a refactor.
//
// Bulky content — prompts, tool arguments, tool results — is not stored here.
// Payloads carry a reference plus a digest, and the content itself lives in
// object storage (PRD AU-04). Arguments frequently contain personal data, and
// inlining them would make retention impossible to honour.

type RunStartedPayload struct {
	Trigger  string `json:"trigger"`
	InputRef string `json:"input_ref,omitempty"`

	// Simulated marks a run that never called a tool: the Gate decided, the
	// ledger recorded, and the tool layer answered with nothing. It is in the
	// ledger so the trail, the diagram and the verifier read it unchanged,
	// and every projection excludes it so it is never counted as production.
	Simulated bool `json:"simulated,omitempty"`
	// Simulation names the batch this run belonged to, so a report can find
	// its cases again. Both fields are written from one input by the opener:
	// two marks somebody could set independently is two marks that disagree,
	// and the disagreement that matters is a simulated run a worker claims.
	Simulation string `json:"simulation,omitempty"`
	// Case names the regression case this run replays, when it replays one.
	// The battery matches an expectation to a run by this rather than by
	// position: runs are folded in the order the ledger holds them, which is
	// not the order a corpus was written in, and checking case three against
	// case one's correction reports a failure nobody can act on while hiding
	// a real one.
	Case string `json:"case,omitempty"`

	// Origin is where the ask came from, when it came from a conversation.
	Origin *RunOrigin `json:"origin,omitempty"`
}

/*
RunOrigin is the conversation an ask arrived in, sealed on the opening step.

It is here and not in the content because it is what the run *is*, not what the
run is about: a reply belongs to the message that asked, and a reply addressed
from anywhere else is the platform choosing a recipient. Sealed once, at the
start, so the reach of a reply is fixed by provenance rather than by a rule
somebody has to remember to write (NT-005 §3).

Without it the answer to "why did this agent do that" is a screenshot. With it
the trail names the conversation, the message and the thread, and an auditor
reading it a year later can go and find what somebody typed — or find that it
was erased, which is also an answer.
*/
type RunOrigin struct {
	// Channel is the configured connection, never the vendor.
	Channel      string `json:"channel"`
	Conversation string `json:"conversation"`
	// Message is what the channel called the message that asked.
	Message string `json:"message,omitempty"`
	// Thread is where a reply belongs: the parent when the ask came inside
	// one, and the message itself when it started one.
	Thread string `json:"thread,omitempty"`
}

type PlannedPayload struct {
	Node        string `json:"node"`
	ProposalRef string `json:"proposal_ref,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
}

type GateDecidedPayload struct {
	Tool    ToolID  `json:"tool"`
	Effect  Effect  `json:"effect"`
	Verdict Verdict `json:"verdict"`
	Rule    string  `json:"rule,omitempty"`
	// Reason is a message key resolved through i18n, never a localised string.
	Reason string `json:"reason,omitempty"`
	// PolicyCode names which authored rule produced this. Without it every
	// policy decision records "policy" and nobody can count what one rule did
	// or tell two of them apart.
	PolicyCode string `json:"policy_code,omitempty"`
	// Monitored are rules that matched while watching. They changed nothing,
	// and the trail says so — otherwise a screen shows a rule denying things
	// beside a run that carried on, and somebody spends an afternoon on it.
	Monitored []MonitoredPolicy `json:"monitored,omitempty"`

	// Stage is how far the agent was trusted when this was decided. Recorded
	// because it is state beside the specification rather than in it: it
	// changes on an afternoon, and a decision replayed under today's trust is
	// not the decision that was made (PRD AU-07).
	Stage Stage `json:"stage,omitempty"`

	// Labels is the taint the arguments carried, and ArgsDigest identifies
	// them without holding them.
	//
	// Recorded because a decision is only re-evaluable if its inputs were
	// written down (AU-08): a record of the outcome alone can be replayed and
	// never re-decided. The arguments themselves are deliberately not copied
	// here — they carry whatever the case carries, and creating a second copy
	// of personal data to enable a reporting feature is the wrong trade. What
	// that costs is exact: a policy reading argument content cannot be
	// re-evaluated against a past decision, and the replay says so rather
	// than reporting it unchanged.
	Labels     Labels `json:"labels,omitempty"`
	ArgsDigest string `json:"args_digest,omitempty"`
}

type BudgetReservedPayload struct {
	Micros int64 `json:"micros,omitempty"`
	Tokens int64 `json:"tokens,omitempty"`
}

// BudgetReconciledPayload releases a reservation. The step's Cost field holds
// what was actually spent; these fields hold what is being given back.
type BudgetReconciledPayload struct {
	ReleasedMicros int64 `json:"released_micros,omitempty"`
	ReleasedTokens int64 `json:"released_tokens,omitempty"`
}

type ToolCalledPayload struct {
	Tool    ToolID `json:"tool"`
	Effect  Effect `json:"effect"`
	ArgsRef string `json:"args_ref,omitempty"`
	// ArgsDigest lets an auditor prove which arguments were used without the
	// ledger holding them.
	ArgsDigest string `json:"args_digest,omitempty"`
}

type ToolReturnedPayload struct {
	Tool      ToolID `json:"tool"`
	ResultRef string `json:"result_ref,omitempty"`
	Failed    bool   `json:"failed,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

type ApprovalRequestedPayload struct {
	Tool ToolID `json:"tool"`
	// Rule is the stable key of the check that demanded a human. The trail and
	// the inbox render it localised; Reason stays developer-facing English.
	Rule      string    `json:"rule,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`

	// What the approver is actually deciding about. Asking someone to
	// authorise a call without showing its arguments, its effect and what it
	// will cost is asking them to sign a blank page — and the record of the
	// decision would not say what was decided either.
	//
	// The arguments themselves stay in the content store: they carry whatever
	// the case carries, including personal data, and the ledger is kept for
	// years and read by people who have no business seeing it (AU-04).
	Effect     Effect      `json:"effect,omitempty"`
	ArgsRef    string      `json:"args_ref,omitempty"`
	ArgsDigest string      `json:"args_digest,omitempty"`
	Estimate   Consumption `json:"estimate,omitzero"`
	// Labels are the taint the arguments carry. The approver needs to know
	// that a field came from an untrusted source, which is usually the whole
	// reason the call was escalated (SE-06).
	Labels Labels `json:"labels,omitempty"`
}

type ApprovalDecidedPayload struct {
	Approved bool   `json:"approved"`
	By       UserID `json:"by"`
	Note     string `json:"note,omitempty"`
}

// ResumedPayload records a person returning a parked run to the queue.
//
// Parking withdraws a run because retrying will not help until somebody does
// something — raise a ceiling, fix an upstream, widen a pack. This is that
// somebody saying they have. It is deliberately not automatic: a ceiling
// raised across a company would otherwise restart every run that ever hit it,
// including the ones people have since dealt with by hand.
type ResumedPayload struct {
	By UserID `json:"by"`
	// Note is what changed, in the words of whoever changed it. The trail
	// otherwise records a run that resumed for no stated reason.
	Note string `json:"note,omitempty"`
}

// AbandonedPayload records a person deciding a run cannot go on.
//
// It is never written by the loop. Parking is the machine saying it is stuck;
// this is somebody saying it is over, which is a different fact and belongs to
// a different actor (PRD SE-08).
type AbandonedPayload struct {
	By     UserID `json:"by"`
	Reason string `json:"reason"`
	// Compensate is whether to undo what the run left standing. False is a
	// legitimate answer — sometimes the world should keep what happened — and
	// it is recorded so the trail shows it was chosen, not forgotten.
	Compensate bool `json:"compensate"`
}

type CompensatedPayload struct {
	Tool      ToolID `json:"tool"`
	ForSeq    int64  `json:"for_seq"`
	Succeeded bool   `json:"succeeded"`
}

type FailedPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable"`
}

// ParkedPayload records a run stopped awaiting human intervention. Parking is
// resumable by design: a budget ceiling raise or a fixed upstream continues
// the run from the exact step it stopped at (PRD FO-04, NF-14).
type ParkedPayload struct {
	Reason   string `json:"reason"`
	Attempts int    `json:"attempts,omitempty"`
}

type RunFinishedPayload struct {
	// Outcome is the model's closing answer, and is written only by runs
	// recorded before it moved to the content store. The chain is immutable,
	// so those runs keep it inline for ever; nothing writes it now.
	//
	// It restates whatever the agent read on the way — a name, an address, the
	// body of a ticket — and run_steps has no UPDATE and no DELETE, so an
	// erasure request could never reach it. That is why it moved.
	Outcome string `json:"outcome,omitempty"`
	// OutcomeRef and OutcomeDigest are where it lives now: the bytes in the
	// content store, under the same retention and the same erasure as a tool's
	// arguments, and a digest so an auditor can prove which answer was given
	// without the answer surviving to prove it.
	OutcomeRef    string `json:"outcome_ref,omitempty"`
	OutcomeDigest string `json:"outcome_digest,omitempty"`
	// StoppedBy is the step's declared exception, when that is why the run
	// ended here. The author's own words, recorded verbatim.
	//
	// It says the agent asserted the exception happened, and never that
	// anything checked: the condition is a sentence about the world and the
	// platform has no way to evaluate one. A trail that read as verified
	// would be claiming more than was done.
	StoppedBy string `json:"stopped_by,omitempty"`
}
