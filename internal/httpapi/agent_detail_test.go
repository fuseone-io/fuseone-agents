package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// Reading one agent is reading what somebody told it to do, in the exact
// version a run was pinned to. Publishing never rewrites history, so the
// screen that shows a run's version has to be able to ask for that version
// rather than whatever is newest now.

func publishedVersions(t *testing.T) *fakeDetail {
	t.Helper()
	base := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

	return &fakeDetail{
		versions: []domain.AgentSummary{
			{
				ID: "triage", VersionID: "v2", Name: "Atendimento",
				Scope:    domain.Scope{Company: "acme", Area: "cx"},
				Provider: "openai", Model: "devstack", Latest: true,
				PublishedAt: base.Add(time.Hour), PublishedBy: "ana",
			},
			{
				ID: "triage", VersionID: "v1", Name: "Atendimento",
				Scope:    domain.Scope{Company: "acme", Area: "cx"},
				Provider: "openai", Model: "devstack",
				PublishedAt: base, PublishedBy: "ana",
			},
		},
		instructions: map[domain.VersionID]string{
			"v1": "responda o cliente",
			"v2": "responda o cliente, com aprovação",
		},
	}
}

func TestGetAgent_withoutAVersion_readsTheNewestPublished(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(publishedVersions(t)).
		GetAgent(inArea("cx", domain.RoleAuthor), openapi.GetAgentRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}

	got, ok := resp.(openapi.GetAgent200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the agent", resp)
	}
	if got.Agent.VersionId != "v2" {
		t.Errorf("version = %q, want the newest", got.Agent.VersionId)
	}
	if got.Instructions == nil || *got.Instructions != "responda o cliente, com aprovação" {
		t.Errorf("instructions = %v, want the newest version's text", got.Instructions)
	}
	if len(got.Versions) != 2 {
		t.Errorf("versions = %d, want the whole history", len(got.Versions))
	}
}

func TestGetAgent_namingAVersion_readsThatOne(t *testing.T) {
	t.Parallel()

	// A run pinned to v1 is explained by v1. Answering with v2 would show an
	// auditor instructions the run never ran under.
	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(publishedVersions(t)).
		GetAgent(inArea("cx", domain.RoleAuthor), openapi.GetAgentRequestObject{
			AgentId: "triage",
			Params:  openapi.GetAgentParams{Version: ptr("v1")},
		})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}

	got := resp.(openapi.GetAgent200JSONResponse)
	if got.Agent.VersionId != "v1" || *got.Instructions != "responda o cliente" {
		t.Errorf("read %q, want v1's own text", got.Agent.VersionId)
	}
}

func TestGetAgent_inAnotherArea_readsAsAbsent(t *testing.T) {
	t.Parallel()

	// Not forbidden: confirming an agent exists in an area somebody cannot see
	// is itself information about that area.
	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(publishedVersions(t)).
		GetAgent(inArea("marketing", domain.RoleAuthor), openapi.GetAgentRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if _, absent := resp.(openapi.GetAgent404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestGetAgent_neverPublished_readsAsAbsent(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(&fakeDetail{}).
		GetAgent(inArea("cx", domain.RoleAuthor), openapi.GetAgentRequestObject{AgentId: "ghost"})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if _, absent := resp.(openapi.GetAgent404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

// fakeDetail stands in for the registry, versions and text alike.
type fakeDetail struct {
	versions     []domain.AgentSummary
	instructions map[domain.VersionID]string
}

func (f *fakeDetail) List(_ context.Context, _ domain.Scope, _ bool) ([]domain.AgentSummary, error) {
	return f.versions, nil
}

func (f *fakeDetail) Versions(_ context.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	var out []domain.AgentSummary
	for _, v := range f.versions {
		if v.ID == agent {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeDetail) Instructions(
	_ context.Context, _ domain.AgentID, version domain.VersionID,
) (text, source string, err error) {
	return f.instructions[version], "dev/agents/triage.agent.md", nil
}
