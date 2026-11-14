package httpapi

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// listToolsReach is readerReach for ListTools, with its refusals: 400 for a
// target that is not a real scope, 404 for one the caller may not read. Asked
// for what the caller may classify, it is classifiableReach instead.
func listToolsReach(ctx context.Context, params openapi.ListToolsParams) (reach, openapi.ListToolsResponseObject) {
	if params.Classifiable != nil && *params.Classifiable {
		return classifiableReach(ctx, params.Scope != nil)
	}
	raw := params.Scope
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

/*
classifiableReach keeps the tools the caller may rule on as the owner
(NT-019): the server's scope is a company or an area that a scope where the
caller holds tool:classify contains, as ClassifyTool's owner door asks it.
The platform's curator governs every tool. The installation's tools, and a
tool whose server no row places, are the platform's alone.

A target beside it is refused rather than combined: a target answers what an
agent stored there can run, which is the other direction.
*/
func classifiableReach(ctx context.Context, withTarget bool) (reach, openapi.ListToolsResponseObject) {
	if withTarget {
		return nil, openapi.ListTools400ApplicationProblemPlusJSONResponse{
			BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
				invalid("classifiable and scope are two answers: ask for one")),
		}
	}
	p, _ := auth.PrincipalFrom(ctx)
	if !p.CanAnywhere(domain.PermToolClassify) {
		return nil, openapi.ListTools403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: forbidden(domain.PermToolClassify, domain.Scope{}),
		}
	}
	if p.Can(domain.PermToolClassify, adminScope) {
		return func(domain.Scope) bool { return true }, nil
	}
	return func(server domain.Scope) bool {
		owned := server.Company != "" && server.Company != domain.Installation
		return owned && p.Can(domain.PermToolClassify, server)
	}, nil
}

// ownersMayRule leaves out what no owner's ruling is written for: a native
// tool, such as a connector instance's, belongs to no stored MCP server, so
// the owner's door refuses it. The platform's administration keeps every tool.
func ownersMayRule(ctx context.Context, entries []domain.ToolEntry) []domain.ToolEntry {
	p, _ := auth.PrincipalFrom(ctx)
	if p.Can(domain.PermToolClassify, adminScope) {
		return entries
	}
	out := make([]domain.ToolEntry, 0, len(entries))
	for _, e := range entries {
		if !e.Native {
			out = append(out, e)
		}
	}
	return out
}
