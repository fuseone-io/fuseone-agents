package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/githubapp/githubapptest"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/memory"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/vault"
	"github.com/fuseone/agents/internal/workitem"
)

/*
The factory's Triage reference stays publishable.

factory/agents/triage.agent.md is copied into the Studio by a person, after
registering the role's GitHub server the way factory/agents/README.md says:
as a GitHub App, with X-MCP-Toolsets naming the sets the role needs. Nothing
else in CI holds the two together, and each half moves on its own: the
reference gains a tool, the hosted server renames one, the App mode drops the
header, the publish check changes what it compares. Any of those leaves a
reference nobody can publish, and the first to find out is the person
setting the factory up.

So this starts where that person starts — the registration route, with the
App and the header — and goes through the worker's real discovery against a
hosted server whose catalogue depends on the header, the Curator's rulings
through the route, and the publish route with the reference as the file says
it. The file is read from the repository, not copied here, so a change to it
is judged by this test the day it is made.
*/

const triageReference = "../../factory/agents/triage.agent.md"

// The hosted GitHub MCP server's sets, by the tool names the references use.
// Its default does not include projects; only the header adds it.
var hostedToolsets = map[string][]string{
	"issues":   {"issue_read", "list_issues", "search_issues", "add_issue_comment", "issue_write"},
	"projects": {"projects_list", "projects_get", "projects_write"},
	"repos": {"get_file_contents", "search_code", "get_commit",
		"create_branch", "create_or_update_file", "push_files"},
	"pull_requests": {"search_pull_requests", "pull_request_read", "create_pull_request"},
}

// hostedGitHub is the hosted server as far as this journey needs it: a live
// installation token or nothing, and the tools of the sets the request names.
type hostedGitHub struct {
	github *githubapptest.Fake

	handler http.Handler

	mu       sync.Mutex
	toolsets []string
	refused  int
}

func (h *hostedGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	h.mu.Lock()
	if !h.github.Valid(token) {
		h.refused++
		h.mu.Unlock()
		http.Error(w, "Bad credentials", http.StatusUnauthorized)
		return
	}
	h.toolsets = append(h.toolsets, r.Header.Get("X-MCP-Toolsets"))
	h.mu.Unlock()
	h.handler.ServeHTTP(w, r)
}

func toolsetServer(header string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "github", Version: "test"}, nil)
	sets := []string{"issues"}
	if header != "" {
		sets = strings.Split(header, ",")
	}
	for _, set := range sets {
		for _, name := range hostedToolsets[strings.TrimSpace(set)] {
			mcp.AddTool(server, &mcp.Tool{Name: name},
				func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
					return &mcp.CallToolResult{}, nil, nil
				})
		}
	}
	return server
}

// factoryJourney is one installation: the console's server and the worker's
// reconciler over the same database, as serve and worker are deployed.
type factoryJourney struct {
	server       string // the role's registration, github-<role>
	pool         *pgxpool.Pool
	api          *httpapi.Server
	worker       *reconciler
	catalog      *tools.Catalog
	hosted       *hostedGitHub
	curator      *admin.Curator
	integrations *admin.Integrations
	remote       string
}

func newFactoryJourney(t *testing.T, toolsets string) *factoryJourney {
	t.Helper()
	return newRoleJourney(t, "github-triage", toolsets)
}

