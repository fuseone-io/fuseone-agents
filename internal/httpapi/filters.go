package httpapi

import (
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// runFilter builds the store's filter from query parameters.
//
// One builder for every endpoint that narrows runs, so "since" cannot mean one
// thing on a list and another on a total.
func runFilter(company, area, agentID *string, since, until *time.Time) domain.RunFilter {
	var f domain.RunFilter
	if company != nil {
		f.Scope.Company = domain.CompanyID(*company)
	}
	if area != nil {
		f.Scope.Area = domain.AreaID(*area)
	}
	if agentID != nil {
		f.AgentID = domain.AgentID(*agentID)
	}
	if since != nil {
		f.Since = *since
	}
	if until != nil {
		f.Until = *until
	}
	return f
}

// runFromSummary renders what the projection stored.
func runFromSummary(s domain.RunSummary) openapi.Run {
	run := openapi.Run{
		RunId:     string(s.RunID),
		Scope:     openapi.Scope{Company: string(s.Scope.Company), Area: string(s.Scope.Area)},
		AgentId:   string(s.AgentID),
		VersionId: string(s.VersionID),
		Phase:     openapi.Phase(s.Phase),
		Seq:       s.Seq,
		StartedAt: s.StartedAt,
		Cost:      toCost(s.Cost),
	}

	if s.OnBehalfOf != "" {
		run.OnBehalfOf = ptr(string(s.OnBehalfOf))
	}
	if !s.EndedAt.IsZero() {
		run.EndedAt = ptr(s.EndedAt)
	}
	if s.ReservedMicros != 0 {
		run.ReservedMicros = ptr(s.ReservedMicros)
	}
	if len(s.Labels) > 0 {
		run.Labels = ptr([]string(s.Labels))
	}
	if s.PendingApproval != nil {
		run.PendingApproval = &openapi.PendingApproval{
			RunId:   string(s.RunID),
			Scope:   &openapi.Scope{Company: string(s.Scope.Company), Area: string(s.Scope.Area)},
			AgentId: ptr(string(s.AgentID)),
			Tool:    string(s.PendingApproval.Tool),
			Rule:    ptr(s.PendingApproval.Rule),
			Reason:  ptr(s.PendingApproval.Reason),
			AtSeq:   s.PendingApproval.AtSeq,
		}
	}
	return run
}

// valueOr reads through an optional field, giving the zero value when absent.
func valueOr[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func stringMapOrNil(p *map[string]string) map[string]string {
	if p == nil {
		return nil
	}
	out := make(map[string]string, len(*p))
	for k, v := range *p {
		out[k] = v
	}
	return out
}
