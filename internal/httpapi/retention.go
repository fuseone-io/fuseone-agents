package httpapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Retention is how long content is kept, declared here by the consumer.
type Retention interface {
	Window(ctx context.Context) (time.Duration, error)
	SetWindow(ctx context.Context, by domain.UserID, scope domain.Scope, window time.Duration) error
}

// Erasures performs a subject's request.
type Erasures interface {
	ForSubject(ctx context.Context, by domain.UserID, scope domain.Scope,
		runs []domain.RunID, reason string) (int, error)
}

// WithRetention wires how long content is kept and how it is erased.
func (s *Server) WithRetention(retention Retention, erasures Erasures) *Server {
	s.retention, s.erasures = retention, erasures
	return s
}

const hoursPerDay = 24

func (s *Server) GetRetention(
	ctx context.Context, _ openapi.GetRetentionRequestObject,
) (openapi.GetRetentionResponseObject, error) {
	if refused := s.mayErase(ctx); refused != nil {
		return openapi.GetRetention403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.retention == nil {
		return openapi.GetRetention200JSONResponse{
			Days: int(admin.DefaultRetention.Hours()) / hoursPerDay, Configured: false,
		}, nil
	}

	window, err := s.retention.Window(ctx)
	if err != nil {
		return nil, fmt.Errorf("read retention: %w", err)
	}
	return openapi.GetRetention200JSONResponse{
		Days: int(window.Hours()) / hoursPerDay,
		// A default is a promise the installation has not made deliberately,
		// and a screen that showed it as one would be reporting a decision
		// nobody took.
		Configured: window != admin.DefaultRetention,
	}, nil
}

func (s *Server) SetRetention(
	ctx context.Context, req openapi.SetRetentionRequestObject,
) (openapi.SetRetentionResponseObject, error) {
	if refused := s.mayErase(ctx); refused != nil {
		return openapi.SetRetention403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.retention == nil || req.Body == nil {
		return openapi.SetRetention204Response{}, nil
	}

	window := time.Duration(req.Body.Days) * hoursPerDay * time.Hour
	if err := s.retention.SetWindow(ctx, callerOf(ctx), adminScope, window); err != nil {
		if errors.Is(err, admin.ErrRetentionTooShort) {
			return openapi.SetRetention400ApplicationProblemPlusJSONResponse(notStored(err.Error())), nil
		}
		return nil, fmt.Errorf("set retention: %w", err)
	}
	return openapi.SetRetention204Response{}, nil
}

func (s *Server) EraseContent(
	ctx context.Context, req openapi.EraseContentRequestObject,
) (openapi.EraseContentResponseObject, error) {
	if refused := s.mayErase(ctx); refused != nil {
		return openapi.EraseContent403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *refused,
		}, nil
	}
	if s.erasures == nil || req.Body == nil {
		return openapi.EraseContent200JSONResponse{Objects: 0}, nil
	}

	runs := make([]domain.RunID, 0, len(req.Body.Runs))
	for _, run := range req.Body.Runs {
		runs = append(runs, domain.RunID(run))
	}

	erased, err := s.erasures.ForSubject(ctx, callerOf(ctx), adminScope, runs, req.Body.Reason)
	if err != nil {
		return nil, fmt.Errorf("erase content: %w", err)
	}
	return openapi.EraseContent200JSONResponse{Objects: erased}, nil
}

// mayErase guards the three endpoints that decide whether data survives.
//
// Its own permission rather than administration in general: every other
// change here can be changed back, and this one cannot by anybody.
func (s *Server) mayErase(ctx context.Context) *openapi.ForbiddenApplicationProblemPlusJSONResponse {
	if err := auth.Require(ctx, domain.PermDataErase, adminScope); err != nil {
		body := forbidden(domain.PermDataErase, adminScope)
		return &body
	}
	return nil
}
