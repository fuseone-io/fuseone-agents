package httpapi

import (
	"context"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/known"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The door a ruling arrives at, and the two ways it is refused.

The catalogue enforces this properly on its own — a ruling only reaches a tool
whose definition it names. But the catalogue is reached through here, and this
is the surface an old console, a script or a curl actually calls. A rule the
store keeps and the door does not is a rule that holds until somebody stops
using the screen.
*/

// judging is a Curator that records what it was asked to store, over a
// catalogue holding one tool with a known definition.
type judging struct {
	stored domain.ToolClassification
	tools  []domain.ToolEntry
}

func (j *judging) Classify(_ context.Context, _ domain.Scope, r domain.ToolClassification) error {
	j.stored = r
	return nil
}

func (j *judging) List(context.Context, domain.Scope) ([]domain.ToolClassification, error) {
	return nil, nil
}

func (j *judging) Events(context.Context, string, string, int) ([]domain.AdminEvent, string, error) {
	return nil, "", nil
}

func (j *judging) Tools(context.Context) ([]domain.ToolEntry, error) { return j.tools, nil }

func judgingOne(t *testing.T, digest string) (*Server, *judging) {
	t.Helper()
	one := &judging{tools: []domain.ToolEntry{{
		ID: "crm.lookup", Server: "crm", Description: "Look a customer up",
		Digest: digest,
	}}}
	return NewServer(ledger.NewMemory(), "test").WithAdministration(one, one, nil), one
}

func ruling(digest *string) openapi.ClassifyToolRequestObject {
	return openapi.ClassifyToolRequestObject{
		ToolId: "crm.lookup",
		Body:   &openapi.ClassifyToolJSONRequestBody{Effect: "read", Digest: digest},
	}
}

/*
A ruling that names no definition, for a tool whose definition we hold.

An empty digest is how a ruling recorded before any of this existed keeps
working, so it cannot also mean "I did not check". Accepted here, any caller
that omits the field is back to classification by name — which is the model
this replaced, reachable by leaving a field out.
*/
func TestClassifyTool_withNoDigestForAToolWeKnow_isRefused(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "sha-current")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(nil))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, refused := resp.(openapi.ClassifyTool400ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 400", resp)
	}
	if curator.stored.Tool != "" {
		t.Error("the ruling was stored anyway; the check is a message, not a control")
	}
}

// A ruling about the definition that was read is recorded, or the check is an
// outage rather than a control.
func TestClassifyTool_namingTheDefinitionOnOffer_isRecorded(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "sha-current")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(ptr("sha-current")))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, ok := resp.(openapi.ClassifyTool204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if curator.stored.Digest != "sha-current" {
		t.Errorf("stored digest = %q, want the definition that was judged", curator.stored.Digest)
	}
}

/*
A ruling about a definition the server has since replaced.

Refused rather than stored against what is there now: the Curator read a
description and a schema, and recording their judgement over a different one
would put their name on a decision they never made.
*/
func TestClassifyTool_namingADefinitionAlreadyReplaced_isRefused(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "sha-current")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(ptr("sha-from-this-morning")))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, refused := resp.(openapi.ClassifyTool409ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 409", resp)
	}
	if curator.stored.Tool != "" {
		t.Error("stored anyway; somebody's name is now on a judgement of another definition")
	}
}

