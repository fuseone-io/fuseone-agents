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
	Outcome string `json:"outcome"`
}
