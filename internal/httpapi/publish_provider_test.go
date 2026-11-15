package httpapi

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

/*
Publishing checks the main pair's provider against the agent's scope (NT-019,
phase 3, Enforcement): configured there, whatever this process can do with it
now. Configured and not Usable, because a provider switched off, or whose key
did not open in this process, is still a configuration; the worker's planner is
what refuses a run on it.
*/

func publishOn(t *testing.T, reg ProviderNames) (openapi.PublishAgentResponseObject, *publisher) {
	t.Helper()
	pub := newPublisher()
	resp, err := publishServer(t, pub).WithProviderNames(reg).
		PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
			AgentId: "triage", Body: definition(func(*openapi.AgentDefinition) {}),
		})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	return resp, pub
}

func TestPublishAgent_aMainProviderConfiguredOnlyForAnotherArea_isRefused(t *testing.T) {
	t.Parallel()
	resp, pub := publishOn(t, providersAt(t, map[string]domain.Scope{"openai": {Company: "acme", Area: "ops"}}))
	bad, refused := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse)
	if !refused || len(pub.published) != 0 {
		t.Fatalf("response = %T, published %d; want acme/ops's provider refused for an agent of acme/cx", resp, len(pub.published))
	}
	if !strings.Contains(valueOr(bad.Detail), `"openai"`) || strings.Contains(valueOr(bad.Detail), "ops") {
		t.Errorf("detail = %q, want the provider named and its owner not", valueOr(bad.Detail))
	}
}

func TestPublishAgent_aMainProviderNobodyConfigured_isRefused(t *testing.T) {
	t.Parallel()
	resp, pub := publishOn(t, providersAt(t, map[string]domain.Scope{"anthropic": installationScope}))
	if _, refused := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !refused || len(pub.published) != 0 {
		t.Fatalf("response = %T, published %d; want an unknown provider refused", resp, len(pub.published))
	}
}

// Switched off, or sealed in this process: configured all the same.
func TestPublishAgent_aMainProviderClaimedButNotBuiltHere_isPublished(t *testing.T) {
	t.Parallel()
	reg := model.NewRegistry(nil)
	reg.SetConfigured(nil, model.Claim{Name: "openai", Scope: domain.Scope{Company: "acme"}})
	if _, pub := publishOn(t, reg); len(pub.published) != 1 {
		t.Fatal("a provider configured for the agent's company and not built here was refused")
	}
}

func TestPublishAgent_aMainProviderOfTheInstallation_isPublished(t *testing.T) {
	t.Parallel()
	if _, pub := publishOn(t, providersAt(t, map[string]domain.Scope{"openai": installationScope})); len(pub.published) != 1 {
		t.Fatal("the installation's provider was refused")
	}
}
