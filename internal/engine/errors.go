package engine

import "github.com/fuseone/agents/internal/domain"

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
