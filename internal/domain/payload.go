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
