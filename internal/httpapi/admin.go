package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/known"
)

// Curator is the administration this server delegates rulings to, declared
// here by the consumer.
type Curator interface {
	Classify(ctx context.Context, scope domain.Scope, ruling domain.ToolClassification) error
	List(ctx context.Context, scope domain.Scope) ([]domain.ToolClassification, error)
	Events(ctx context.Context, target, cursor string, limit int) ([]domain.AdminEvent, string, error)
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

// adminScope is the platform administration area used by operational settings
// that are still ordinary scoped operations. Identity administration is not
// checked here: it can mint installation administrators, so it uses the
// installation scope below.
var adminScope = domain.Scope{Company: "default", Area: "platform"}

// identityScope is where changing who may administer the installation is
// authorised. Using adminScope here would let somebody with identity:write in
// the ordinary default/platform area mint installation-wide administrators.
var identityScope = domain.Scope{Company: domain.Installation}

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

	// Who would notice this tool going away, read once for the whole listing.
	// A query per tool would ask the same question of the same rows twenty
	// times to draw one screen.
	declaring, err := s.declaringTools(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]openapi.Tool, 0, len(entries))
	for _, e := range entries {
		description := e.Description
		tool := openapi.Tool{
			ToolId: string(e.ID), Server: e.Server, Description: &description,
			Effect: openapi.ToolEffect(e.Effect.String()), Untrusted: e.Untrusted,
		}
		if e.CompensatedBy != "" {
			tool.CompensatedBy = ptr(string(e.CompensatedBy))
		}
		// What the platform already knows about this server, resolved on read
		// rather than stored. It is derived from a table shipped in the
		// binary, and a derived value persisted is one that goes stale against
		// the table it came from.
		if found := s.suggestionFor(e.ID); found != nil {
			tool.Suggested = found
		}
		// No observations at all means nothing can be said, and everything
		// reads as offered. With observations, silence about a server is the
		// answer: it is not answering.
		tool.Offered = ptr(answering == nil || answering[e.Server])
		// What is on offer now, so a ruling made on this screen can name the
		// definition it judged — and whether an existing ruling was already
		// overtaken by one.
		if e.Digest != "" {
			tool.Digest = ptr(e.Digest)
		}
		if e.Stale {
			tool.Stale = ptr(true)
		}
		tool.OnSurface = ptr(e.OnSurface)
		if named := declaring[e.ID]; len(named) > 0 {
			tool.DeclaredBy = &named
		}
		items = append(items, tool)
	}
	return openapi.ListTools200JSONResponse{Items: items}, nil
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

	var cursor string
	if req.Params.Cursor != nil {
		cursor = *req.Params.Cursor
	}
	events, next, err := s.curator.Events(ctx, target, cursor, limitOf(req.Params.Limit))
	if err != nil {
		return nil, fmt.Errorf("list admin events: %w", err)
	}

	page := openapi.ListAdminEvents200JSONResponse{Items: make([]openapi.AdminEvent, 0, len(events))}
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
		page.Items = append(page.Items, event)
	}
	if next != "" {
		page.NextCursor = &next
	}
	return page, nil
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

// Suggesters is what the platform ships about servers other people publish,
// declared here by the consumer.
type Suggesters interface {
	Suggest(server, remoteName string) (known.Suggestion, bool)
	// All is every recipe, for the screen that offers them.
	All() []known.Entry
}

// WithKnown wires the shipped suggestions.
func (s *Server) WithKnown(k Suggesters) *Server {
	s.known = k
	return s
}

// suggestionFor is what the platform believes about one tool, or nothing.
//
// The tool id is `server.remoteName` and it is split on the first dot, which
// is the same split that made it — a server named with a dot in it would be
// namespaced ambiguously long before this line saw it.
func (s *Server) suggestionFor(id domain.ToolID) *openapi.ToolSuggestion {
	if s.known == nil {
		return nil
	}
	server, remote, ok := strings.Cut(string(id), ".")
	if !ok {
		return nil
	}
	found, ok := s.known.Suggest(server, remote)
	if !ok {
		return nil
	}

	out := &openapi.ToolSuggestion{
		Effect: openapi.Effect(found.Effect), Why: found.Why,
	}
	if found.Untrusted != nil {
		out.Untrusted = found.Untrusted
	}
	if found.CompensatedBy != "" {
		out.CompensatedBy = ptr(found.CompensatedBy)
	}
	return out
}
