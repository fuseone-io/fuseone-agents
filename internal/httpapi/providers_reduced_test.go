package httpapi

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

/*
GET /providers is the reduced answer an author picks a provider and a model
from (NT-019, rule 5, phase 2 writers part B2a): names, kinds, models and
whether each is usable, asked for one target as ListTools is, and never an
address or anything sealed.

Through the real store: the platform's main (with a key and an address) and
spare (switched off), and, planted as a later phase will store them, a
provider of cx and one of finance.
*/

const (
	sealedKey   = "sk-sealed-provider-key"
	mainAddress = "https://llm.example.com/v1"
)

func newProvidersWorld(t *testing.T) *Server {
	t.Helper()
	w := newClassifyWorld(t)
	for _, p := range []struct {
		name    string
		enabled bool
	}{{"main", true}, {"spare", false}} {
		resp, err := w.api.PutModelProvider(as(domain.RoleAdmin), openapi.PutModelProviderRequestObject{
			Name: p.name, Body: &openapi.PutModelProviderJSONRequestBody{
				Kind: openapi.PutModelProviderJSONBodyKindOpenaiCompatible, BaseUrl: ptr(mainAddress),
				ApiKey: ptr(sealedKey), Models: &[]string{"gpt-x", "gpt-y"}, Enabled: ptr(p.enabled),
			},
		})
		if _, ok := resp.(openapi.PutModelProvider204Response); err != nil || !ok {
			t.Fatalf("PutModelProvider %s = %#v, %v", p.name, resp, err)
		}
	}
	if _, err := w.pool.Exec(t.Context(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'cx', 'model_provider', 'cx-llm',
		        '{"kind":"anthropic","baseURL":"https://cx.example.com","models":["claude-x"]}', true, 'w'),
		       ('area', 'acme', 'finance', 'model_provider', 'fin-llm',
		        '{"kind":"anthropic","baseURL":"https://fin.example.com","models":["claude-y"]}', true, 'w')`); err != nil {
		t.Fatalf("plant the areas' providers: %v", err)
	}
	return w.api.WithProviderNames(configuredProviders{"main": true, "cx-llm": true})
}

// providersFor is ListProviders for a target ("" for none): the status, and
// the names of a 200, sorted.
func providersFor(t *testing.T, s *Server, caller context.Context, target string) (string, []string) {
	t.Helper()
	req := openapi.ListProvidersRequestObject{}
	if target != "" {
		req.Params.Scope = ptr(target)
	}
	resp, err := s.ListProviders(caller, req)
	if err != nil {
		t.Fatalf("ListProviders %q: %v", target, err)
	}
	switch r := resp.(type) {
	case openapi.ListProviders200JSONResponse:
		names := make([]string, 0, len(r.Items))
		for _, p := range r.Items {
			names = append(names, p.Name)
		}
		slices.Sort(names)
		return "200", names
	case openapi.ListProviders400ApplicationProblemPlusJSONResponse:
		return "400", nil
	case openapi.ListProviders403ApplicationProblemPlusJSONResponse:
		return "403", nil
	case openapi.ListProviders404ApplicationProblemPlusJSONResponse:
		return "404", nil
	}
	t.Fatalf("ListProviders %q = %T", target, resp)
	return "", nil
}

func TestListProviders_answersNamesKindsModelsAndUsable_neverAnAddressOrASecret(t *testing.T) {
	s := newProvidersWorld(t)
	resp, err := s.ListProviders(as(domain.RoleAuthor), openapi.ListProvidersRequestObject{})
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	page, ok := resp.(openapi.ListProviders200JSONResponse)
	if !ok {
		t.Fatalf("ListProviders = %T", resp)
	}
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, leak := range []string{sealedKey, "example.com", "baseUrl", "baseURL", "hasKey", "secret", "updatedBy"} {
		if strings.Contains(string(body), leak) {
			t.Errorf("the answer carries %q: %s", leak, body)
		}
	}
	got := map[string]openapi.ProviderChoice{}
	for _, p := range page.Items {
		got[p.Name] = p
	}
	main, spare := got["main"], got["spare"]
	if !main.Usable || main.Kind != openapi.ProviderChoiceKindOpenaiCompatible ||
		!slices.Equal(main.Models, []string{"gpt-x", "gpt-y"}) {
		t.Errorf("main = %+v, want openai_compatible, its models, usable", main)
	}
	if spare.Usable {
		t.Errorf("spare = %+v, want not usable: it is switched off", spare)
	}
	// Switched on, and this process built no client for it: its credential
	// did not open, or its address is not whole.
	if fin := got["fin-llm"]; fin.Usable {
		t.Errorf("fin-llm = %+v, want not usable: the registry would not ask it", fin)
	}
	if len(page.Items) != 4 {
		t.Errorf("the platform's reader is answered %d providers, want every one of 4", len(page.Items))
	}
}

func TestListProviders_isAskedForATarget_asListToolsIs(t *testing.T) {
	s := newProvidersWorld(t)
	installation := []string{"main", "spare"}
	for _, one := range []struct {
		name   string
		caller context.Context
		target string
		status string
		want   []string
	}{
		{"an area author, its own area", grantedAt(domain.RoleAuthor, cx), "acme/cx", "200", []string{"cx-llm", "main", "spare"}},
		{"an author of two areas, one of them", grantedOnEach(domain.RoleAuthor, cx, finance), "acme/cx", "200",
			[]string{"cx-llm", "main", "spare"}},
		{"a company author, one of its areas", grantedAt(domain.RoleAuthor, acmeOnly), "acme/cx", "200",
			[]string{"cx-llm", "main", "spare"}},
		{"an area author, a sibling area", grantedAt(domain.RoleAuthor, cx), "acme/finance", "404", nil},
		{"an author elsewhere, approver of the target", authorHereApproverThere, "acme/cx", "404", nil},
		{"a malformed target", grantedAt(domain.RoleAuthor, cx), "acme/CX", "400", nil},
		{"the installation as a target", grantedAt(domain.RoleAuthor, cx), "*", "400", nil},
		{"an area author, no target", grantedAt(domain.RoleAuthor, cx), "", "200", installation},
		{"no tool:read anywhere", grantedAt(domain.RoleApprover, cx), "acme/cx", "403", nil},
	} {
		got, names := providersFor(t, s, one.caller, one.target)
		if got != one.status || !slices.Equal(names, one.want) {
			t.Errorf("%s: %s %v, want %s %v", one.name, got, names, one.status, one.want)
		}
	}
}

// Unwired, as on a laptop, nothing says what this process built: a provider
// switched off is still not usable, and one switched on reads as usable.
func TestListProviders_unwired_aSwitchedOffProviderIsNotUsable(t *testing.T) {
	s := newProvidersWorld(t)
	s.providerNames = nil
	resp, err := s.ListProviders(as(domain.RoleAuthor), openapi.ListProvidersRequestObject{})
	page, ok := resp.(openapi.ListProviders200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("ListProviders = %#v, %v", resp, err)
	}
	for _, p := range page.Items {
		if want := p.Name != "spare"; p.Usable != want {
			t.Errorf("%s usable = %v, want %v", p.Name, p.Usable, want)
		}
	}
}
