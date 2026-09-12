package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// GraviteeReconciliations seals final remote knowledge into the immutable run
// record before the external-attempt journal is retired.
type GraviteeReconciliations interface {
	Seal(context.Context, GraviteeAttempt, engine.ToolResult, time.Time) error
}

type reconciliationLedger interface {
	Read(context.Context, domain.RunID, int64) ([]domain.Step, error)
	Append(context.Context, domain.Step) (domain.Step, error)
}

type GraviteeReconciliationLedger struct {
	ledger reconciliationLedger
}

func NewGraviteeReconciliationLedger(ledger reconciliationLedger) *GraviteeReconciliationLedger {
	return &GraviteeReconciliationLedger{ledger: ledger}
}

func (r *GraviteeReconciliationLedger) Seal(
	ctx context.Context, attempt GraviteeAttempt, result engine.ToolResult, at time.Time,
) error {
	if r == nil || r.ledger == nil || attempt.Execution.RunID == "" ||
		attempt.CallSeq <= 0 || attempt.IdemKey == "" || !validReconciledResult(result) || at.IsZero() {
		return errors.New("connector: incomplete Gravitee reconciliation")
	}
	steps, err := r.ledger.Read(ctx, attempt.Execution.RunID, attempt.CallSeq)
	if err != nil {
		return fmt.Errorf("connector: read Gravitee call for reconciliation: %w", err)
	}
	call, err := reconciliationCallStep(steps, attempt)
	if err != nil {
		return err
	}
	if reconciliationAlreadySealed(steps[1:], attempt, result) {
		return nil
	}
	payload, err := json.Marshal(reconciliationPayload(attempt, result))
	if err != nil {
		return fmt.Errorf("connector: encode Gravitee reconciliation: %w", err)
	}
	_, appendErr := r.ledger.Append(ctx, domain.Step{
		RunID: call.RunID, Kind: domain.StepEffectReconciled,
		Scope: call.Scope, AgentID: call.AgentID, VersionID: call.VersionID,
		OnBehalfOf: call.OnBehalfOf, Labels: result.Labels,
		IdemKey: attempt.IdemKey + ":reconciled:" + result.ResultDigest,
		Payload: payload, At: at.UTC(),
	})
	if appendErr == nil {
		return nil
	}
	// Another reconciler may have won the append. Only the exact durable fact
	// turns that error into success; an unrelated failure remains visible.
	steps, readErr := r.ledger.Read(ctx, attempt.Execution.RunID, attempt.CallSeq+1)
	if readErr == nil && reconciliationAlreadySealed(steps, attempt, result) {
		return nil
	}
	return fmt.Errorf("connector: seal Gravitee reconciliation: %w", appendErr)
}

func reconciliationCallStep(steps []domain.Step, attempt GraviteeAttempt) (domain.Step, error) {
	if len(steps) == 0 || steps[0].Seq != attempt.CallSeq ||
		steps[0].Kind != domain.StepToolCalled || steps[0].IdemKey != attempt.IdemKey {
		return domain.Step{}, errors.New("connector: Gravitee call is absent from the ledger")
	}
	var called domain.ToolCalledPayload
	if err := json.Unmarshal(steps[0].Payload, &called); err != nil ||
		called.Tool != toolForAttempt(attempt) || called.Effect != domain.EffectWrite {
		return domain.Step{}, errors.New("connector: Gravitee call disagrees with the ledger")
	}
	return steps[0], nil
}

func reconciliationAlreadySealed(
	steps []domain.Step, attempt GraviteeAttempt, result engine.ToolResult,
) bool {
	for _, step := range steps {
		if step.Kind != domain.StepEffectReconciled {
			continue
		}
		var got domain.EffectReconciledPayload
		if json.Unmarshal(step.Payload, &got) == nil &&
			got == reconciliationPayload(attempt, result) {
			return true
		}
	}
	for _, step := range steps {
		switch step.Kind {
		case domain.StepToolReturned:
			var got domain.ToolReturnedPayload
			if json.Unmarshal(step.Payload, &got) == nil {
				return sameReturnedResult(got, attempt, result)
			}
			// The first return belongs to this call. If it is malformed or says
			// something else, the later audit correction is still owed.
			return false
		case domain.StepToolCalled:
			return false
		}
	}
	return false
}

func sameReturnedResult(
	got domain.ToolReturnedPayload, attempt GraviteeAttempt, result engine.ToolResult,
) bool {
	return got.Tool == toolForAttempt(attempt) && got.ResultRef == result.ResultRef &&
		got.ResultDigest == result.ResultDigest && got.ResultBytes == result.ResultBytes &&
		got.Failed == result.Failed && got.ErrorCode == result.ErrorCode
}

func reconciliationPayload(
	attempt GraviteeAttempt, result engine.ToolResult,
) domain.EffectReconciledPayload {
	return domain.EffectReconciledPayload{
		Tool: toolForAttempt(attempt), ForSeq: attempt.CallSeq,
		ResultRef: result.ResultRef, ResultDigest: result.ResultDigest,
		ResultBytes: result.ResultBytes, Failed: result.Failed, ErrorCode: result.ErrorCode,
	}
}

func toolForAttempt(attempt GraviteeAttempt) domain.ToolID {
	return domain.ToolID("gravitee." + attempt.Instance + ".accept_subscription")
}

func validReconciledResult(result engine.ToolResult) bool {
	return result.ResultRef != "" && result.ResultDigest != "" && result.ResultBytes >= 0 &&
		result.Failed == (result.ErrorCode != "")
}
