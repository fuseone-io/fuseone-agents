package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
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

type declaredDetail struct {
	steps []spec.Step
	emits spec.Emits
}

func (d declaredDetail) Declared(
	context.Context, domain.AgentID, domain.VersionID,
) ([]spec.Step, spec.Emits, error) {
	return d.steps, d.emits, nil
}

func TestGetAgent_eventContextIsReturnedWithTheVersion(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").
		WithAgents(publishedVersions(t)).
		WithDefinitions(declaredDetail{emits: spec.Emits{{
			Event: "incident.triaged", Context: "incident",
			Artifacts: []string{"triage_summary"},
		}}}).
		GetAgent(inArea("cx", domain.RoleAuthor), openapi.GetAgentRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}

	got := resp.(openapi.GetAgent200JSONResponse)
	if got.Emits == nil || len(*got.Emits) != 1 {
		t.Fatalf("emits = %+v, want the declared event", got.Emits)
	}
	event := (*got.Emits)[0]
	if event.Event != "incident.triaged" || event.Context == nil || *event.Context != "incident" ||
		event.Artifacts == nil || len(*event.Artifacts) != 1 || (*event.Artifacts)[0] != "triage_summary" {
		t.Errorf("event = %+v, want the context-carrying declaration", event)
	}
}

/*
Whether the agent is running.

Published paused, so the screen that publishes it is also the screen somebody
starts it from, and it cannot offer that without knowing the state. Read from
the agent rather than from the version: an older version is shown beside the
state of the agent that has it, because there is only one thing to start.
*/
func TestGetAgent_reportsWhetherItIsRunning(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(publishedVersions(t)).
		WithPauses(runningAgent{}).
		GetAgent(inArea("cx", domain.RoleAuthor), openapi.GetAgentRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	got := resp.(openapi.GetAgent200JSONResponse)
	if got.Agent.Paused == nil || *got.Agent.Paused {
		t.Errorf("paused = %v, want it reported as running", got.Agent.Paused)
	}
}

// An agent nobody ever decided about is stopped, and reads as stopped. The
// safe direction: showing a stopped agent as live because a row is missing is
// how somebody concludes the platform is acting when it is not.
func TestGetAgent_nobodyEverDecided_readsAsStopped(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(publishedVersions(t)).
		WithPauses(pausedAgent{}).
		GetAgent(inArea("cx", domain.RoleAuthor), openapi.GetAgentRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	got := resp.(openapi.GetAgent200JSONResponse)
	if got.Agent.Paused == nil || !*got.Agent.Paused {
		t.Errorf("paused = %v, want it reported as stopped", got.Agent.Paused)
	}
}

type runningAgent struct{}

func (runningAgent) IsPaused(context.Context, domain.AgentID) (bool, error) { return false, nil }

func (runningAgent) Paused(context.Context) (map[domain.AgentID]bool, error) {
	return map[domain.AgentID]bool{"triage": false}, nil
}
