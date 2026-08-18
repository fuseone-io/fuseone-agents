package ledger

import "github.com/fuseone/agents/internal/domain"

func outcomeReason(outcome domain.ClaimOutcome) string {
	if outcome.Failure != nil && outcome.Failure.Code != "" {
		return outcome.Failure.Code
	}
	return outcome.Reason()
}