// newRoleJourney is newFactoryJourney for the role registered as server.
func newRoleJourney(t *testing.T, server, toolsets string) *factoryJourney {
	t.Helper()
	pool := testdb.Pool(t)
	// The catalogue and its rulings are installation-wide and outlive a
	// journey — a ruling is kept on purpose when its tool leaves — so a
	// journey starts from an installation that has never discovered or ruled
	// on a tool, whatever an earlier run or another journey left behind.
	if _, err := pool.Exec(t.Context(),
		`delete from settings where kind in ('tool', 'tool_classification'); delete from tool_server_leases; delete from tool_server_grace`); err != nil {
		t.Fatalf("reset the catalogue: %v", err)
	}
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	curator := admin.NewCurator(pool)
	integrations := admin.NewIntegrations(pool, store)

	// The tool list serve hands the console, layer for layer.
	listed := workitem.NewToolList(memory.NewToolList(
		connectortools.NewToolList(curator, connectortools.NewSettings(store))))
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAdministration(curator, listed, integrations).
		WithAgents(spec.NewRegistry(pool)).
		WithPublisher(spec.NewPublisher(pool, engine.SystemClock{}))

	content := engine.NewMemoryContent()
	catalog := tools.NewCatalog(content)
	t.Cleanup(func() { _ = catalog.Close() })
	worker := newReconciler(catalog, integrations, healthOf(pool)).
		publishingTo(curator).classifyingWith(curator)

	// Stateless, so every request is judged by its own header, as the
	// hosted server judges it.
	hosted := &hostedGitHub{
		github: githubapptest.New(t, 5550564, 177087947),
		handler: mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			return toolsetServer(r.Header.Get("X-MCP-Toolsets"))
		}, &mcp.StreamableHTTPOptions{Stateless: true}),
	}
	remote := httptest.NewServer(hosted)
	t.Cleanup(remote.Close)

	j := &factoryJourney{
		server: server, pool: pool, api: api, worker: worker, catalog: catalog, hosted: hosted,
		curator: curator, integrations: integrations, remote: remote.URL + "/mcp/",
	}
	j.register(t, j.remote, toolsets)
	return j
}

// installer holds what setting the factory up takes: configuring servers and
// ruling on tools for the installation, and authoring in the factory area.
func installer(t *testing.T) context.Context {
	return auth.WithPrincipal(t.Context(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{
			{Scope: domain.Scope{Company: "default", Area: "platform"}, Role: domain.RoleCurator},
			{Scope: domain.Scope{Company: "acme", Area: "factory"}, Role: domain.RoleAuthor},
		},
	})
}

// triageToolsets is the header factory/agents/README.md gives the Triage
// registration.
const triageToolsets = "issues,projects"

// register is the administration form for the role's server: the App, and the
// toolsets beside it.
func (j *factoryJourney) register(t *testing.T, url, toolsets string) {
	t.Helper()
	github := j.hosted.github
	headers := map[string]string{"X-MCP-Toolsets": toolsets}
	resp, err := j.api.PutMCPServer(installer(t), openapi.PutMCPServerRequestObject{
		Name: j.server,
		Body: &openapi.PutMCPServerJSONRequestBody{
			Transport: ptrTo(openapi.TransportHttp), Url: &url, Headers: &headers,
			GithubApp: &openapi.MCPGitHubApp{
				AppId: &github.AppID, InstallationId: &github.InstallationID,
				PrivateKey: ptrTo(github.PKCS1()), ApiBaseURL: ptrTo(github.Base()),
			},
		},
	})
	if _, ok := resp.(openapi.PutMCPServer204Response); err != nil || !ok {
		t.Fatalf("register %s = %#v, %v; want 204", j.server, resp, err)
	}
}

// discover is one reconcile pass of the worker: connect, and publish the
// catalogue the console reads.
func (j *factoryJourney) discover(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	j.worker.reconcile(ctx)
	if !j.catalog.Connected(j.server) {
		t.Fatalf("%s did not connect; the publish below would prove nothing", j.server)
	}
}

