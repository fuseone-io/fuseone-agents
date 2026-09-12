package connectortools

import (
	"context"
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

type GraviteeAcceptanceResult struct {
	Operation           string           `json:"operation"`
	Status              string           `json:"status"`
	SubscriptionID      string           `json:"subscriptionId"`
	Application         GraviteeResource `json:"application"`
	API                 GraviteeResource `json:"api"`
	Plan                GraviteeResource `json:"plan"`
	RequestedExpiration *string          `json:"requestedExpiration,omitempty"`
	DecidedBy           domain.UserID    `json:"decidedBy"`
	ConfirmedAt         string           `json:"confirmedAt,omitempty"`
	Recovered           bool             `json:"recovered,omitempty"`
}

func (g *GraviteeAcceptRuntime) finalize(
	ctx context.Context, attempt GraviteeAttempt, claimedBy string,
	snapshot GraviteeSnapshot, status GraviteeAttemptStatus, code, confirmedAt string,
	recovered bool,
) (engine.ToolResult, error) {
	now := g.now().UTC()
	result := GraviteeAcceptanceResult{
		Operation: "gravitee.accept_subscription", Status: code,
		SubscriptionID: snapshot.SubscriptionID,
		Application:    snapshot.Application.GraviteeResource,
		API:            snapshot.API, Plan: snapshot.Plan,
		RequestedExpiration: snapshot.RequestedExpiration,
		DecidedBy:           attempt.DecidedBy, ConfirmedAt: confirmedAt, Recovered: recovered,
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return engine.ToolResult{}, err
	}
	ref, err := g.content.Put(ctx, attempt.Execution.RunID, attempt.CallSeq, raw)
	if err != nil {
		return engine.ToolResult{}, err
	}
	content := ticket.ContentRef{Ref: ref, Digest: engine.ResultDigest(raw)}
	resolved, err := g.attempts.Resolve(ctx, GraviteeAttemptResolution{
		IdemKey: attempt.IdemKey, ClaimedBy: claimedBy, Status: status,
		Result: content, OutcomeCode: code, At: now,
	})
	if err != nil {
		return engine.ToolResult{}, err
	}
	return g.finishKnown(ctx, resolved)
}

func (g *GraviteeAcceptRuntime) finishKnown(
	ctx context.Context, attempt GraviteeAttempt,
) (engine.ToolResult, error) {
	switch attempt.Status {
	case GraviteeAttemptConfirmed, GraviteeAttemptTerminal:
		raw, err := g.content.Get(ctx, attempt.Result.Ref)
		if err != nil || engine.ResultDigest(raw) != attempt.Result.Digest {
			return engine.ToolResult{}, ErrGraviteeAttemptNotFound
		}
		phase := ticket.PhaseCompleted
		failed := false
		if attempt.Status == GraviteeAttemptTerminal {
			phase, failed = ticket.PhaseRejected, true
		}
		if _, err := g.tickets.FinishExecution(ctx, ticket.FinishInput{
			Execution: attempt.Execution, Phase: phase,
			Result: attempt.Result, At: g.now().UTC(),
		}); err != nil {
			return engine.ToolResult{}, err
		}
		// The effect result and ticket are already durable. If cleanup is down,
		// reconciliation sees the final attempt again and settles it later.
		_ = g.attempts.Settle(ctx, attempt.IdemKey, g.now().UTC())
		return engine.ToolResult{
			ResultRef: attempt.Result.Ref, ResultDigest: attempt.Result.Digest,
			ResultBytes: int64(len(raw)), Labels: domain.NewLabels(domain.LabelUntrusted),
			Failed: failed, ErrorCode: failureCode(failed, attempt.OutcomeCode),
		}, nil
	case GraviteeAttemptManual:
		return failed(CodeConnectorNeedsAttention), nil
	default:
		return failed(CodeConnectorOutcomeUnknown), nil
	}
}

func failureCode(failed bool, code string) string {
	if failed {
		return code
	}
	return ""
}
