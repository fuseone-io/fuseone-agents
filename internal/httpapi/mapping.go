package httpapi

import (
	"encoding/hex"
	"encoding/json"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Translation between the domain and the wire contract lives here and nowhere
// else. Handlers stay readable, and a change to the generated types surfaces
// as a compile error in one file instead of a dozen.

func toRun(runID domain.RunID, s engine.State, steps []domain.Step) openapi.Run {
	out := openapi.Run{
		RunId:          string(runID),
		Scope:          openapi.Scope{Company: string(s.Scope.Company), Area: string(s.Scope.Area)},
		AgentId:        string(s.AgentID),
		VersionId:      string(s.VersionID),
		Phase:          openapi.Phase(s.Phase.String()),
		Seq:            s.Seq,
		Cost:           toCostFromConsumption(s.Spent),
		ReservedMicros: ptr(s.Reserved.Micros),
	}
	if s.OnBehalfOf != "" {
		out.OnBehalfOf = ptr(string(s.OnBehalfOf))
	}
	if len(s.Labels) > 0 {
		out.Labels = ptr([]string(s.Labels))
	}
	if len(steps) > 0 {
		out.StartedAt = steps[0].At
		// The state's notion of ended, not the step kind's: a run somebody
		// abandoned has ended too, and reporting it as still running put "em
		// curso" on the duration of a run that was over. Parked stays open,
		// which is correct — it is a pause.
		if s.Terminal() {
			out.EndedAt = ptr(steps[len(steps)-1].At)
		}
	}
	if s.PendingApproval != nil {
		out.PendingApproval = ptr(toPendingApproval(runID, s))
	}
	if failure := parkedFailure(steps); failure != nil {
		out.Failure = &openapi.RunFailure{
			Code:      failure.Code,
			Provider:  stringPtr(failure.Provider),
			Status:    intPtr(failure.Status),
			RequestId: stringPtr(failure.RequestID),
			Retryable: ptr(failure.Retryable),
		}
	}
	return out
}

func parkedFailure(steps []domain.Step) *domain.FailureSummary {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Kind != domain.StepParked {
			continue
		}
		var p domain.ParkedPayload
		if err := json.Unmarshal(steps[i].Payload, &p); err != nil {
			return nil
		}
		return p.Failure
	}
	return nil
}

func toPendingApproval(runID domain.RunID, s engine.State) openapi.PendingApproval {
	return openapi.PendingApproval{
		RunId:   string(runID),
		Scope:   &openapi.Scope{Company: string(s.Scope.Company), Area: string(s.Scope.Area)},
		AgentId: ptr(string(s.AgentID)),
		Tool:    string(s.PendingApproval.Tool),
		Rule:    ptr(s.PendingApproval.Rule),
		Reason:  ptr(s.PendingApproval.Reason),
		AtSeq:   s.PendingApproval.AtSeq,
		// The moment the run stopped, not the moment it was read. An approval
		// screen counts how long somebody has been waiting on this person.
		RequestedAt: s.PendingApproval.At,
		Effect:      ptr(openapi.Effect(s.PendingApproval.Effect.String())),
	}
}

func toStep(s domain.Step) openapi.Step {
	out := openapi.Step{
		Seq:  s.Seq,
		Kind: openapi.StepKind(s.Kind),
		At:   s.At,
		Hash: hex.EncodeToString(s.Hash),
		Cost: ptr(toCost(s.Cost)),
	}
	if len(s.Labels) > 0 {
		out.Labels = ptr([]string(s.Labels))
	}
	if s.PolicyHash != "" {
		out.PolicyHash = ptr(s.PolicyHash)
	}
	if len(s.Payload) > 0 {
		var p map[string]any
		// A payload that fails to decode still yields the step with an empty
		// object rather than dropping it: the trail must never hide an entry.
		if err := json.Unmarshal(s.Payload, &p); err == nil {
			out.Payload = &p
		}
	}
	return out
}

func toCost(c domain.Cost) openapi.Cost {
	return openapi.Cost{
		Micros:           c.Micros,
		InputTokens:      ptr(c.InputTokens),
		OutputTokens:     ptr(c.OutputTokens),
		CacheReadTokens:  ptr(c.CacheReadTokens),
		CacheWriteTokens: ptr(c.CacheWriteTokens),
	}
}

func toCostFromConsumption(c domain.Consumption) openapi.Cost {
	return openapi.Cost{Micros: c.Micros, InputTokens: ptr(c.Tokens)}
}

func mustJSON(v any) []byte {
	// The payload types are closed structs of scalars; marshalling one cannot
	// fail, and a nil payload would corrupt the audit record.
	b, err := json.Marshal(v)
	if err != nil {
		panic("httpapi: payload is not serialisable: " + err.Error())
	}
	return b
}

func groupByString(g *openapi.GetCostRollupParamsGroupBy) string {
	if g == nil {
		return string(openapi.GetCostRollupParamsGroupByAgent)
	}
	return string(*g)
}

func ptr[T any](v T) *T { return &v }

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