// classify rules on every discovered tool the way README says: every read is
// untrusted, and the tools that change GitHub are writes.
func (j *factoryJourney) classify(t *testing.T) {
	t.Helper()
	resp, err := j.api.ListTools(installer(t), openapi.ListToolsRequestObject{})
	listed, ok := resp.(openapi.ListTools200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("ListTools = %#v, %v", resp, err)
	}
	writes := []string{"add_issue_comment", "issue_write", "projects_write",
		"create_branch", "create_or_update_file", "push_files", "create_pull_request"}
	for _, tool := range listed.Items {
		name, ours := strings.CutPrefix(tool.ToolId, j.server+".")
		if !ours {
			continue
		}
		body := &openapi.ClassifyToolJSONRequestBody{
			Effect: openapi.EffectRead, Untrusted: ptrTo(true), Digest: tool.Digest,
		}
		if slices.Contains(writes, name) {
			body = &openapi.ClassifyToolJSONRequestBody{Effect: openapi.EffectWrite, Digest: tool.Digest}
		}
		got, err := j.api.ClassifyTool(installer(t), openapi.ClassifyToolRequestObject{ToolId: tool.ToolId, Body: body})
		if _, ok := got.(openapi.ClassifyTool204Response); err != nil || !ok {
			t.Fatalf("classify %s = %#v, %v", tool.ToolId, got, err)
		}
	}
}

// publish copies the reference into the Studio's form, field for field, and
// sends it through the publish route.
func (j *factoryJourney) publish(t *testing.T, reference spec.Spec) openapi.PublishAgentResponseObject {
	t.Helper()
	resp, err := j.api.PublishAgent(installer(t), openapi.PublishAgentRequestObject{
		AgentId: string(reference.ID), Body: studioForm(reference),
	})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	return resp
}

func readTriageReference(t *testing.T) spec.Spec {
	t.Helper()
	return readReference(t, triageReference)
}

func readReference(t *testing.T, path string) spec.Spec {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the reference: %v", err)
	}
	reference, err := spec.Parse(path, raw)
	if err != nil {
		t.Fatalf("the reference does not parse: %v", err)
	}
	return reference
}

// studioForm is what a person fills in from the file. The company is the
// installation's, chosen in the console; the file has none by design.
func studioForm(s spec.Spec) *openapi.AgentDefinition {
	form := &openapi.AgentDefinition{
		Name: s.Name, Company: "acme", Area: string(s.Area),
		Provider: s.Provider, Model: s.Model, Effort: ptrTo(s.Effort),
		ReplyLocale: ptrTo(s.ReplyLocale), Instructions: s.Instructions,
		Budget: &openapi.Budget{
			Micros: &s.Budget.Micros, ToolCalls: &s.Budget.ToolCalls, Steps: &s.Budget.Steps,
		},
	}
	if s.StandingApprovals.Forbidden() {
		form.StandingApprovals = ptrTo(openapi.StandingPolicyForbidden)
	}
	toolNames := make([]string, len(s.Tools))
	for i, tool := range s.Tools {
		toolNames[i] = string(tool)
	}
	form.Tools = &toolNames
	var triggers []openapi.AgentTrigger
	for _, trigger := range s.Triggers {
		triggers = append(triggers, openapi.AgentTrigger{
			Type: openapi.AgentTriggerType(trigger.Type), Schedule: ptrTo(trigger.Schedule),
		})
	}
	form.Triggers = &triggers
	var steps []openapi.AgentStep
	for _, step := range s.Steps {
		reaches := make([]string, len(step.Reaches))
		for i, tool := range step.Reaches {
			reaches[i] = string(tool)
		}
		out := openapi.AgentStep{Name: step.Name, Reaches: &reaches}
		if step.StopsWhen != "" {
			out.StopsWhen = ptrTo(step.StopsWhen)
		}
		steps = append(steps, out)
	}
	form.Steps = &steps
	return form
}

func ptrTo[T any](v T) *T { return &v }

// differingFields names each Spec field that differs, with both values, so a
// failure says which line of the file the copy lost.
func differingFields(got, want spec.Spec) []string {
	g, w := reflect.ValueOf(got), reflect.ValueOf(want)
	var out []string
	for i := range g.NumField() {
		if !reflect.DeepEqual(g.Field(i).Interface(), w.Field(i).Interface()) {
			out = append(out, fmt.Sprintf("%s: got %#v, want %#v",
				g.Type().Field(i).Name, g.Field(i).Interface(), w.Field(i).Interface()))
		}
	}
	return out
}

