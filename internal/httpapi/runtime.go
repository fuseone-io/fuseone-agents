package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

func (s *Server) GetRuntimeHealth(
	ctx context.Context, req openapi.GetRuntimeHealthRequestObject,
) (openapi.GetRuntimeHealthResponseObject, error) {
	filter := runFilter(req.Params.Company, req.Params.Area, req.Params.AgentId, req.Params.Since, nil)
	filter, allowed := narrow(ctx, filter, domain.PermRunRead)
	if !allowed {
		return openapi.GetRuntimeHealth403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(
				domain.PermRunRead, scopeParams(req.Params.Company, req.Params.Area)),
		}, nil
	}

	health, err := s.store.RuntimeHealth(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("runtime health: %w", err)
	}
	out := runtimeHealthFrom(health)
	if health.EventDispatch != nil {
		dispatch := runtimeEventDispatchFrom(*health.EventDispatch, clockOr(s.clock).Now())
		out.EventDispatch = &dispatch
	}
	return openapi.GetRuntimeHealth200JSONResponse(out), nil
}

func runtimeHealthFrom(h domain.RuntimeHealth) openapi.RuntimeHealth {
	out := openapi.RuntimeHealth{
		ByPhase:         h.ByPhase,
		Queue:           runtimeQueueFrom(h.Queue),
		Failures:        make([]openapi.RuntimeFailureBucket, 0, len(h.Failures)),
		ToolFailures:    make([]openapi.RuntimeToolFailureBucket, 0, len(h.ToolFailures)),
		ChannelFailures: make([]openapi.RuntimeChannelFailureBucket, 0, len(h.ChannelFailures)),
		EgressDenials:   make([]openapi.RuntimeEgressDenialBucket, 0, len(h.EgressDenials)),
	}
	for _, one := range h.Failures {
		out.Failures = append(out.Failures, openapi.RuntimeFailureBucket{
			Code:      one.Code,
			Provider:  stringPtr(one.Provider),
			Status:    intPtr(one.Status),
			Retryable: ptr(one.Retryable),
			Runs:      one.Runs,
			LastAt:    one.LastAt,
		})
	}
	for _, one := range h.ToolFailures {
		out.ToolFailures = append(out.ToolFailures, openapi.RuntimeToolFailureBucket{
			Code:   one.Code,
			Calls:  one.Calls,
			Runs:   one.Runs,
			LastAt: one.LastAt,
		})
	}
	for _, one := range h.ChannelFailures {
		out.ChannelFailures = append(out.ChannelFailures, openapi.RuntimeChannelFailureBucket{
			Code: one.Code, Attempts: one.Attempts,
			Conversations: one.Conversations, ScopeWide: one.ScopeWide, Runs: one.Runs,
			FirstAt: one.FirstAt, LastAt: one.LastAt,
		})
	}
	for _, one := range h.EgressDenials {
		out.EgressDenials = append(out.EgressDenials, openapi.RuntimeEgressDenialBucket{
			Code: one.Code, Attempts: one.Attempts,
			Servers: one.Servers, Destinations: one.Destinations,
			FirstAt: one.FirstAt, LastAt: one.LastAt,
		})
	}
	return out
}

func runtimeQueueFrom(q domain.RuntimeQueue) openapi.RuntimeQueue {
	out := openapi.RuntimeQueue{
		Ready: q.Ready, Leased: q.Leased,
		BackingOff: q.BackingOff, ExpiredLeases: q.ExpiredLeases,
	}
	if !q.OldestReadyAt.IsZero() {
		out.OldestReadyAt = ptr(q.OldestReadyAt)
	}
	return out
}

// runtimeEventDispatchFrom states the backlog as ages at now, by the server's
// clock. The age of a holder the role cannot see is absent, not zero: zero
// would read as nothing open.
func runtimeEventDispatchFrom(b domain.EventDispatchBacklog, now time.Time) openapi.RuntimeEventDispatch {
	out := openapi.RuntimeEventDispatch{
		Status:               openapi.RuntimeEventDispatchStatus(b.Status(now)),
		Ready:                b.Ready,
		HeldByXmin:           b.HeldByXmin,
		ReadyCapped:          b.ReadyCapped,
		HeldCapped:           b.HeldCapped,
		CountCap:             domain.EventDispatchCountCap,
		LagSeconds:           int64(b.Lag(now).Seconds()),
		XactOpen:             b.Xact.Open,
		DegradedAfterSeconds: int64(domain.EventDispatchDegradedAfter.Seconds()),
	}
	if !b.OldestFinishedAt.IsZero() {
		out.OldestFinishedAt = ptr(b.OldestFinishedAt)
	}
	age, known := b.XactAge(now)
	out.XactAgeKnown = known
	if known && b.Xact.Open {
		out.XactAgeSeconds = ptr(int64(age.Seconds()))
	}
	return out
}
