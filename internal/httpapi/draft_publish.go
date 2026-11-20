package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
Publishing from the workbench (NT-018, contracts 4, 5 and 10).

The draft becomes the agent's next version through the ordinary publication,
its fences and its claim included. What the workbench adds is judged in that
publication's transaction: the agent is still at the version the draft was
based on, the draft is still the one the caller read, both scopes are the
caller's, and the draft is based on the new version as it commits.
*/

// draftMoved is the draft's own precondition failing inside a publication,
// told apart from the agent's.
type draftMoved struct{ stale *domain.StaleError }

func (d draftMoved) Error() string { return "the draft moved: " + d.stale.Error() }

type publishRefusal = openapi.PublishAgentDraftResponseObject

// PublishAgentDraft publishes the caller's agent draft.
func (s *Server) PublishAgentDraft(
	ctx context.Context, req openapi.PublishAgentDraftRequestObject,
) (openapi.PublishAgentDraftResponseObject, error) {
	agent := domain.AgentID(req.AgentId)
	if s.drafts == nil || s.publisher == nil {
		return draftPublishAbsent(req.AgentId), nil
	}
	expect, err := domain.ParseIfMatch(&req.Params.IfMatch)
	if err != nil {
		return draftPublishInvalid(CodeInvalidInput, err.Error()), nil
	}
	opened, err := s.drafts.Get(ctx, agent, draftCaller(ctx))
	if errors.Is(err, drafts.ErrNotFound) {
		return draftPublishAbsent(req.AgentId), nil
	}
	if err != nil {
		return nil, err
	}
	if !expect.Allows(true, opened.Revision) {
		return draftPublishStale(opened.Revision), nil
	}
	published, scope, refusal, err := s.publishable(ctx, opened)
	if refusal != nil || err != nil {
		return refusal, err
	}
	outcome, err := s.publishDraft(ctx, opened, published, scope)
	if refused, ok := draftPublicationRefused(req.AgentId, err); ok {
		return refused, nil
	}
	if err != nil {
		return nil, err
	}
	return s.draftPublished(ctx, opened, outcome)
}

/*
publishable is the draft as the version it would become, refused if it may
not be one: a draft with no "never" block (until candidates exist), a
destination the caller may not publish into (404, as the draft's own scope
is), or a definition the publication's checks refuse.
*/
func (s *Server) publishable(ctx context.Context, o drafts.Opened) (spec.Spec, domain.Scope, publishRefusal, error) {
	if !hasNever(o.Draft.Blocks) {
		return spec.Spec{}, domain.Scope{}, draftPublishInvalid(CodePublishNeedsNever,
			"publishing from the workbench needs a non-empty never block"), nil
	}
	definition := o.Draft.Definition()
	scope, allowed := publishScope(ctx, definition.Company, definition.Area)
	if !allowed {
		return spec.Spec{}, scope, draftPublishAbsent(string(o.Agent)), nil
	}
	_, published, err := canonical(definition)
	if err != nil {
		return spec.Spec{}, scope, draftPublishInvalid(CodeInvalidInput, err.Error()), nil
	}
	detail, err := s.checker().Check(ctx, published, clockOr(s.clock).Now())
	if err != nil || detail != "" {
		return spec.Spec{}, scope, draftPublishInvalid(CodeInvalidInput, detail), err
	}
	return published, scope, nil, nil
}

// draftOutcome is a publication from the workbench that committed.
type draftOutcome struct {
	version spec.Spec
	scope   domain.Scope
	// judged is what the publication found in the event graph as it committed.
	judged  spec.Published
	created bool
}

