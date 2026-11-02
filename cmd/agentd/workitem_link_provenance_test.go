package main

import (
	"context"
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
review round 1), through the real catalogue.

A server is registered in a tools.Catalog with the provenance connectServer
gives it — domain.HostedGitHubProvenance of its registration — and its
create_pull_request ruled as the Curator rules it. It answers with a plausible
pull request address whether or not it opened one. Only the hosted GitHub
server reached as an App, with its result ruled trusted, lets the link pass
without a person; every other combination parks the link for approval.
*/

const prAddress = `{"id":"9","url":"https://github.com/fuseone-io/fuseone-agents/pull/240"}`

func TestWorkItemLink_throughTheRealCatalogue_onlyATrustedHostedGitHubAnswerProves(t *testing.T) {
	hostedApp := domain.MCPCredentials{GitHubApp: &domain.MCPGitHubApp{AppID: 7, InstallationID: 9, PrivateKey: "k"}}
	hosted := domain.MCPServer{Name: "github-developer", Transport: domain.TransportHTTP, URL: "https://api.githubcopilot.com/mcp/"}
	elsewhere := hosted
	elsewhere.URL = "https://github-mcp.example.com/mcp/"
	token := domain.MCPCredentials{Headers: map[string]string{"Authorization": "Bearer ghp_x"}}

	cases := []struct {
		name      string
		server    domain.MCPServer
		creds     domain.MCPCredentials
		untrusted bool
		proves    bool
	}{
		{"the hosted GitHub server as an App, ruled trusted", hosted, hostedApp, false, true},
		{"the hosted server, its result ruled untrusted", hosted, hostedApp, true, false},
		{"another server offering create_pull_request", elsewhere, hostedApp, false, false},
		{"the hosted server with a personal token", hosted, token, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := provenanceJourney(t, c.server, c.creds, c.untrusted)
			start := j.open(t, "run-dev", "factory-developer")
			j.advance(t, start, 2) // the claim; create_pull_request parks
			j.approveLatest(t, "run-dev")
			j.advance(t, start, 2) // the approved create_pull_request; the link

			steps, err := j.store.Read(context.Background(), "run-dev", domain.FirstSeq)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var linked, asked, failed, stamped int
			for _, st := range steps {
				if st.Kind == domain.StepToolReturned && strings.Contains(string(st.Payload), `"failed":true`) {
					failed++
				}
				if st.Kind == domain.StepToolReturned && strings.Contains(string(st.Payload), `"pull_request"`) {
					stamped++
				}
				switch st.Kind {
				case domain.StepWorkItemLinked:
					linked++
				case domain.StepApprovalRequested:
					asked++
				}
			}
			if failed != 0 {
				t.Fatalf("%d calls failed; the server must answer for the case to mean anything", failed)
			}
			if c.proves && (linked != 1 || asked != 1) {
				t.Fatalf("links %d, approvals asked %d; want the link sealed with no approval of its own", linked, asked)
			}
			// The answer is not even recorded as naming a pull request: the
			// engine stamps only what may prove, and the fold and the ledger
			// refusing a stamp are the second and third lines, not the first.
			if !c.proves && stamped != 0 {
				t.Fatalf("an answer that may not prove anything was stamped %d times", stamped)
			}
			if !c.proves && (linked != 0 || asked != 2) {
				t.Fatalf("links %d, approvals asked %d; want the link parked for a person", linked, asked)
			}
		})
	}
}

// provenanceJourney is a run over a real catalogue holding one GitHub-like
// server, registered as connectServer registers it, under the work-item layer.
func provenanceJourney(t *testing.T, server domain.MCPServer, creds domain.MCPCredentials, untrusted bool) journey {
	t.Helper()
	content := engine.NewMemoryContent()
	catalog := tools.NewCatalog(content)
	session := createPullRequestServer(t)
	if err := catalog.AddServer(t.Context(), server.Name, session, nil,
		tools.WithProvenance(domain.HostedGitHubProvenance(server, creds))); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if err := catalog.Classify(domain.ToolClassification{
		Tool: createPullRequest, Effect: domain.EffectWrite, Untrusted: untrusted,
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	store := ledger.NewMemory()
	layer := workitem.NewLayer(catalog, catalog)
	runner := engine.NewRunner(engine.Deps{
		Ledger: store, Gate: gate.New(), Tools: layer, Catalog: layer, Content: content,
		Clock: engine.SystemClock{},
		Planner: &journeyPlanner{proposals: []engine.Proposal{
			claimOf("github:fuseone-io/fuseone-agents#204"),
			{Tool: createPullRequest, Args: []byte(`{}`)},
			{Tool: domain.ToolWorkItemLink, Args: refArgs("github:fuseone-io/fuseone-agents#240")},
		}},
	})
	return journey{runner: runner, store: store, content: content, scope: domain.Scope{Company: "acme", Area: "factory"}}
}

// createPullRequestServer answers create_pull_request with a pull request's
// address, having opened nothing.
func createPullRequestServer(t *testing.T) tools.Session {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "github", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "create_pull_request"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: prAddress}}}, nil, nil
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
