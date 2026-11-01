package httpapi

import (
	"context"
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// syncedHooks records what a publish reconciles, and is otherwise spyHooks.
type syncedHooks struct {
	spyHooks
	synced map[domain.AgentID][]domain.WebhookTrigger
}

func (s *syncedHooks) Sync(_ context.Context, agent domain.AgentID, _ domain.Scope, hooks []domain.WebhookTrigger) error {
	if s.synced == nil {
		s.synced = map[domain.AgentID][]domain.WebhookTrigger{}
	}
	s.synced[agent] = hooks
	return nil
}

func githubTrigger() openapi.AgentTrigger {
	from := openapi.AgentTriggerFromGithub
	return openapi.AgentTrigger{
		Type: "webhook", Path: ptr("gh/reviewer"), From: &from,
		Events: &[]string{"pull_request.opened", "issues"},
	}
}

/*
A GitHub webhook typed into the console reaches the door with its source and
events.

The console publishes by rendering a file from the form. A field the mapping
dropped on the way would publish a GitHub hook as an ordinary one: GitHub's
signed deliveries would all be refused, and the screen would print back a
trigger that looks configured.
*/
func TestPublishAgent_aGitHubWebhook_isReconciledWithItsSourceAndEvents(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	hooks := &syncedHooks{}

	if _, err := publishServer(t, pub).WithWebhooks(hooks).PublishAgent(
		inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{
			AgentId: "triage",
			Body: definition(func(d *openapi.AgentDefinition) {
				d.Triggers = &[]openapi.AgentTrigger{githubTrigger()}
			}),
		},
	); err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}

	want := []domain.WebhookTrigger{{
		Path: "gh/reviewer", From: domain.WebhookFromGitHub, Events: []string{"pull_request.opened", "issues"},
	}}
	if got := hooks.synced["triage"]; !reflect.DeepEqual(got, want) {
		t.Errorf("synced = %+v, want %+v", got, want)
	}
	if len(pub.published) != 1 || pub.published[0].Triggers[0].From != domain.WebhookFromGitHub {
		t.Errorf("published = %+v, want the version to carry the source", pub.published)
	}
}

// Read back, the agent shows the source and events it was published with, so
// the next save from the same form writes them again.
func TestAgentFrom_aGitHubWebhook_showsItsSourceAndEvents(t *testing.T) {
	t.Parallel()

	got := agentFrom(domain.AgentSummary{ID: "triage", Triggers: []domain.AgentTrigger{{
		Type: "webhook", Path: "gh/reviewer", From: domain.WebhookFromGitHub,
		Events: []string{"pull_request.opened", "issues"},
	}}})

	if got.Triggers == nil || !reflect.DeepEqual((*got.Triggers)[0], githubTrigger()) {
		t.Errorf("triggers = %+v, want %+v", got.Triggers, githubTrigger())
	}
}

func githubWebhookServer(t *testing.T, hooks *spyHooks) *Server {
	t.Helper()
	return NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).WithWebhooks(hooks)
}

// Without the master key a GitHub secret could only be stored in the clear.
// The operator reads why the button did nothing, not a 500.
func TestRotateWebhookSecret_aGitHubHookWithNoMasterKey_isRefusedWithAReason(t *testing.T) {
	t.Parallel()
	hooks := &spyHooks{path: "gh/reviewer", agent: "triage", from: domain.WebhookFromGitHub, noVault: true}

	resp, err := githubWebhookServer(t, hooks).RotateWebhookSecret(
		inArea("cx", domain.RoleAuthor),
		openapi.RotateWebhookSecretRequestObject{AgentId: "triage", Path: "gh/reviewer"},
	)
	if err != nil {
		t.Fatalf("RotateWebhookSecret: %v", err)
	}
	refused, ok := resp.(openapi.RotateWebhookSecret409ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 409", resp)
	}
	if refused.Type == nil || *refused.Type != "fuseone:no-master-key" {
		t.Errorf("problem type = %v, want fuseone:no-master-key", refused.Type)
	}
}

// The list says which paths GitHub signs, so the console can say where the
// secret goes.
func TestListWebhooks_aGitHubHook_saysItComesFromGitHub(t *testing.T) {
	t.Parallel()
	hooks := &spyHooks{path: "gh/reviewer", agent: "triage", from: domain.WebhookFromGitHub}

	resp, err := githubWebhookServer(t, hooks).ListWebhooks(
		inArea("cx", domain.RoleAuthor), openapi.ListWebhooksRequestObject{AgentId: "triage"})
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	list, ok := resp.(openapi.ListWebhooks200JSONResponse)
	if !ok || len(list.Items) != 1 || list.Items[0].From == nil || *list.Items[0].From != openapi.WebhookFromGithub {
		t.Errorf("response = %+v, want the hook marked as from github", resp)
	}
}
