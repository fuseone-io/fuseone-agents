package httpapi

import (
	"context"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/known"
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

	/*
		Where the caller may read agents, not where the administration area
		lives.

		`adminScope` is one company and one area — the platform's own — so
		asking for agents there answered with almost nothing: an installation's
		real agents live in `acme/cx` and the warning quietly said nobody would
		be affected. A warning that undercounts is worse than none, because it
		is read as an all-clear.

		Scoped to the caller rather than read wholesale, because the answer
		names agents. Somebody who may classify tools in one area has no
		business learning what runs in another.
	*/
	/*
		An installation-wide grant is not a scope to filter by.

		`{Company: Installation}` is the scope above every company, and passed
		to the listing as though it were one it filters for a company literally
		named "installation" and matches nothing. The unfiltered read is what
		it means, and getting this wrong looks exactly like getting the old
		version right: an empty warning either way.
	*/
	visible := auth.VisibleScopes(ctx, domain.PermAgentRead)
	for _, scope := range visible {
		if scope.Company == domain.Installation {
			visible = []domain.Scope{{}}
			break
		}
	}

	var (
		out  = map[domain.ToolID][]string{}
		seen = map[domain.AgentID]bool{}
	)
	for _, scope := range visible {
		current, err := s.agents.List(ctx, scope, false)
		if err != nil {
			return nil, err
		}
		for _, agent := range current {
			// A grant on a company and one on an area inside it both answer,
			// and an agent counted twice would read as two agents stopping.
			if seen[agent.ID] {
				continue
			}
			seen[agent.ID] = true
			for _, tool := range agent.Tools {
				out[tool] = append(out[tool], string(agent.ID))
			}
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
			AuthModes:          authModes(e.AuthModes),
			Category:           e.Category,
			ConfigRequirements: configRequirements(e.Config),
			DocsFrom:           openapi.ServerRecipeDocsFrom(e.DocsFrom),
			Provenance:         openapi.ServerRecipeProvenance(e.Provenance),
			Status:             openapi.ServerRecipeStatus(e.Status),
			Suggestions:        ptr(len(e.Suggestions)),
		}
		if e.Docs != "" {
			recipe.Docs = ptr(e.Docs)
		}
		if e.Auth != "" {
			recipe.Auth = ptr(e.Auth)
		}
		if e.Note != "" {
			recipe.Note = ptr(e.Note)
		}
		if e.Command != "" {
			recipe.Command = ptr(e.Command)
		}
		if e.URL != "" {
			recipe.Url = ptr(e.URL)
		}
		if e.Transport != "" {
			recipe.Transport = ptr(openapi.Transport(e.Transport))
		}
		if e.ProtocolMode != "" {
			recipe.ProtocolMode = ptr(openapi.MCPProtocolMode(e.ProtocolMode))
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

func authModes(in []known.AuthMode) *[]openapi.ServerRecipeAuthMode {
	if len(in) == 0 {
		return nil
	}
	out := make([]openapi.ServerRecipeAuthMode, 0, len(in))
	for _, one := range in {
		mode := openapi.ServerRecipeAuthMode{
			Type:      openapi.ServerRecipeAuthModeType(one.Type),
			Principal: openapi.ServerRecipeAuthModePrincipal(one.Principal),
		}
		if one.Header != "" {
			mode.Header = ptr(one.Header)
		}
		if len(one.Headers) > 0 {
			mode.Headers = ptr(one.Headers)
		}
		if one.Env != "" {
			mode.Env = ptr(one.Env)
		}
		if one.Label != "" {
			mode.Label = ptr(one.Label)
		}
		if one.Note != "" {
			mode.Note = ptr(one.Note)
		}
		if one.Prefix != "" {
			mode.Prefix = ptr(one.Prefix)
		}
		if len(one.Scopes) > 0 {
			mode.Scopes = ptr(one.Scopes)
		}
		out = append(out, mode)
	}
	return &out
}

func configRequirements(in []known.ConfigRequirement) []openapi.ServerRecipeConfigRequirements {
	out := make([]openapi.ServerRecipeConfigRequirements, 0, len(in))
	for _, one := range in {
		out = append(out, openapi.ServerRecipeConfigRequirements(one))
	}
	return out
}
