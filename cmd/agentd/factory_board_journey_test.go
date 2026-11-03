package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/githubapp/githubapptest"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
)

/*
The factory's board tools, registered as factory/agents/README.md says (#213).

Each role reaches two servers with one GitHub App: github-<role>, the hosted
GitHub server, whose surface keeps projects_write out, and factory-<role>,
the factory's own server on the worker's loopback, which reads the role from
the App's token and serves move_card, set_card_fields or record_qa_verdict.
The factory server is a stub here, in the pattern of mcp_github_app_test.go:
it accepts only a token the fake GitHub minted, and lists its role's row.
agentd's tests do not import factory/guard, and the guard's own tests cover
what it does with the token.
*/

// boardSurfaces is each role's row of the factory server's table
// (factory/mcp/README.md § Roles and their surfaces).
var boardSurfaces = map[string][]string{
	"triage":    {"move_card", "set_card_fields"},
	"architect": {"move_card", "set_card_fields"},
	"developer": {"move_card"},
	"reviewer":  {"move_card"},
	"qa":        {"move_card", "record_qa_verdict"},
}

// roleToolsets is the X-MCP-Toolsets factory/agents/README.md gives each
// role's GitHub registration.
var roleToolsets = map[string]string{
	"triage":    triageToolsets,
	"architect": architectToolsets,
	"developer": developerToolsets,
	"reviewer":  "pull_requests,repos,issues,projects",
	"qa":        "pull_requests,repos,issues,projects",
}

var roleReferences = map[string]string{
	"triage":    triageReference,
	"architect": architectReference,
	"developer": developerReference,
	"reviewer":  "../../factory/agents/reviewer.agent.md",
	"qa":        "../../factory/agents/qa.agent.md",
}

// boardStub is the factory server as agentd sees it: a live installation
// token or nothing, then its role's tools.
type boardStub struct {
	server  string
	github  *githubapptest.Fake
	handler http.Handler

	mu      sync.Mutex
	served  int
	refused int
}

func (b *boardStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	b.mu.Lock()
	if !b.github.Valid(token) {
		b.refused++
		b.mu.Unlock()
		http.Error(w, "Bad credentials", http.StatusUnauthorized)
		return
	}
	b.served++
	b.mu.Unlock()
	b.handler.ServeHTTP(w, r)
}

// assertReachedAsTheApp requires every request the stub saw to carry a token
// the fake GitHub minted for the role's App.
func (b *boardStub) assertReachedAsTheApp(t *testing.T) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.refused != 0 || b.served == 0 || b.github.Minted() == 0 {
		t.Fatalf("%s: refused %d, served %d, minted %d; want discovery authorised by the App's installation token",
			b.server, b.refused, b.served, b.github.Minted())
	}
}

func newBoardStub(t *testing.T, role string, github *githubapptest.Fake) (*boardStub, string) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "factory-mcp", Version: "test"}, nil)
	for _, name := range boardSurfaces[role] {
		mcp.AddTool(server, &mcp.Tool{Name: name},
			func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{}, nil, nil
			})
	}
	stub := &boardStub{
		server: "factory-" + role, github: github,
		handler: mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
			&mcp.StreamableHTTPOptions{Stateless: true}),
	}
	remote := httptest.NewServer(stub)
	t.Cleanup(remote.Close)
	// httptest listens on 127.0.0.1: plain http beside an App, as the
	// sidecar's http://127.0.0.1:8091/ is.
	return stub, remote.URL + "/"
}

// referenceGitHubTools is every tool reference declares on github-<role>: the
// surface the setup texts give that server (internal/spec's
// factory_surface_test.go holds the texts to it).
func referenceGitHubTools(reference spec.Spec, role string) []string {
	var out []string
	for _, tool := range reference.Tools {
		if name, ok := strings.CutPrefix(string(tool), "github-"+role+"."); ok {
			out = append(out, name)
		}
	}
	return out
}

// newDocumentedJourney registers role as the setup texts say: github-<role>
// with its App, its toolsets and a surface of exactly its reference's GitHub
// tools; factory-<role> on loopback with the same App and nothing else.
func newDocumentedJourney(t *testing.T, role string) *factoryJourney {
	t.Helper()
	j := newInstallation(t, "github-"+role)
	surface := referenceGitHubTools(readReference(t, roleReferences[role]), role)
	j.registerAs(t, j.server, j.remote, map[string]string{"X-MCP-Toolsets": roleToolsets[role]}, &surface)
	j.registerBoard(t, role)
	return j
}

func (j *factoryJourney) registerBoard(t *testing.T, role string) {
	t.Helper()
	stub, url := newBoardStub(t, role, j.hosted.github)
	j.board = stub
	j.registerAs(t, stub.server, url, nil, nil)
}

