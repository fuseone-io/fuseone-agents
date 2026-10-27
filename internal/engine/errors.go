package engine

import (
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

const CodeDedupeInFlight = "dedupe_in_flight"

// DedupeInFlightError means another run currently owns the semantic effect
// reservation. It is retryable supervision state, not an upstream failure.
type DedupeInFlightError struct{}

func (DedupeInFlightError) Error() string {
	return "engine: dedupe reservation is still pending"
}

func (DedupeInFlightError) Summary() domain.FailureSummary {
	return domain.FailureSummary{Code: CodeDedupeInFlight, Retryable: true}
}

/*
SpentPlanError is a planning attempt that failed after money was spent.

A turn can make more than one provider call — a learned-auto nudge is the
first case — and when a later call fails, the earlier one was still billed.
The error fails the turn exactly as its cause would have, but it carries the
spend, and the runner puts that spend on the ledger before the claim is
released: otherwise every retry burns a successful, invoiced call that no
budget, trail or FinOps figure ever sees.
*/
type SpentPlanError struct {
	Cost domain.Cost
	// Prompt is the input breakdown the billed call was made with: the
	// ledger is one projection of a spend and the pool metrics are the
	// other, and a billed attempt invisible in either under-reads exactly
	// the runs burning money on retries.
	Prompt   domain.PromptInputBreakdown
	Price    domain.ModelPriceUse
	Provider string
	Model    string
	Err      error
	// Unavailable is stamped by the runner, not the planner: the billed
	// prompt went out without these pack tools, and the spent step must say
	// so exactly as a successful one would.
	Unavailable []domain.ToolID
}

func (e *SpentPlanError) Error() string {
	return fmt.Sprintf("engine: plan failed after spending %d tokens: %v",
		e.Cost.TotalTokens(), e.Err)
}

func (e *SpentPlanError) Unwrap() error { return e.Err }
