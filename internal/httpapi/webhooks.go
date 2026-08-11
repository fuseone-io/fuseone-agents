package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/trigger"
)

// WithWebhooks wires the declared paths and their secrets.
func (s *Server) WithWebhooks(webhooks trigger.Webhooks) *Server {
	s.webhooks = webhooks
	return s
}

// ListWebhooks reports which of an agent's declared paths can actually fire.
//
// An operator needs to see the difference between a webhook that exists and
// one that works: a declared path with no secret is closed, and silence is a
// bad way to learn that.
func (s *Server) ListWebhooks(
	ctx context.Context, req openapi.ListWebhooksRequestObject,
) (openapi.ListWebhooksResponseObject, error) {
	if _, ok := s.agentScope(ctx, domain.AgentID(req.AgentId)); !ok || s.webhooks == nil {
		return openapi.ListWebhooks404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
		}, nil
	}

	hooks, err := s.webhooks.ForAgent(ctx, domain.AgentID(req.AgentId))
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}

	items := make([]openapi.Webhook, 0, len(hooks))
	for _, hook := range hooks {
		item := openapi.Webhook{Path: hook.Path, Armed: hook.Armed}
		if hook.By != "" {
			item.RotatedBy = ptr(string(hook.By))
		}
		if !hook.Rotated.IsZero() {
			item.RotatedAt = ptr(hook.Rotated)
		}
		items = append(items, item)
	}
	return openapi.ListWebhooks200JSONResponse{Items: items}, nil
}

// RotateWebhookSecret generates the secret, once.
//
// Generating and rotating are the same act: a hook either has a current secret
// or it is closed, and there is no state in between worth naming.
func (s *Server) RotateWebhookSecret(
	ctx context.Context, req openapi.RotateWebhookSecretRequestObject,
) (openapi.RotateWebhookSecretResponseObject, error) {
	absent := openapi.RotateWebhookSecret404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: notFound(req.AgentId),
	}

	scope, ok := s.agentScope(ctx, domain.AgentID(req.AgentId))
	if !ok || s.webhooks == nil {
		return absent, nil
	}

	// Generating this secret is granting the ability to make the agent run,
	// so it needs the same authority as pressing the button.
	if err := auth.Require(ctx, domain.PermRunTrigger, scope); err != nil {
		return openapi.RotateWebhookSecret403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermRunTrigger, scope),
		}, nil
	}

	path := strings.TrimPrefix(req.Path, "/")
	hook, err := s.webhooks.Find(ctx, path)
	if err != nil || hook.Agent != domain.AgentID(req.AgentId) {
		// A path belonging to another agent answers as absent rather than as
		// forbidden: which paths exist is information about the installation.
		return absent, nil
	}

	secret, err := s.webhooks.Rotate(ctx, path, callerOf(ctx), clockOr(s.clock).Now())
	if err != nil {
		return nil, fmt.Errorf("rotate webhook secret: %w", err)
	}

	return openapi.RotateWebhookSecret200JSONResponse{
		Secret: secret,
		Url:    "/hooks/" + path,
	}, nil
}

// agentScope resolves an agent to its scope, if the caller may see it at all.
func (s *Server) agentScope(ctx context.Context, agent domain.AgentID) (domain.Scope, bool) {
	if s.agents == nil {
		return domain.Scope{}, false
	}
	versions, err := s.agents.Versions(ctx, agent)
	if err != nil || len(versions) == 0 {
		return domain.Scope{}, false
	}
	if !readable(versions[0].Scope, auth.VisibleScopes(ctx, domain.PermAgentRead)) {
		return domain.Scope{}, false
	}
	return versions[0].Scope, true
}
