package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Curator is the administration this server delegates rulings to, declared
// here by the consumer.
type Curator interface {
	Classify(ctx context.Context, scope domain.Scope, ruling domain.ToolClassification) error
	List(ctx context.Context, scope domain.Scope) ([]domain.ToolClassification, error)
	Events(ctx context.Context, target string, limit int) ([]domain.AdminEvent, error)
}

// Tools is the published catalogue as the administration area reads it.
//
// Read from the installation's own record rather than from a catalogue this
// process discovered: the API does not connect to MCP servers, and an operator
// asking what the platform knows should get the same answer wherever they ask.
type Tools interface {
	Tools(ctx context.Context) ([]domain.ToolEntry, error)
}

/*
answeringServers is which tool servers were reachable when last observed.

Stale observations count as silence. A worker that stopped observing — because
it was shut down, or because the server was removed from its configuration —
leaves its last "reachable" reading behind, and trusting it for ever would
report a server as answering years after it stopped existing.
*/
func (s *Server) answeringServers(ctx context.Context) (map[string]bool, error) {
	if s.health == nil {
		// Nothing observes. Every tool reads as offered, because nothing says
		// otherwise and greying out a working installation for want of a
		// health store would be worse than saying nothing.
		return nil, nil
	}
	observed, err := s.health.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("read integration health: %w", err)
	}

	fresh := clockOr(s.clock).Now().Add(-staleObservation)
	out := make(map[string]bool, len(observed))
	for name, h := range observed {
		out[name] = h.Reachable && h.ObservedAt.After(fresh)
	}
	return out, nil
}

// adminScope is where platform-wide administration is authorised.
//
// What a tool does to the world does not vary by who calls it, so a ruling is
// installation-wide and the permission to make one is checked in the scope
// that owns the installation.
var adminScope = domain.Scope{Company: "default", Area: "platform"}

// staleObservation is how long an unconfigured server stays on the screen
// after the last worker stopped saying it holds it. Generous next to the
// reconcile interval, so a worker restart does not blink every flag-configured
// server off the list.
const staleObservation = 5 * time.Minute

func (s *Server) ListTools(ctx context.Context, _ openapi.ListToolsRequestObject) (openapi.ListToolsResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolRead); resp != nil {
		return openapi.ListTools403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.tools == nil {
		return openapi.ListTools200JSONResponse{Items: []openapi.Tool{}}, nil
	}

	entries, err := s.tools.Tools(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	// Whether each tool's server answers now. The published list is what this
	// installation has ever offered and it never shrinks — two workers
	// connected to different servers would delete each other's rows if it did
	// — so liveness is read from the observations instead.
	answering, err := s.answeringServers(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]openapi.Tool, 0, len(entries))
	for _, e := range entries {
		description := e.Description
		tool := openapi.Tool{
			ToolId: string(e.ID), Server: e.Server, Description: &description,
			Effect: openapi.Effect(e.Effect.String()), Untrusted: e.Untrusted,
		}
		if e.CompensatedBy != "" {
			tool.CompensatedBy = ptr(string(e.CompensatedBy))
		}
		// No observations at all means nothing can be said, and everything
		// reads as offered. With observations, silence about a server is the
		// answer: it is not answering.
		tool.Offered = ptr(answering == nil || answering[e.Server])
		items = append(items, tool)
	}
	return openapi.ListTools200JSONResponse{Items: items}, nil
}

func (s *Server) ClassifyTool(ctx context.Context, req openapi.ClassifyToolRequestObject) (openapi.ClassifyToolResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolClassify); resp != nil {
		return openapi.ClassifyTool403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.curator == nil {
		return nil, errors.New("this installation has no administration store")
	}

	effect, err := domain.ParseEffect(string(req.Body.Effect))
	if err != nil {
		return openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}

	caller, _ := auth.PrincipalFrom(ctx)
	ruling := domain.ToolClassification{
		Tool: domain.ToolID(req.ToolId), Effect: effect, By: caller.ID,
	}
	if req.Body.Untrusted != nil {
		ruling.Untrusted = *req.Body.Untrusted
	}
	if req.Body.Reason != nil {
		ruling.Reason = *req.Body.Reason
	}
	if req.Body.CompensatedBy != nil {
		ruling.CompensatedBy = domain.ToolID(*req.Body.CompensatedBy)
	}

	if err := s.curator.Classify(ctx, adminScope, ruling); err != nil {
		return openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				notStored(err.Error())),
		}, nil
	}
	return openapi.ClassifyTool204Response{}, nil
}

func (s *Server) ListAdminEvents(ctx context.Context, req openapi.ListAdminEventsRequestObject) (openapi.ListAdminEventsResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermAuditRead); resp != nil {
		return openapi.ListAdminEvents403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.curator == nil {
		return openapi.ListAdminEvents200JSONResponse{Items: []openapi.AdminEvent{}}, nil
	}

	var target string
	if req.Params.Target != nil {
		target = *req.Params.Target
	}

	events, err := s.curator.Events(ctx, target, limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("list admin events: %w", err)
	}

	items := make([]openapi.AdminEvent, 0, len(events))
	for _, e := range events {
		event := openapi.AdminEvent{
			At: e.At, PrincipalId: string(e.Principal),
			Action: e.Action, Target: e.Target,
			Scope: &openapi.Scope{Company: string(e.Scope.Company), Area: string(e.Scope.Area)},
		}
		if len(e.Detail) > 0 {
			var detail map[string]any
			if err := json.Unmarshal(e.Detail, &detail); err == nil {
				event.Detail = &detail
			}
		}
		items = append(items, event)
	}
	return openapi.ListAdminEvents200JSONResponse{Items: items}, nil
}

// refuse checks a permission and renders the refusal, or returns nil.
//
// Hiding navigation in the console is a courtesy; this is the control. It
// names the permission that was missing, because "forbidden" alone leaves an
// operator guessing which grant to ask for.
func (s *Server) refuse(ctx context.Context, perm domain.Permission) *openapi.ForbiddenApplicationProblemPlusJSONResponse {
	if err := auth.Require(ctx, perm, adminScope); err != nil {
		body := forbidden(perm, adminScope)
		return &body
	}
	return nil
}
