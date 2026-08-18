package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
)

func provider(mappings ...auth.GroupMapping) *auth.OIDCProvider {
	return &auth.OIDCProvider{ID: "keycloak", Mappings: mappings}
}

func TestGrantsFor_noMapping_grantsNothing(t *testing.T) {
	t.Parallel()

	// Authenticating proves who someone is. It must never by itself decide
	// what they may do, or a provider misconfiguration becomes an access
	// grant.
	got := provider().GrantsFor([]string{"cx-team", "everyone"})
	if len(got) != 0 {
		t.Errorf("GrantsFor = %v, want nothing without a mapping", got)
	}
}

func TestGrantsFor_mappedGroup_becomesAScopedGrant(t *testing.T) {
	t.Parallel()

	p := provider(
		auth.GroupMapping{Group: "cx-team", Company: "acme", Area: "cx", Role: "author"},
		auth.GroupMapping{Group: "plataforma", Company: "acme", Area: "cx", Role: "curator"},
	)

	got := p.GrantsFor([]string{"cx-team"})
	if len(got) != 1 {
		t.Fatalf("GrantsFor = %v, want exactly the mapped grant", got)
	}
	want := domain.Grant{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor}
	if got[0] != want {
		t.Errorf("GrantsFor = %v, want %v", got[0], want)
	}
}

func TestGrantsFor_groupComparisonIsExactApartFromCase(t *testing.T) {
	t.Parallel()

	p := provider(auth.GroupMapping{Group: "CX-Team", Company: "acme", Area: "cx", Role: "author"})

	if len(p.GrantsFor([]string{"cx-team"})) != 1 {
		t.Error("case difference blocked a legitimate mapping")
	}
	// No prefix or wildcard matching. A wildcard in an access rule is a
	// mistake nobody notices until it grants more than intended.
	if len(p.GrantsFor([]string{"cx-team-readonly"})) != 0 {
		t.Error("a longer group name matched by prefix")
	}
}

func TestGrantsFor_malformedMapping_grantsNothingRatherThanSomethingApproximate(t *testing.T) {
	t.Parallel()

	p := provider(
		auth.GroupMapping{Group: "a", Company: "acme", Area: "cx", Role: "wizard"},
		auth.GroupMapping{Group: "b", Company: "", Area: "cx", Role: "author"},
		auth.GroupMapping{Group: "c", Company: "acme", Area: "", Role: "author"},
	)

	// Silently widening a scope from a broken mapping would be invisible to
	// the operator who wrote it.
	if got := p.GrantsFor([]string{"a", "b", "c"}); len(got) != 0 {
		t.Errorf("GrantsFor = %v, want nothing from malformed mappings", got)
	}
}

func TestGrantsFor_twoGroupsMappingToTheSameGrant_deduplicate(t *testing.T) {
	t.Parallel()

	p := provider(
		auth.GroupMapping{Group: "cx", Company: "acme", Area: "cx", Role: "author"},
		auth.GroupMapping{Group: "suporte", Company: "acme", Area: "cx", Role: "author"},
	)

	if got := p.GrantsFor([]string{"cx", "suporte"}); len(got) != 1 {
		t.Errorf("GrantsFor = %v, want one grant", got)
	}
}

func TestStart_unknownProvider_isRejected(t *testing.T) {
	t.Parallel()

	o := auth.NewOIDC("https://console.example", true)
	rec := httptest.NewRecorder()

	err := o.Start(rec, httptest.NewRequest(http.MethodGet, "/auth/start/nope", nil), "nope", "/")
	if err == nil {
		t.Fatal("Start accepted an unconfigured provider")
	}
}

func TestStartWithProvider_usesTheReconciledSnapshot(t *testing.T) {
	t.Parallel()

	issuer := fakeOIDCIssuer(t)
	o := auth.NewOIDC("https://console.example", true)
	if err := o.Add(t.Context(), &auth.OIDCProvider{
		ID: "keycloak", Issuer: issuer, ClientID: "fuseone", ClientSecret: "secret",
	}); err != nil {
		t.Fatalf("add provider: %v", err)
	}
	provider, ok := o.Provider("keycloak")
	if !ok {
		t.Fatal("provider was not registered")
	}
	o.Remove("keycloak")
	rec := httptest.NewRecorder()

	err := o.StartWithProvider(rec, httptest.NewRequest(http.MethodGet, "/auth/start/keycloak", nil), provider, "/")

	if err != nil {
		t.Fatalf("StartWithProvider = %v, want the snapshot to remain usable", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want redirect", rec.Code)
	}
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, issuer+"/authorize?") {
		t.Fatalf("redirect = %q, want the reconciled provider endpoint", location)
	}
}

func TestComplete_withoutTheFlowCookie_isRejected(t *testing.T) {
	t.Parallel()

	o := auth.NewOIDC("https://console.example", true)
	rec := httptest.NewRecorder()

	// No flow cookie means either an expired sign-in or a callback fabricated
	// somewhere else. Neither may proceed.
	_, err := o.Complete(t.Context(), rec,
		httptest.NewRequest(http.MethodGet, "/auth/callback/keycloak?code=x&state=y", nil), "keycloak")
	if err == nil {
		t.Fatal("Complete accepted a callback with no flow in progress")
	}
}

func fakeOIDCIssuer(t *testing.T) string {
	t.Helper()

	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                issuer,
				"authorization_endpoint":                issuer + "/authorize",
				"token_endpoint":                        issuer + "/token",
				"jwks_uri":                              issuer + "/jwks",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
				"code_challenge_methods_supported":      []string{"S256"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	issuer = server.URL
	t.Cleanup(server.Close)
	return issuer
}
