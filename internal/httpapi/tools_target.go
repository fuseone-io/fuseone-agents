package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// listToolsReach is readerReach for ListTools, with its refusals: 400 for a
// target that is not a real scope, 404 for one the caller may not read.
func listToolsReach(ctx context.Context, raw *string) (reach, openapi.ListToolsResponseObject) {
	keep, err := readerReach(ctx, raw)
	switch {
	case errors.Is(err, errTargetHidden):
		return nil, openapi.ListTools404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: notFound("scope " + *raw),
		}
	case err != nil:
		return nil, openapi.ListTools400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid(err.Error())),
		}
	}
	return keep, nil
}

// toolsInReach keeps the tools whose declaring server or instance a run in
// reach can call: the entry's runtime scope, as the Gate's checkScope reads it.
func toolsInReach(entries []domain.ToolEntry, keep reach) []domain.ToolEntry {
	out := make([]domain.ToolEntry, 0, len(entries))
	for _, e := range entries {
		if keep(e.Scope) {
			out = append(out, e)
		}
	}
	return out
}
