package httpapi

import (
	"context"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
declaringTools maps each tool to the agents whose current version names it.

Current versions, because an older one that names a tool is pinned to runs
already recorded and cannot be started again — counting it would warn somebody
about an agent they replaced last month. The same listing the console reads
elsewhere, so the two cannot disagree about which version would run.
*/
func (s *Server) declaringTools(ctx context.Context) (map[domain.ToolID][]string, error) {
	if s.agents == nil {
		return nil, nil
	}
	current, err := s.agents.List(ctx, adminScope, false)
	if err != nil {
		return nil, err
	}
	out := map[domain.ToolID][]string{}
	for _, agent := range current {
		for _, tool := range agent.Tools {
			out[tool] = append(out[tool], string(agent.ID))
		}
	}
	return out, nil
}

/*
ListRecipes answers what the platform has read about servers other people
publish.

Identity, a link, how it is usually reached and the credential it expects. It
fills a form and decides nothing: not a supported connector, not a hosted
service, and not an endorsement — this platform did not write these servers.
*/
func (s *Server) ListRecipes(ctx context.Context, _ openapi.ListRecipesRequestObject) (openapi.ListRecipesResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermToolClassify); resp != nil {
		return openapi.ListRecipes403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.known == nil {
		// An installation shipping no recipes is a real mode, and an empty
		// list is the honest answer to "what do you know".
		return openapi.ListRecipes200JSONResponse{Items: []openapi.ServerRecipe{}}, nil
	}

	entries := s.known.All()
	items := make([]openapi.ServerRecipe, 0, len(entries))
	for _, e := range entries {
		recipe := openapi.ServerRecipe{
			Server: e.Server, Title: e.Title, Publisher: e.Publisher,
			DocsFrom:    openapi.ServerRecipeDocsFrom(e.DocsFrom),
			Provenance:  openapi.ServerRecipeProvenance(e.Provenance),
			Suggestions: ptr(len(e.Suggestions)),
		}
		for value, into := range map[string]**string{
			e.Docs: &recipe.Docs, e.Auth: &recipe.Auth,
			e.Note: &recipe.Note, e.Command: &recipe.Command, e.URL: &recipe.Url,
		} {
			if value != "" {
				*into = ptr(value)
			}
		}
		if e.Transport != "" {
			recipe.Transport = ptr(openapi.Transport(e.Transport))
		}
		if len(e.Args) > 0 {
			recipe.Args = ptr(e.Args)
		}
		items = append(items, recipe)
	}
	slices.SortFunc(items, func(a, b openapi.ServerRecipe) int {
		return strings.Compare(a.Title, b.Title)
	})
	return openapi.ListRecipes200JSONResponse{Items: items}, nil
}