/*
A tool the catalogue holds without a digest of its own.

Published before this existed and not rediscovered since. Nothing to compare
against is not a mismatch, and refusing here would make an upgrade look like a
platform that had forgotten every tool it has.
*/
func TestClassifyTool_forAToolPublishedBeforeDigests_isRecorded(t *testing.T) {
	t.Parallel()
	server, curator := judgingOne(t, "")

	resp, err := server.ClassifyTool(as(domain.RoleCurator), ruling(nil))
	if err != nil {
		t.Fatalf("ClassifyTool: %v", err)
	}
	if _, ok := resp.(openapi.ClassifyTool204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if curator.stored.Effect != domain.EffectRead {
		t.Errorf("stored = %+v, want the ruling recorded", curator.stored)
	}
}

/*
What the console is told about the servers this platform has read about.

A recipe fills a form and decides nothing. What it must carry is enough for a
reader to judge it themselves: who publishes the server, whose page this was
read from, and whether anybody ran it. Shipped without those, a suggestion is
an anonymous opinion with the platform's name on it.
*/
func TestListRecipes_carryWhoPublishesAndWhoseDocumentationWasRead(t *testing.T) {
	t.Parallel()
	shipped, err := known.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	server := NewServer(ledger.NewMemory(), "test").WithKnown(shipped)

	resp, err := server.ListRecipes(as(domain.RoleCurator), openapi.ListRecipesRequestObject{})
	if err != nil {
		t.Fatalf("ListRecipes: %v", err)
	}
	listed, ok := resp.(openapi.ListRecipes200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the listing", resp)
	}
	if len(listed.Items) == 0 {
		t.Fatal("no recipes; the screen would offer nothing to start from")
	}
	for _, recipe := range listed.Items {
		if recipe.Publisher == "" || recipe.DocsFrom == "" {
			t.Errorf("%s ships without saying who publishes it or whose page was read", recipe.Server)
		}
		if recipe.Status == "" {
			t.Errorf("%s ships without a recipe status", recipe.Server)
		}
		if recipe.ConfigRequirements == nil {
			t.Errorf("%s ships with nil configuration requirements", recipe.Server)
		}
		if slices.Contains(recipe.ConfigRequirements, openapi.ServerRecipeConfigRequirementsCredential) && (recipe.AuthModes == nil || len(*recipe.AuthModes) == 0) {
			t.Errorf("%s asks for a credential without telling the console what kind", recipe.Server)
		}
	}

	datadog, ok := recipeNamed(listed.Items, "datadog")
	if !ok {
		t.Fatal("Datadog recipe missing")
	}
	if datadog.RequiresPersonalCredential {
		t.Fatal("Datadog recipe says every credential is personal; service credentials are documented")
	}
	if mode, ok := authMode(*datadog.AuthModes, openapi.ServerRecipeAuthModeTypeHeaders); !ok || mode.Headers == nil ||
		!slices.Equal(*mode.Headers, []string{"DD_API_KEY", "DD_APPLICATION_KEY"}) {
		t.Fatalf("Datadog headers auth = %+v, want both header names delivered to the console", mode)
	}
	outline, ok := recipeNamed(listed.Items, "outline")
	if !ok {
		t.Fatal("Outline recipe missing")
	}
	if !outline.RequiresPersonalCredential {
		t.Fatal("Outline recipe does not say tools require a personal credential")
	}
	postgres, ok := recipeNamed(listed.Items, "postgres")
	if !ok {
		t.Fatal("PostgreSQL recipe missing")
	}
	if mode, ok := authMode(*postgres.AuthModes, openapi.ServerRecipeAuthModeTypeDsn); !ok || mode.Env == nil ||
		*mode.Env != "DATABASE_URL" {
		t.Fatalf("PostgreSQL DSN auth = %+v, want the env variable delivered to the console", mode)
	}
}

func recipeNamed(in []openapi.ServerRecipe, name string) (openapi.ServerRecipe, bool) {
	for _, one := range in {
		if one.Server == name {
			return one, true
		}
	}
	return openapi.ServerRecipe{}, false
}

func authMode(
	in []openapi.ServerRecipeAuthMode,
	typ openapi.ServerRecipeAuthModeType,
) (openapi.ServerRecipeAuthMode, bool) {
	for _, one := range in {
		if one.Type == typ {
			return one, true
		}
	}
	return openapi.ServerRecipeAuthMode{}, false
}

// An installation shipping none is a real mode, and an empty list is the
// honest answer to "what do you know".
func TestListRecipes_withNothingShipped_isAnEmptyListAndNotAFailure(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").
		ListRecipes(as(domain.RoleCurator), openapi.ListRecipesRequestObject{})
	if err != nil {
		t.Fatalf("ListRecipes: %v", err)
	}
	if listed, ok := resp.(openapi.ListRecipes200JSONResponse); !ok || listed.Items == nil {
		t.Fatalf("response = %T, want an empty listing", resp)
	}
}

// An auditor reads everything and changes nothing, and a recipe is a step
// towards a connection: the same permission that governs the act governs the
// screen that starts it.
func TestListRecipes_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").
		ListRecipes(as(domain.RoleAuditor), openapi.ListRecipesRequestObject{})
	if err != nil {
		t.Fatalf("ListRecipes: %v", err)
	}
	if _, refused := resp.(openapi.ListRecipes403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

/*
What the console is told about a server it has already narrowed.

The surface is stored and the read model did not carry it, so a server somebody
had cut down to three tools read back as all-in — and the page, which starts
from what it is told, would have offered to save the wider list. The screen
disagreeing with the server, in the direction that widens.

An empty surface is the case worth naming: "chosen, and none of it" has to
survive the round trip as itself and not as "nobody has chosen".
*/
func TestListIntegrations_carriesTheSurfaceThatWasChosen(t *testing.T) {
	t.Parallel()
	none := []string{}
	admin := &fakeAdmin{servers: []domain.MCPServer{
		{Name: "narrowed", Transport: domain.TransportHTTP, URL: "https://x/mcp",
			Enabled: true, Surface: &[]string{"lookup"}},
		{Name: "emptied", Transport: domain.TransportHTTP, URL: "https://y/mcp",
			Enabled: true, Surface: &none},
		{Name: "unchosen", Transport: domain.TransportHTTP, URL: "https://z/mcp",
			Enabled: true},
	}}

	resp, err := serverWith(t, admin).ListIntegrations(as(domain.RoleCurator),
		openapi.ListIntegrationsRequestObject{})
	if err != nil {
		t.Fatalf("ListIntegrations: %v", err)
	}
	listed, ok := resp.(openapi.ListIntegrations200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}

	by := map[string]openapi.MCPServer{}
	for _, s := range listed.McpServers {
		by[s.Name] = s
	}
	if got := by["narrowed"].Surface; got == nil || len(*got) != 1 {
		t.Errorf("narrowed surface = %v, want the one tool that was chosen", got)
	}
	if got := by["emptied"].Surface; got == nil || len(*got) != 0 {
		t.Errorf("emptied surface = %v, want a chosen-and-empty surface", got)
	}
	if got := by["unchosen"].Surface; got != nil {
		t.Errorf("unchosen surface = %v, want absent to stay absent", got)
	}
}

/*
The warning asks about the areas the caller can see, not about the
administration area.

It used to ask for agents in `adminScope` — one company and one area, the
platform's own — so an installation whose agents live in `acme/cx` was told
nobody would be affected by removing anything. A warning that undercounts is
worse than none: it is read as an all-clear.

Scoped to the caller rather than read wholesale, because the answer names
agents. Somebody who may classify tools in one area has no business learning
what runs in another.
*/
func TestListTools_theImpactWarning_asksAboutTheCallersScopesNotTheAdminArea(t *testing.T) {
	t.Parallel()
	one := &judging{tools: []domain.ToolEntry{
		{ID: "crm.lookup", Server: "crm", OnSurface: true},
	}}
	agents := &agentsAsked{listed: []domain.AgentSummary{
		{ID: "triagem", Tools: []domain.ToolID{"crm.lookup"}},
	}}
	server := NewServer(ledger.NewMemory(), "test").
		WithAdministration(one, one, nil).
		WithAgents(agents)

	// An installation-wide curator: the grant contains every scope, which is
	// what the person who classifies tools actually holds.
	resp, err := server.ListTools(installationWide(domain.RoleCurator),
		openapi.ListToolsRequestObject{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	listed, ok := resp.(openapi.ListTools200JSONResponse)
	if !ok {
		t.Fatalf("response = %T", resp)
	}

	if slices.Contains(agents.asked, adminScope) {
		t.Error("asked about the administration area, where an installation keeps no agents")
	}
	// And not about the scope above every company as though it were one: that
	// filters for a company literally named "installation" and finds nobody,
	// which looks exactly like the bug it replaced.
	if slices.Contains(agents.asked, domain.Scope{Company: domain.Installation}) {
		t.Error("asked about the installation scope as though it were a company")
	}
	named := listed.Items[0].DeclaredBy
	if named == nil || len(*named) != 1 || (*named)[0] != "triagem" {
		t.Errorf("declaredBy = %v, want the agent that would stop", named)
	}
}

func installationWide(role domain.Role) context.Context {
	return auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: domain.Installation}, Role: role}},
	})
}

// agentsAsked records which scopes it was asked about, which is the thing that
// was wrong rather than the answer it gave.
type agentsAsked struct {
	listed []domain.AgentSummary
	asked  []domain.Scope
}

func (a *agentsAsked) List(_ context.Context, scope domain.Scope, _ bool) ([]domain.AgentSummary, error) {
	a.asked = append(a.asked, scope)
	return a.listed, nil
}

func (a *agentsAsked) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	return nil, nil
}

func (a *agentsAsked) Instructions(context.Context, domain.AgentID, domain.VersionID) (string, string, error) {
	return "", "", nil
}
