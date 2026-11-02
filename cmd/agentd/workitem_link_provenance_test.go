package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/gate"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/workitem"
)

/*
Who answered a create_pull_request decides whether it proves a link (#239,
review rounds 1 and 2), through the real catalogue.

Two trusts, kept apart. The answer's content is untrusted — the default the
Curator leaves on create_pull_request — and goes on tainting the run, so every
later write still waits for a person. What proves the link is not that
content: it is the trusted proof the engine derives from it, the stamp
{ref, via}, whose trust comes from the registration's provenance (the hosted
GitHub server reached as an App) and the strict parser. Another server, a
personal token or a look-alike host establishes no provenance: no stamp, and
the link parks.
*/

const prAddress = `{"id":"9","url":"https://github.com/fuseone-io/fuseone-agents/pull/240"}`

var (
	hostedApp = domain.MCPCredentials{GitHubApp: &domain.MCPGitHubApp{AppID: 7, InstallationID: 9, PrivateKey: "k"}}
	hostedMCP = domain.MCPServer{Name: "github-developer", Transport: domain.TransportHTTP, URL: "https://api.githubcopilot.com/mcp/"}
)

const addIssueComment = domain.ToolID("github-developer.add_issue_comment")

/*
The decisive case. A clean, autonomous run whose writes an authored policy
allows — so nothing but taint can park them. create_pull_request is left
untrusted. All three must hold at once: the link passes with no approval, the
run is tainted from the create_pull_request's return on, and a later ordinary
write in the same run parks for a person because of that taint.
*/
func TestWorkItemLink_anUntrustedHostedAnswer_provesTheLinkAndStillTaintsTheRun(t *testing.T) {
	j := provenanceJourney(t, hostedMCP, hostedApp, domain.StageAutonomous,
		engine.Proposal{Tool: addIssueComment, Args: []byte(`{}`)})
	start := j.open(t, "run-dev", "factory-developer", domain.ScopeLabels(j.scope))
	j.advance(t, start, 4) // claim; create_pull_request; link; the comment

	steps := readSteps(t, j, "run-dev")
	returned := stepOfTool(t, steps, domain.StepToolReturned, createPullRequest)
	if !returned.Labels.HasAny(domain.LabelUntrusted) || !strings.Contains(string(returned.Payload), `"via":"github_hosted_mcp_app"`) {
		t.Fatalf("create_pull_request returned %s with labels %v; want untrusted content and a trusted proof stamped", returned.Payload, returned.Labels)
	}
	if n := count(steps, domain.StepWorkItemLinked); n != 1 {
		t.Fatalf("links = %d; want the link sealed", n)
	}
	var asked []domain.ToolID
	for _, st := range steps {
		if st.Kind == domain.StepApprovalRequested {
			asked = append(asked, toolOf(t, st))
		}
	}
	if len(asked) != 1 || asked[0] != addIssueComment {
		t.Fatalf("approvals asked for %v; want only the later comment — the link passes, the taint stays", asked)
	}
	last := steps[len(steps)-1]
	if !last.Labels.HasAny(domain.LabelUntrusted) {
		t.Fatalf("the run's last step carries %v; the run must stay tainted after the link", last.Labels)
	}
}

func TestWorkItemLink_throughTheRealCatalogue_aRegistrationWithNoProvenanceDoesNotProve(t *testing.T) {
	elsewhere := hostedMCP
	elsewhere.URL = "https://github-mcp.example.com/mcp/"
	lookalike := hostedMCP
	lookalike.URL = "https://api.githubcopilot.com.evil.example/mcp/"
	token := domain.MCPCredentials{Headers: map[string]string{"Authorization": "Bearer ghp_x"}}
	for name, c := range map[string]struct {
		server domain.MCPServer
		creds  domain.MCPCredentials
	}{
		"another server offering create_pull_request": {elsewhere, hostedApp},
		"a look-alike host":                           {lookalike, hostedApp},
		"the hosted server with a personal token":     {hostedMCP, token},
	} {
		t.Run(name, func(t *testing.T) {
			j := provenanceJourney(t, c.server, c.creds, domain.StageAutonomous)
			start := j.open(t, "run-dev", "factory-developer", domain.ScopeLabels(j.scope))
			j.advance(t, start, 3) // claim; create_pull_request; the link parks

			steps := readSteps(t, j, "run-dev")
			if returned := stepOfTool(t, steps, domain.StepToolReturned, createPullRequest); strings.Contains(string(returned.Payload), `"pull_request"`) {
				t.Fatalf("an answer with no provenance was stamped: %s", returned.Payload)
			}
			if n := count(steps, domain.StepWorkItemLinked); n != 0 {
				t.Fatalf("links = %d; want none", n)
			}
			if requested := stepOfTool(t, steps, domain.StepApprovalRequested, domain.ToolWorkItemLink); requested.Seq == 0 {
				t.Fatal("the link did not park for a person")
			}
		})
	}
}

