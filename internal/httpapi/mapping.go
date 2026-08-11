package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

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
		if last := steps[len(steps)-1]; last.Kind == domain.StepRunFinished {
			out.EndedAt = ptr(last.At)
		}
	}
	if s.PendingApproval != nil {
		out.PendingApproval = ptr(toPendingApproval(runID, s))
	}
	return out
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

func problem(status int, title, detail string) openapi.Problem {
	return openapi.Problem{Title: title, Status: status, Detail: ptr(detail)}
}

// notFound builds the shared problem body every 404 in the contract reuses.
func notFound(id string) openapi.NotFoundApplicationProblemPlusJSONResponse {
	return openapi.NotFoundApplicationProblemPlusJSONResponse(
		problem(http.StatusNotFound, "Run not found", "No run with id "+id))
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

// bucketKey picks the dimension a run rolls up under. The run is always the
// accounting unit; these are only ways of grouping the same rows, which is why
// totals reconcile whatever the grouping (PRD FO-07).
func bucketKey(first domain.Step, g *openapi.GetCostRollupParamsGroupBy) string {
	switch openapi.GetCostRollupParamsGroupBy(groupByString(g)) {
	case openapi.GetCostRollupParamsGroupByCompany:
		return string(first.Scope.Company)
	case openapi.GetCostRollupParamsGroupByArea:
		return string(first.Scope.Area)
	case openapi.GetCostRollupParamsGroupByDay:
		return first.At.Format(time.DateOnly)
	default:
		return string(first.AgentID)
	}
}

func ptr[T any](v T) *T { return &v }
