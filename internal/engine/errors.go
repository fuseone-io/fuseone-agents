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
	Cost     domain.Cost
	Price    domain.ModelPriceUse
	Provider string
	Model    string
	Err      error
}

func (e *SpentPlanError) Error() string {
	return fmt.Sprintf("engine: plan failed after spending %d tokens: %v",
		e.Cost.TotalTokens(), e.Err)
}

func (e *SpentPlanError) Unwrap() error { return e.Err }