// provenanceJourney is a run over a real catalogue holding one GitHub-like
// server, registered as connectServer registers it, under the work-item layer.
// create_pull_request and add_issue_comment are writes left untrusted, and an
// authored policy allows both, so only taint can park them.
func provenanceJourney(
	t *testing.T, server domain.MCPServer, creds domain.MCPCredentials, stage domain.Stage, then ...engine.Proposal,
) journey {
	t.Helper()
	content := engine.NewMemoryContent()
	catalog := tools.NewCatalog(content)
	if err := catalog.AddServer(t.Context(), server.Name, githubLikeServer(t), nil,
		tools.WithProvenance(domain.HostedGitHubProvenance(server, creds))); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	var allow []domain.Policy
	for _, tool := range []domain.ToolID{createPullRequest, addIssueComment} {
		if err := catalog.Classify(domain.ToolClassification{Tool: tool, Effect: domain.EffectWrite, Untrusted: true}); err != nil {
			t.Fatalf("Classify %s: %v", tool, err)
		}
		allow = append(allow, domain.Policy{Code: "POL-" + string(tool), Resource: string(tool),
			Reach: domain.ReachInstallation, Effect: domain.PolicyAllow, Mode: domain.PolicyEnforce, Enabled: true})
	}
	store := ledger.NewMemory()
	layer := workitem.NewLayer(catalog, catalog)
	proposals := append([]engine.Proposal{
		claimOf("github:fuseone-io/fuseone-agents#204"),
		{Tool: createPullRequest, Args: []byte(`{}`)},
		{Tool: domain.ToolWorkItemLink, Args: refArgs("github:fuseone-io/fuseone-agents#240")},
	}, then...)
	runner := engine.NewRunner(engine.Deps{
		Ledger: store, Gate: gate.New().WithPolicies(gate.Policies{Hash: "pol_factory", Set: allow}),
		Tools: layer, Catalog: layer, Content: content, Clock: engine.SystemClock{},
		Planner: &journeyPlanner{proposals: proposals},
	})
	return journey{runner: runner, store: store, content: content, scope: domain.Scope{Company: "acme", Area: "factory"},
		stage: stage, pack: []domain.ToolID{addIssueComment}}
}

// githubLikeServer answers create_pull_request with a pull request's address,
// having opened nothing, and add_issue_comment with nothing much.
func githubLikeServer(t *testing.T) tools.Session {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "github", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "create_pull_request"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: prAddress}}}, nil, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "add_issue_comment"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: `{"id":1}`}}}, nil, nil
		})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "fuseone", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func readSteps(t *testing.T, j journey, id domain.RunID) []domain.Step {
	t.Helper()
	steps, err := j.store.Read(context.Background(), id, domain.FirstSeq)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, st := range steps {
		if st.Kind == domain.StepToolReturned && strings.Contains(string(st.Payload), `"failed":true`) {
			t.Fatalf("a call failed (%s); the server must answer for the case to mean anything", st.Payload)
		}
	}
	return steps
}

func count(steps []domain.Step, kind domain.StepKind) int {
	n := 0
	for _, st := range steps {
		if st.Kind == kind {
			n++
		}
	}
	return n
}

func toolOf(t *testing.T, st domain.Step) domain.ToolID {
	t.Helper()
	var p struct {
		Tool domain.ToolID `json:"tool"`
	}
	if err := json.Unmarshal(st.Payload, &p); err != nil {
		t.Fatalf("decode %s: %v", st.Kind, err)
	}
	return p.Tool
}

// stepOfTool is the first step of kind about tool, or the zero step.
func stepOfTool(t *testing.T, steps []domain.Step, kind domain.StepKind, tool domain.ToolID) domain.Step {
	t.Helper()
	for _, st := range steps {
		if st.Kind == kind && toolOf(t, st) == tool {
			return st
		}
	}
	return domain.Step{}
}