// publishDraft runs the publication with the draft's condition inside it.
func (s *Server) publishDraft(
	ctx context.Context, o drafts.Opened, published spec.Spec, scope domain.Scope,
) (draftOutcome, error) {
	out := draftOutcome{version: published, scope: scope}
	before, err := s.agentVersions(ctx, o.Agent)
	if err != nil {
		return out, err
	}
	guard := spec.Guard{Allow: mayPublishAt(ctx), Absent: o.BaseVersion == ""}
	if !guard.Absent {
		// The agent as the draft was based on it: a version published
		// elsewhere since is not overwritten.
		based := string(domain.AgentRevision(o.BaseVersion, false))
		if guard.Expect, err = domain.ParseIfMatch(&based); err != nil {
			return out, err
		}
	}
	condition := s.drafts.WithinPublication(drafts.Publishing{Agent: o.Agent, Expect: o.Revision, Caller: draftCaller(ctx)})
	out.judged, err = s.publisher.Publish(ctx, spec.Publication{
		Spec: published, By: callerOf(ctx), Company: scope.Company, Guard: guard,
		Within: func(ctx context.Context, tx pgx.Tx, version domain.VersionID) error {
			var stale *domain.StaleError
			if err := condition(ctx, tx, version); errors.As(err, &stale) {
				return draftMoved{stale}
			} else if err != nil {
				return err
			}
			return nil
		},
	})
	out.created = !before[published.Version]
	return out, err
}

/*
draftPublished answers a publication that committed. The draft is answered as
it was published, based on the new version, not read again: the commit
judged its ETag and set its base, and a discard landing after the commit must
not turn a version already in production into an error the person cannot
retry. The event cycles the version closes are told as the form's publication
tells them (#227), from the graph the commit judged.
*/
func (s *Server) draftPublished(
	ctx context.Context, published drafts.Opened, o draftOutcome,
) (openapi.PublishAgentDraftResponseObject, error) {
	version := o.version
	paused, err := s.afterPublication(ctx, version, o.scope)
	if err != nil {
		return nil, err
	}
	after := published
	after.BaseVersion = version.Version
	agent := published.Agent
	out := openapi.PublishAgentDraft200JSONResponse{
		Headers: openapi.PublishAgentDraft200ResponseHeaders{ETag: string(after.Revision)},
	}
	out.Body.AgentId, out.Body.VersionId = string(agent), string(version.Version)
	out.Body.Created, out.Body.Paused, out.Body.Draft = o.created, paused, draftView(after)
	out.Body.EventCycleWarning = eventCycleWarning(o.judged, auth.VisibleScopes(ctx, domain.PermAgentRead))
	return out, nil
}

// hasNever is whether the instructions say what must never happen: a never
// block with something in it (NT-018, decision 3).
func hasNever(doc []blocks.Block) bool {
	for _, b := range doc {
		if b.Kind == blocks.Never && strings.TrimSpace(b.Text) != "" {
			return true
		}
	}
	return false
}

func draftPublicationRefused(agent string, err error) (openapi.PublishAgentDraftResponseObject, bool) {
	var moved draftMoved
	switch {
	case err == nil:
		return nil, false
	case errors.As(err, &moved):
		return draftPublishStale(moved.stale.Current), true
	case agentAbsent(err), errors.Is(err, drafts.ErrNotFound):
		return draftPublishAbsent(agent), true
	}
	if stale, ok := staleFrom(err); ok {
		// The agent's precondition: its own code, and no ETag header, which
		// on this route would read as the draft's.
		stale.Body.Type = ptr(string(CodeAgentMoved))
		stale.Headers.ETag = nil
		return openapi.PublishAgentDraft412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}, true
	}
	// Refused by the version, as the file's door refuses it: nothing was written.
	if conflict, ok := publicationConflict(err); ok {
		return openapi.PublishAgentDraft409ApplicationProblemPlusJSONResponse(conflict), true
	}
	return nil, false
}

func draftPublishAbsent(agent string) openapi.PublishAgentDraftResponseObject {
	return openapi.PublishAgentDraft404ApplicationProblemPlusJSONResponse{NotFoundApplicationProblemPlusJSONResponse: notFound(agent)}
}

func draftPublishInvalid(code Code, detail string) openapi.PublishAgentDraftResponseObject {
	return openapi.PublishAgentDraft400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			refusal(http.StatusBadRequest, code, "Not publishable", detail)),
	}
}

func draftPublishStale(current domain.Revision) openapi.PublishAgentDraftResponseObject {
	stale, _ := staleFrom(&domain.StaleError{Current: current})
	return openapi.PublishAgentDraft412ApplicationProblemPlusJSONResponse{PreconditionFailedApplicationProblemPlusJSONResponse: stale}
}
