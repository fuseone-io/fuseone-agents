package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
)

func TestAuthProviders_listsDurableProvidersNotOnlyThisProcessRegistry(t *testing.T) {
	t.Parallel()

	oidc := auth.NewOIDC("https://console.example", true)
	routes := NewAuthRoutes(oidc, nil, nil, true).
		WithIdentityProviders(&fakeIdentity{stored: []domain.IdentityProvider{{
			ID: "keycloak", Display: "Keycloak", Issuer: "https://id.example", Enabled: true,
		}}})
	mux := http.NewServeMux()
	routes.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/providers", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Providers []struct {
			ID      string `json:"id"`
			Display string `json:"display"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Providers) != 1 || body.Providers[0].ID != "keycloak" {
		t.Fatalf("providers = %+v, want the provider stored by another replica", body.Providers)
	}
}

func TestAuthStart_loadsTheProviderFromTheDurableStoreBeforeRedirecting(t *testing.T) {
	t.Parallel()

	issuer := fakeOIDCIssuer(t)
	oidc := auth.NewOIDC("https://console.example", true)
	routes := NewAuthRoutes(oidc, nil, nil, true).
		WithIdentityProviders(&fakeIdentity{
			secret: "client-secret",
			stored: []domain.IdentityProvider{{
				ID: "keycloak", Display: "Keycloak", Issuer: issuer,
				ClientID: "fuseone", Enabled: true,
				Mappings: []domain.GroupMapping{{
					Group: "suporte", Company: "acme", Area: "cx", Role: "author",
				}},
			}},
		})
	mux := http.NewServeMux()
	routes.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/start/keycloak?returnTo=%2Fruns", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, issuer+"/authorize?") {
		t.Fatalf("redirect = %q, want the discovered authorization endpoint", location)
	}
	if !strings.Contains(location, "client_id=fuseone") ||
		!strings.Contains(location, "redirect_uri=https%3A%2F%2Fconsole.example%2Fauth%2Fcallback%2Fkeycloak") {
		t.Fatalf("redirect = %q, want the provider loaded with this installation's client settings", location)
	}
}

func TestAuthStart_removesAProviderThatOnlyThisProcessStillRemembers(t *testing.T) {
	t.Parallel()

	issuer := fakeOIDCIssuer(t)
	oidc := auth.NewOIDC("https://console.example", true)
	if err := oidc.Add(t.Context(), &auth.OIDCProvider{
		ID: "keycloak", Issuer: issuer, ClientID: "fuseone", ClientSecret: "old-secret",
	}); err != nil {
		t.Fatalf("seed live provider: %v", err)
	}
	routes := NewAuthRoutes(oidc, nil, nil, true).
		WithIdentityProviders(&fakeIdentity{})
	mux := http.NewServeMux()
	routes.Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/start/keycloak", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want stale provider refused; body %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Location"), issuer+"/authorize") {
		t.Fatal("stale provider still redirected to the old identity provider")
	}
}

func fakeOIDCIssuer(t *testing.T) string {
	t.Helper()

	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeJSON(w, http.StatusOK, map[string]any{
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
