package httpapi

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/model"
)

/*
A provider the environment supplies is the installation's under its name, and
every scope reaches it (NT-019). An area that stored a provider under that name
would, by the registry's precedence, take it from every other scope: refused.
A write at the installation still replaces it, deliberately.
*/

func envRegistry(t *testing.T) *model.Registry {
	t.Helper()
	reg := model.NewRegistry(nil)
	if err := reg.Register(model.Provider{Name: "anthropic", Kind: model.KindAnthropic}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return reg
}

func TestPutModelProvider_anAreaTakingAnEnvironmentProvidersName_isRefused(t *testing.T) {
	t.Parallel()
	fake := &fakeAdmin{}
	resp, err := serverWith(t, fake).WithProviderNames(envRegistry(t)).
		PutModelProvider(inArea("cx", domain.RoleAdmin), openapi.PutModelProviderRequestObject{
			Name: "anthropic",
			Body: &openapi.PutModelProviderJSONRequestBody{
				Kind: "anthropic", ScopeKind: ptr(openapi.ConnectorScopeKindArea), Company: ptr("acme"), Area: ptr("cx"),
			},
		})
	if err != nil {
		t.Fatalf("PutModelProvider: %v", err)
	}
	if _, taken := resp.(openapi.PutModelProvider409ApplicationProblemPlusJSONResponse); !taken || fake.putProvider.Name != "" {
		t.Fatalf("response = %T, stored %q; want the environment's name refused to an area", resp, fake.putProvider.Name)
	}
}

func TestPutModelProvider_theInstallationReplacingAnEnvironmentProvider_isStored(t *testing.T) {
	t.Parallel()
	fake := &fakeAdmin{}
	resp, err := serverWith(t, fake).WithProviderNames(envRegistry(t)).
		PutModelProvider(as(domain.RoleAdmin), openapi.PutModelProviderRequestObject{
			Name: "anthropic", Body: &openapi.PutModelProviderJSONRequestBody{Kind: "anthropic"},
		})
	if err != nil {
		t.Fatalf("PutModelProvider: %v", err)
	}
	if _, stored := resp.(openapi.PutModelProvider204Response); !stored || fake.putProvider.Name != "anthropic" {
		t.Fatalf("response = %T; want the installation's write stored", resp)
	}
}
