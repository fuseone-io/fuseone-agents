package httpapi

import (
	"context"
	"fmt"

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
	return openapi.GetRuntimeHealth200JSONResponse(runtimeHealthFrom(health)), nil
}

func runtimeHealthFrom(h domain.RuntimeHealth) openapi.RuntimeHealth {
	out := openapi.RuntimeHealth{
		ByPhase:      h.ByPhase,
		Queue:        runtimeQueueFrom(h.Queue),
		Failures:     make([]openapi.RuntimeFailureBucket, 0, len(h.Failures)),
		ToolFailures: make([]openapi.RuntimeToolFailureBucket, 0, len(h.ToolFailures)),
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
