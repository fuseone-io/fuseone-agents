package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
Recording what a tool does, and refusing to record it about the wrong thing.

Its own file because it is its own subject: the single point where write access
enters the platform, and the only handler that answers 409.
*/

func (s *Server) ClassifyTool(ctx context.Context, req openapi.ClassifyToolRequestObject) (openapi.ClassifyToolResponseObject, error) {
	// A level given must be one there is. Only a level left out means the
	// platform's, as before: a typo falling back to the ceiling would write a
	// ruling stronger than the owner's (NT-019).
	if req.Body.Level != nil && !req.Body.Level.Valid() {
		return openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("level is owner or platform; leave it out for the platform's")),
		}, nil
	}
	if req.Body.Level != nil && *req.Body.Level == openapi.RulingLevelOwner {
		return s.classifyAsOwner(ctx, req)
	}
	if resp := s.refuse(ctx, domain.PermToolClassify); resp != nil {
		return openapi.ClassifyTool403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.curator == nil {
		return nil, errors.New("this installation has no administration store")
	}
	ruling, refused, err := s.rulingFrom(ctx, req)
	if refused != nil || err != nil {
		return refused, err
	}
	if err := s.curator.Classify(ctx, adminScope, ruling); err != nil {
		return openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				notStored(err.Error())),
		}, nil
	}
	return openapi.ClassifyTool204Response{}, nil
}

// rulingFrom reads the ruling a request makes, attributed to its caller, and
// refuses one that names no definition or another than the one on offer.
func (s *Server) rulingFrom(
	ctx context.Context, req openapi.ClassifyToolRequestObject,
) (domain.ToolClassification, openapi.ClassifyToolResponseObject, error) {
	effect, err := domain.ParseEffect(string(req.Body.Effect))
	if err != nil {
		return domain.ToolClassification{}, openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}, nil
	}
	caller, _ := auth.PrincipalFrom(ctx)
	ruling := domain.ToolClassification{
		Tool: domain.ToolID(req.ToolId), Effect: effect, By: caller.ID,
		Untrusted: valueOr(req.Body.Untrusted), Reason: valueOr(req.Body.Reason),
		CompensatedBy: domain.ToolID(valueOr(req.Body.CompensatedBy)), Digest: valueOr(req.Body.Digest),
	}
	if req.Body.Dedupe != nil {
		ruling.Dedupe = domain.ToolDedupe{
			WindowSeconds: req.Body.Dedupe.WindowSeconds,
			ArgPaths:      append([]string(nil), req.Body.Dedupe.ArgPaths...),
		}
	}
	refused, err := s.judgedRefusal(ctx, ruling)
	return ruling, refused, err
}

func (s *Server) judgedRefusal(ctx context.Context, ruling domain.ToolClassification) (openapi.ClassifyToolResponseObject, error) {
	switch known, err := s.judged(ctx, ruling); {
	case err != nil:
		return nil, err
	case known == judgedNothing:
		// A ruling that names no definition, for a tool whose definition we
		// hold. Refused rather than stored: an empty digest is how a ruling
		// from before this existed keeps working, so accepting one now would
		// hand any caller a way back to classification by name.
		return openapi.ClassifyTool400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("this tool has a definition, so a ruling has to say which one it judged")),
		}, nil
	case known == judgedSomethingElse:
		return openapi.ClassifyTool409ApplicationProblemPlusJSONResponse{
			Title:  "The tool changed",
			Detail: ptr("This tool's definition changed while you were reading it. Look again before ruling."),
			Status: http.StatusConflict,
		}, nil
	}
	return nil, nil
}

/*
What a ruling was made about, against what the catalogue holds.

The digest comes from the client because that is what makes it mean anything:
the Curator read a description and a schema, and stamping whatever is current
at save time would record a judgement of something nobody read. The same shape
as an approval carrying the step it approved.

Three answers rather than two. A ruling that names the current definition is
recorded; one that names another is refused so somebody looks again; and one
that names *nothing*, for a tool whose definition we hold, is refused as well.
That third case is the one worth being strict about: an empty digest is how a
ruling recorded before any of this existed keeps working, so a caller allowed
to send one now — an old console, a script, a curl — walks straight back into
classification by name, which is the thing this replaced.
*/
type judgement int

const (
	judgedTheCurrentDefinition judgement = iota
	judgedSomethingElse
	judgedNothing
)

func (s *Server) judged(ctx context.Context, ruling domain.ToolClassification) (judgement, error) {
	if s.tools == nil {
		return judgedTheCurrentDefinition, nil
	}
	listed, err := s.tools.Tools(ctx)
	if err != nil {
		return judgedTheCurrentDefinition, err
	}
	for _, tool := range listed {
		if tool.ID != ruling.Tool {
			continue
		}
		switch {
		case tool.Digest == "":
			// Held with no digest of its own: published before this existed,
			// and nothing to compare against is not a mismatch.
			return judgedTheCurrentDefinition, nil
		case ruling.Digest == "":
			return judgedNothing, nil
		case ruling.Digest != tool.Digest:
			return judgedSomethingElse, nil
		}
		return judgedTheCurrentDefinition, nil
	}
	// Not in the catalogue at all. The Curator may rule ahead of a server
	// connecting, which the catalogue's own Sync already allows.
	return judgedTheCurrentDefinition, nil
}
