package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The installation's address is where every browser comes back to: lost on
// the way from the flag to the sign-in, each return would go nowhere.
func TestMCPSignIn_carriesTheInstallationsAddressAndStore(t *testing.T) {
	t.Parallel()
	integrations := admin.NewIntegrations(nil, nil)
	cfg := mcpSignIn(integrations, "https://agents.example.com")
	if cfg.BaseURL != "https://agents.example.com" || cfg.Store != integrations || cfg.Client == nil || cfg.Now == nil {
		t.Errorf("config = %+v", cfg)
	}
	a, errA := cfg.Random()
	b, errB := cfg.Random()
	if errA != nil || errB != nil || a == b || len(a) != 43 {
		t.Errorf("random = %q, %q (%v, %v); want two distinct 32-byte values", a, b, errA, errB)
	}
}

// The return is behind the session: whoever comes back is judged against
// whoever began, and an anonymous browser never reaches it.
func TestMountMCPSignInReturn_isBehindTheSession(t *testing.T) {
	t.Parallel()
	root := http.NewServeMux()
	api := httpapi.NewServer(ledger.NewMemory(), "test")
	mountMCPSignInReturn(root, api, func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	})
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/mcp/callback?state=s&code=c", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want the session's refusal", rec.Code)
	}
}
