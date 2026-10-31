package guard

import "regexp"

// Reason is the closed vocabulary a tool answers in. Every value a result
// carries is either one of these, a number, a SHA checked to be hex, or a name
// from this server's own configuration: never a string GitHub returned. The
// Merger's run reads these results, and free text would taint it (NT-013 §
// Guardrails, layer 4).
type Reason string

const (
	ReasonMerged   Reason = "merged"
	ReasonRecorded Reason = "recorded"

	ReasonInvalidInput    Reason = "invalid_input"
	ReasonWrongRepository Reason = "wrong_repository"
	ReasonWrongBase       Reason = "wrong_base"
	ReasonPRNotOpen       Reason = "pr_not_open"
	ReasonHeadMoved       Reason = "head_moved"

	ReasonReviewMissing    Reason = "review_missing"
	ReasonReviewStale      Reason = "review_stale"
	ReasonChangesRequested Reason = "changes_requested"

	ReasonQAMissing          Reason = "qa_missing"
	ReasonQAUntrustedCreator Reason = "qa_untrusted_creator"
	ReasonQANotSuccess       Reason = "qa_not_success"

	ReasonCheckMissing    Reason = "check_missing"
	ReasonCheckNotSuccess Reason = "check_not_success"

	ReasonBaseBehind          Reason = "base_behind"
	ReasonNotMergeable        Reason = "not_mergeable"
	ReasonMergeabilityUnknown Reason = "mergeability_unknown"
	ReasonMergeRefused        Reason = "merge_refused"
	// ReasonOutcomeUnknown: the merge request was sent, its response was lost,
	// and re-reading the PR could not show it merged at the proposed head.
	// Not a refusal: the merge may have happened. A person must look.
	ReasonOutcomeUnknown Reason = "outcome_unknown"
	// ReasonMergeNotSent: the merge request failed before it left the process
	// (no token, no request). Known not to have merged; safe to retry.
	ReasonMergeNotSent Reason = "merge_not_sent"

	ReasonStatusRefused Reason = "status_refused"
	ReasonWrongIdentity Reason = "wrong_identity"
)

// QAContext is the commit-status context the QA verdict lives under, and the
// one the ruleset requires from the QA App (#207).
const QAContext = "factory/qa"

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// validSHA accepts a full, lowercase commit SHA: what GitHub reports as a
// head, so a comparison against it is exact.
func validSHA(s string) bool { return shaPattern.MatchString(s) }