// The reference publishes, against a server registered as the README says.
//
// Neither journey is parallel: the catalogue is installation-wide, and two in
// one database would each publish against the other's tools.
func TestFactoryTriage_referenceRegisteredAsDocumented_publishesFromTheStudio(t *testing.T) {
	j := newFactoryJourney(t, triageToolsets)
	j.discover(t)
	j.classify(t)

	assertPublishesAsTheReference(t, j, readTriageReference(t), triageToolsets)
}

// assertPublishesAsTheReference publishes reference through the Studio's form,
// and requires what was published to be the reference, judged against the
// role's server reached as its App with the role's toolsets.
func assertPublishesAsTheReference(t *testing.T, j *factoryJourney, reference spec.Spec, toolsets string) {
	t.Helper()
	resp := j.publish(t, reference)

	published, ok := resp.(openapi.PublishAgent200JSONResponse)
	if refused, was := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); was && refused.Detail != nil {
		t.Fatalf("publishing the %s reference was refused: %s", reference.ID, *refused.Detail)
	}
	if !ok {
		t.Fatalf("publishing the %s reference answered %#v; want it published", reference.ID, resp)
	}
	// The form carried the whole file: what was published is the reference.
	got, err := spec.NewRegistry(j.pool).Get(t.Context(), reference.ID, domain.VersionID(published.VersionId))
	if err != nil {
		t.Fatalf("read the published version: %v", err)
	}
	// Only what publishing assigns differs, each for a reason that is not the
	// file's: the company is chosen in the console (the file has none by
	// design), the version is the digest of the text the route renders, not
	// of the file's bytes, and the source names where each copy was read.
	want := reference
	want.Company, want.Version, want.Source = got.Company, got.Version, got.Source
	// No events declared reads back as an empty value where the parse left
	// nil. Both say "emits nothing"; anything declared is still compared.
	if len(got.Emits) == 0 && len(want.Emits) == 0 {
		want.Emits = got.Emits
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("published version differs from the reference in %v", differingFields(got, want))
	}
	// And the server it was judged against was reached as the App, with the
	// role's toolsets.
	j.hosted.mu.Lock()
	defer j.hosted.mu.Unlock()
	if j.hosted.refused != 0 || len(j.hosted.toolsets) == 0 || j.hosted.github.Minted() == 0 {
		t.Fatalf("refused %d, served %d, minted %d; want discovery authorised by an installation token",
			j.hosted.refused, len(j.hosted.toolsets), j.hosted.github.Minted())
	}
	for _, header := range j.hosted.toolsets {
		if header != toolsets {
			t.Fatalf("X-MCP-Toolsets = %q on a request, want the registration's", header)
		}
	}
}

// Without projects in the header the hosted server offers no projects_*
// tools, and the publish says which ones it is missing — the failure the
// README warns of, so a person setting the factory up can tell it apart.
//
// The same refusal after narrowing a registration that once had projects is
// TestCatalogue_toolsetsNarrowedAfterADiscovery_stopOfferingTheDroppedTools.
func TestFactoryTriage_toolsetsWithoutProjects_publishNamesTheMissingProjectTools(t *testing.T) {
	j := newFactoryJourney(t, "issues")
	j.discover(t)
	j.classify(t)

	resp := j.publish(t, readTriageReference(t))

	refused, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !ok || refused.Detail == nil {
		t.Fatalf("publish answered %#v; want it refused for the missing tools", resp)
	}
	for _, missing := range []string{"projects_list", "projects_get", "projects_write"} {
		if !strings.Contains(*refused.Detail, "github-triage."+missing) {
			t.Errorf("refusal %q does not name github-triage.%s", *refused.Detail, missing)
		}
	}
	if strings.Contains(*refused.Detail, "issue_read") {
		t.Errorf("refusal %q names an issues tool the server does offer", *refused.Detail)
	}
}
