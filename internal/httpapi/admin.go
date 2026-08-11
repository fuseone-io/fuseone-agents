package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

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

// adminScope is where platform-wide administration is authorised.
//
// What a tool does to the world does not vary by who calls it, so a ruling is
// installation-wide and the permission to make one is checked in the scope
// that owns the installation.
var adminScope = domain.Scope{Company: "default", Area: "platform"}

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

	items := make([]openapi.Tool, 0, len(entries))
	for _, e := range entries {
		description := e.Description
		items = append(items, openapi.Tool{
			ToolId: string(e.ID), Server: e.Server, Description: &description,
			Effect: openapi.Effect(e.Effect.String()), Untrusted: e.Untrusted,
		})
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
				problem(http.StatusBadRequest, "Classificação inválida", err.Error())),
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

	if err := s.curator.Classify(ctx, adminScope, ruling); err != nil {
		return openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				problem(http.StatusBadRequest, "Não foi possível registrar", err.Error())),
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

// forbidden names the permission and the scope that were missing. "Forbidden"
// alone leaves an operator guessing which grant to ask somebody for.
func forbidden(perm domain.Permission, scope domain.Scope) openapi.ForbiddenApplicationProblemPlusJSONResponse {
	return openapi.ForbiddenApplicationProblemPlusJSONResponse(problem(
		http.StatusForbidden, "Sem permissão",
		fmt.Sprintf("esta ação exige %s em %s", perm, scope)))
}

// Integrations is what the platform is configured to talk to, declared here by
// the consumer.
type Integrations interface {
	MCPServers(ctx context.Context) ([]domain.MCPServer, error)
	Providers(ctx context.Context) ([]domain.ModelProvider, error)
	PutMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, server domain.MCPServer) error
	DeleteMCPServer(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
	PutProvider(ctx context.Context, by domain.UserID, scope domain.Scope, provider domain.ModelProvider, apiKey string) error
	DeleteProvider(ctx context.Context, by domain.UserID, scope domain.Scope, name string) error
}

func healthFrom(seen domain.IntegrationHealth) openapi.IntegrationHealth {
	return openapi.IntegrationHealth{
		Reachable: seen.Reachable, ToolCount: seen.ToolCount,
		Detail: ptr(seen.Detail), ObservedAt: seen.ObservedAt,
		ObservedBy: ptr(seen.ObservedBy),
	}
}

func (s *Server) ListIntegrations(ctx context.Context, _ openapi.ListIntegrationsRequestObject) (openapi.ListIntegrationsResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolRead); resp != nil {
		return openapi.ListIntegrations403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}

	body := openapi.ListIntegrations200JSONResponse{
		McpServers: []openapi.MCPServer{},
		Providers:  []openapi.ModelProvider{},
	}
	if s.integrations == nil {
		return body, nil
	}

	servers, err := s.integrations.MCPServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list MCP servers: %w", err)
	}

	// What was observed, beside what was configured. A server can be enabled,
	// correct and unreachable, and only one of those three is somebody's
	// opinion.
	observed := map[string]domain.IntegrationHealth{}
	if s.health != nil {
		if observed, err = s.health.All(ctx); err != nil {
			return nil, fmt.Errorf("read integration health: %w", err)
		}
	}

	configured := map[string]bool{}
	for _, srv := range servers {
		configured[srv.Name] = true
		server := openapi.MCPServer{
			Name: srv.Name, Command: srv.Command, Args: &srv.Args, Enabled: srv.Enabled,
			Managed:   ptr(true),
			UpdatedBy: ptr(srv.UpdatedBy), UpdatedAt: ptr(srv.UpdatedAt),
		}
		// Absent when no worker has tried yet, which is a different thing from
		// a server that failed — and the screen has to be able to say so.
		if seen, tried := observed[srv.Name]; tried {
			server.Health = ptr(healthFrom(seen))
		}
		body.McpServers = append(body.McpServers, server)
	}

	// A server the platform is connected to but nobody configured here — passed
	// to the process by flag or environment. It belongs on this screen: the
	// question the screen answers is what the installation talks to, and
	// listing only what the console wrote would answer a different one.
	for name, seen := range observed {
		if configured[name] {
			continue
		}
		body.McpServers = append(body.McpServers, openapi.MCPServer{
			Name: name, Enabled: true, Managed: ptr(false),
			Health: ptr(healthFrom(seen)),
		})
	}
	slices.SortFunc(body.McpServers, func(a, b openapi.MCPServer) int {
		return strings.Compare(a.Name, b.Name)
	})

	providers, err := s.integrations.Providers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	for _, p := range providers {
		body.Providers = append(body.Providers, openapi.ModelProvider{
			Name: p.Name, Kind: openapi.ModelProviderKind(p.Kind), BaseUrl: p.BaseURL,
			// Only whether a credential exists. The credential itself never
			// leaves the vault through this API.
			Enabled: p.Enabled, HasKey: p.HasKey,
			UpdatedBy: ptr(p.UpdatedBy), UpdatedAt: ptr(p.UpdatedAt),
		})
	}
	return body, nil
}