// The Triage reference publishes with no projects_write anywhere in its
// pack, against both its servers as documented, and nothing untrusted it
// reads can reach an effect that is not reversible.
func TestFactoryBoard_triageRegisteredAsDocumented_publishesWithoutProjectsWrite(t *testing.T) {
	j := newDocumentedJourney(t, "triage")
	j.discover(t)
	j.classify(t)
	reference := readTriageReference(t)

	assertPublishesAsTheReference(t, j, reference, triageToolsets)

	for _, tool := range reference.Tools {
		if strings.HasSuffix(string(tool), ".projects_write") {
			t.Errorf("the published pack holds %s", tool)
		}
	}
	for _, want := range []string{"factory-triage.move_card", "factory-triage.set_card_fields"} {
		if !slices.Contains(reference.Tools, domain.ToolID(want)) {
			t.Errorf("the published pack lacks %s", want)
		}
	}
	// The hosted server still offers it: the surface is what keeps it out.
	if tool, listed := j.listed(t)["github-triage.projects_write"]; !listed || tool.OnSurface == nil || *tool.OnSurface {
		t.Errorf("github-triage.projects_write reads %+v (listed %v); want it discovered and off the surface", tool, listed)
	}
	assertNoBlockingFlow(t, j, reference)
}

// assertNoBlockingFlow runs the console's data-flow check on the reference:
// every tool ruled, and no path from untrusted data to an effect that cannot
// be undone.
func assertNoBlockingFlow(t *testing.T, j *factoryJourney, reference spec.Spec) {
	t.Helper()
	resp, err := j.api.CheckDataFlow(installer(t), openapi.CheckDataFlowRequestObject{
		AgentId: string(reference.ID), Body: studioForm(reference),
	})
	found, ok := resp.(openapi.CheckDataFlow200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("CheckDataFlow = %#v, %v", resp, err)
	}
	if len(found.Unclassified) != 0 {
		t.Errorf("unclassified tools in the pack: %v", found.Unclassified)
	}
	// The anchor: the reads are untrusted and move_card is a write, so the
	// check must find that path; finding none would mean it judged nothing.
	if len(found.Paths) == 0 {
		t.Fatal("the flow check found no path at all; it did not see the pack")
	}
	for _, path := range found.Paths {
		effect, err := domain.ParseEffect(string(path.Effect))
		if err != nil || !effect.Reversible() {
			t.Errorf("untrusted data reaches %s (%s) from %s: a blocking path", path.To, path.Effect, path.From)
		}
	}
}

// Narrowed at the hosted server's end instead: X-MCP-Tools naming the
// role's GitHub tools, with no X-MCP-Toolsets, lists only those, so
// projects_write is never discovered at all. The fake follows the upstream
// handler's rule (hostedTools); the platform's part is sending the header
// beside the App on every request.
func TestFactoryBoard_xMCPToolsWithoutToolsets_neverDiscoversProjectsWrite(t *testing.T) {
	j := newInstallation(t, "github-triage")
	reference := readTriageReference(t)
	tools := strings.Join(referenceGitHubTools(reference, "triage"), ",")
	j.registerAs(t, j.server, j.remote, map[string]string{"X-MCP-Tools": tools}, nil)
	j.registerBoard(t, "triage")
	j.discover(t)
	j.classify(t)

	if _, listed := j.listed(t)["github-triage.projects_write"]; listed {
		t.Error("github-triage.projects_write was discovered; X-MCP-Tools alone should list only the tools it names")
	}
	assertPublishesAsTheReference(t, j, reference, "")
	j.hosted.mu.Lock()
	defer j.hosted.mu.Unlock()
	for _, header := range j.hosted.tools {
		if header != tools {
			t.Fatalf("X-MCP-Tools = %q on a request, want the registration's %q", header, tools)
		}
	}
}

// A reference that still names projects_write is refused at publish under
// the documented setup, though the hosted server offers the tool: off the
// surface it does not exist for agents.
func TestFactoryBoard_aReferenceNamingProjectsWrite_isRefusedOffTheSurface(t *testing.T) {
	j := newDocumentedJourney(t, "triage")
	j.discover(t)
	j.classify(t)
	reference := readTriageReference(t)
	reference.Tools = append(reference.Tools, "github-triage.projects_write")

	refused, ok := j.publish(t, reference).(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !ok || refused.Detail == nil || !strings.Contains(*refused.Detail, "github-triage.projects_write") {
		t.Fatalf("publish answered %#v; want it refused for github-triage.projects_write", refused)
	}
}

// The Reviewer and QA, which had no journey of their own, publish the same
// way; QA holds record_qa_verdict and no pull_request_review_write.
func TestFactoryBoard_reviewerAndQARegisteredAsDocumented_publishFromTheStudio(t *testing.T) {
	for _, role := range []string{"reviewer", "qa"} {
		t.Run(role, func(t *testing.T) {
			j := newDocumentedJourney(t, role)
			j.discover(t)
			j.classify(t)

			assertPublishesAsTheReference(t, j, readReference(t, roleReferences[role]), roleToolsets[role])
		})
	}
}
