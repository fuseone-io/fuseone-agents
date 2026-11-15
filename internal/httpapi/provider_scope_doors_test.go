package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
)

/*
Every door that names a provider asks the registry at the scope the agent will
run in (NT-019, phase 3 readers). Against the real registry, so the check a door
makes is the one the worker's planner makes.
*/

func providersAt(t *testing.T, placed map[string]domain.Scope) *model.Registry {
	t.Helper()
	reg := model.NewRegistry(nil)
	var ps []model.Provider
	var claims []model.Claim
	for name, scope := range placed {
		ps = append(ps, model.Provider{Name: name, Kind: model.KindOpenAICompatible, BaseURL: "http://127.0.0.1:1", Scope: scope})
		claims = append(claims, model.Claim{Name: name, Scope: scope})
	}
	reg.SetConfigured(ps, claims...)
	return reg
}

func TestPublishAgent_aFallbackConfiguredOnlyForAnotherArea_isRefused(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	reg := providersAt(t, map[string]domain.Scope{
		"openai": installationScope, "backup": {Company: "acme", Area: "ops"},
	})
	resp, err := publishServer(t, pub).WithProviderNames(reg).
		PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
			AgentId: "triage", Body: definition(withFallbackPair("backup", "backup-model")),
		})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, refused := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !refused || len(pub.published) != 0 {
		t.Fatalf("response = %T, published = %d; want acme/ops's fallback refused for an agent of acme/cx", resp, len(pub.published))
	}
}

func TestPublishAgent_aFallbackOfTheAgentsCompany_isPublished(t *testing.T) {
	t.Parallel()
	pub := newPublisher()
	reg := providersAt(t, map[string]domain.Scope{"openai": installationScope, "backup": {Company: "acme"}})
	if _, err := publishServer(t, pub).WithProviderNames(reg).
		PublishAgent(inArea("cx", domain.RoleAuthor), openapi.PublishAgentRequestObject{
			AgentId: "triage", Body: definition(withFallbackPair("backup", "backup-model")),
		}); err != nil || len(pub.published) != 1 {
		t.Fatalf("PublishAgent = %v, published %d; want the company's fallback accepted", err, len(pub.published))
	}
}

// An experiment opens no run on a provider the agent's scope does not reach:
// each would only park at the worker's planner.
func TestStartExperiment_onAProviderOfAnotherArea_isRefused(t *testing.T) {
	t.Parallel()
	reg := providersAt(t, map[string]domain.Scope{"anthropic": {Company: "acme", Area: "ops"}})
	resp, err := experimentServer(t, ledger.NewMemory()).WithProviderNames(reg).
		StartExperiment(inArea("cx", domain.RoleAuthor), startExperiment(openapi.ExperimentRequest{}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	if _, accepted := resp.(openapi.StartExperiment202JSONResponse); accepted {
		t.Fatal("an experiment of an acme/cx agent opened runs on acme/ops's provider")
	}
}

// Usable is judged at the target asked for, so an author is not told a
// provider is usable for an agent whose scope does not reach it.
func TestListProviders_usableIsJudgedAtTheTarget(t *testing.T) {
	s := newProvidersWorld(t)
	s.WithProviderNames(providersAt(t, map[string]domain.Scope{
		"main": installationScope, "cx-llm": {Company: "acme", Area: "cx"},
	}))
	resp, err := s.ListProviders(as(domain.RoleAuthor), openapi.ListProvidersRequestObject{
		Params: openapi.ListProvidersParams{Scope: ptr("acme/cx")},
	})
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	usable := map[string]bool{}
	for _, p := range resp.(openapi.ListProviders200JSONResponse).Items {
		usable[p.Name] = p.Usable
	}
	if !usable["main"] || !usable["cx-llm"] {
		t.Errorf("usable at acme/cx = %v, want main and cx-llm", usable)
	}
}

func TestStartExperiment_onAProviderOfTheAgentsArea_isAccepted(t *testing.T) {
	t.Parallel()
	reg := providersAt(t, map[string]domain.Scope{"anthropic": {Company: "acme", Area: "cx"}})
	resp, err := experimentServer(t, ledger.NewMemory()).WithProviderNames(reg).
		StartExperiment(inArea("cx", domain.RoleAuthor), startExperiment(openapi.ExperimentRequest{}))
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	if _, accepted := resp.(openapi.StartExperiment202JSONResponse); !accepted {
		t.Fatalf("response = %T, want an acme/cx agent's experiment on acme/cx's provider accepted", resp)
	}
}

// With no target, the platform's reader is listed every provider, each judged
// usable at its own scope.
func TestListProviders_withNoTarget_eachIsJudgedAtItsOwnScope(t *testing.T) {
	s := newProvidersWorld(t)
	s.WithProviderNames(providersAt(t, map[string]domain.Scope{
		"main": installationScope, "cx-llm": {Company: "acme", Area: "cx"},
	}))
	resp, err := s.ListProviders(as(domain.RoleAuthor), openapi.ListProvidersRequestObject{})
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	usable := map[string]bool{}
	for _, p := range resp.(openapi.ListProviders200JSONResponse).Items {
		usable[p.Name] = p.Usable
	}
	if !usable["cx-llm"] {
		t.Errorf("usable with no target = %v, want cx-llm judged at acme/cx", usable)
	}
}
