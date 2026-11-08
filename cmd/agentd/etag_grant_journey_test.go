package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
)

// grantBody is a tool server whose only credential is an OAuth grant with this refresh token.
func grantBody(refresh string) map[string]any {
	return map[string]any{"transport": "http", "url": "https://mcp.example.com/a", "oauth": map[string]any{
		"refreshToken": refresh, "tokenURL": "https://auth.example.com/token", "clientID": "client", "clientSecret": "ETAG-CLIENT",
	}}
}

// Two people pasting different grants over the same revision are two writers like any
// other: one lands, and the other is told what is there now instead of replacing it.
func TestSettingsETag_twoGrantsPastedWithOneETag_exactlyOneLands(t *testing.T) {
	seeding := newReadBackWorld(t)
	r := managedResources()[0]
	t.Cleanup(func() { seeding.call(t, etagCall{method: http.MethodDelete, path: r.path}) })
	first := seeding.call(t, etagCall{method: http.MethodPut, path: r.path, body: grantBody("ETAG-GRANT-SEED")})
	if first.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d: %s", first.Code, first.Body)
	}
	const writers = 4
	contention := testdb.NewContention(t, writers)
	w := readBackWorldOn(t, contention.Pool)

	codes := make([]int, writers)
	writes := make([]func() error, writers)
	for i := range writes {
		writes[i] = func() error {
			body := grantBody("ETAG-GRANT-" + string(rune('A'+i)))
			codes[i] = w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{first.Header().Get("ETag")}, body: body}).Code
			return nil
		}
	}
	contention.Overlap(t, `select 1 from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5 for update`,
		[]any{string(r.row.ScopeKind), string(r.row.Scope.Company), string(r.row.Scope.Area), string(r.row.Kind), r.row.Name}, writes)

	won := 0
	for _, code := range codes {
		switch code {
		case http.StatusNoContent:
			won++
		case http.StatusPreconditionFailed:
		default:
			t.Fatalf("a writer got %d", code)
		}
	}
	if won != 1 {
		t.Fatalf("codes = %v, want one 204 and the rest 412", codes)
	}
}

// A person pasting a grant moves the revision; the worker rotating it does not, so the
// revision a client read still names the grant somebody chose.
func TestSettingsETag_aPastedGrantMovesIt_aWorkerRotationDoesNot(t *testing.T) {
	w := newReadBackWorld(t)
	r := managedResources()[0]
	t.Cleanup(func() { w.call(t, etagCall{method: http.MethodDelete, path: r.path}) })
	paste := func(refresh string) string {
		t.Helper()
		put := w.call(t, etagCall{method: http.MethodPut, path: r.path, body: grantBody(refresh)})
		if put.Code != http.StatusNoContent {
			t.Fatalf("PUT = %d: %s", put.Code, put.Body)
		}
		return put.Header().Get("ETag")
	}
	pasted := paste("ETAG-REFRESH-ONE")
	before := held(t, r)
	if again := paste("ETAG-REFRESH-ONE"); again != pasted || held(t, r) != before {
		t.Fatalf("pasting the grant that is held moved something: %s, then %s", pasted, again)
	}

	rotate(t, w, r.row.Name)
	if read := w.call(t, etagCall{method: http.MethodGet, path: r.path}).Header().Get("ETag"); read != pasted {
		t.Fatalf("a worker rotation moved the revision: %s, then %s", pasted, read)
	}

	// The grant pasted first, sent back over its rotation, is a person replacing tokens.
	if back := paste("ETAG-REFRESH-ONE"); back == pasted {
		t.Fatal("re-pasting a rotated grant did not move the revision")
	}
	other := paste("ETAG-REFRESH-TWO")
	if !strings.Contains(revealed(t, w, r), "ETAG-REFRESH-TWO") {
		t.Fatal("the pasted token was dropped")
	}

	// The OAuth client lives only in the sealed document, and a read answers it.
	another := grantBody("ETAG-REFRESH-TWO")
	another["oauth"].(map[string]any)["clientID"] = "another-client"
	if moved := w.call(t, etagCall{method: http.MethodPut, path: r.path, body: another}); moved.Header().Get("ETag") == other {
		t.Error("another OAuth client did not move the revision")
	}
}

// rotate stands in for a worker refreshing the stored grant. The generation is not the
// worker's to carry, so the refreshed grant arrives without one.
func rotate(t *testing.T, w *readBackWorld, name string) {
	t.Helper()
	row, err := w.store.Reveal(t.Context(), settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, name)
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	was := domain.ReadMCPCredentials(row.Secret).OAuth
	next := *was
	next.AccessToken, next.RefreshToken, next.ExpiresAtUnix = "ETAG-ROTATED", "ETAG-ROTATED-REFRESH", 1_900_000_000
	next.Generation = ""
	if err := admin.NewIntegrations(testdb.Pool(t), w.store).
		RefreshMCPServerOAuth(t.Context(), name, "", *was, next); err != nil {
		t.Fatalf("rotate: %v", err)
	}
}

func revealed(t *testing.T, w *readBackWorld, r managed) string {
	t.Helper()
	row, err := w.store.Reveal(t.Context(), r.row.ScopeKind, r.row.Scope, r.row.Kind, r.row.Name)
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	return row.Secret
}

// A removal leaves no generation behind. The same grant pasted again names the state it
// named before, under the same revision; another grant does not.
func TestSettingsETag_aGrantRemovedAndPastedAgain_isTheStateItWas(t *testing.T) {
	w := newReadBackWorld(t)
	r := managedResources()[0]
	t.Cleanup(func() { w.call(t, etagCall{method: http.MethodDelete, path: r.path}) })
	put := func(body map[string]any) string {
		t.Helper()
		got := w.call(t, etagCall{method: http.MethodPut, path: r.path, body: body})
		if got.Code != http.StatusNoContent {
			t.Fatalf("PUT = %d: %s", got.Code, got.Body)
		}
		return got.Header().Get("ETag")
	}
	first := put(grantBody("ETAG-REFRESH-T"))
	removed := grantBody("")
	removed["oauth"] = map[string]any{}
	if gone := put(removed); gone == first || strings.Contains(revealed(t, w, r), "ETAG-REFRESH-T") {
		t.Fatalf("removing the grant left it, or its revision (%s)", gone)
	}
	if again := put(grantBody("ETAG-REFRESH-T")); again != first {
		t.Errorf("pasted, removed and pasted again: %s, then %s", first, again)
	}
	if other := put(grantBody("ETAG-REFRESH-U")); other == first {
		t.Error("another grant took the first one's revision")
	}
}
