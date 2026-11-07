package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

// managed is one resource a declarative client manages, stored as a settings row.
type managed struct {
	name string
	path string
	// key names the row, as a test has to find it.
	row settings.Setting
	// body makes the resource with this secret, which "" leaves out.
	body func(secret string) map[string]any
	// change is a real change of configuration on a body.
	change func(body map[string]any)
	// secretField holds the secret in a body.
	secretField string
}

func managedResources() []managed {
	installation := func(kind settings.Kind, name string) settings.Setting {
		return settings.Setting{ScopeKind: settings.ScopeInstallation, Kind: kind, Name: name}
	}
	optional := func(body map[string]any, field, secret string) map[string]any {
		if secret != "" {
			body[field] = secret
		}
		return body
	}
	return []managed{{
		name: "tool server", path: "/admin/integrations/mcp-servers/etag-mcp",
		row: installation(settings.KindMCPServer, "etag-mcp"), secretField: "token",
		body: func(secret string) map[string]any {
			return optional(map[string]any{"transport": "http", "url": "https://mcp.example.com/a", "surface": []string{"search"}}, "token", secret)
		},
		change: func(b map[string]any) { b["url"] = "https://mcp.example.com/b" },
	}, {
		name: "model provider", path: "/admin/integrations/providers/etag-provider",
		row: installation(settings.KindModelProvider, "etag-provider"), secretField: "apiKey",
		body: func(secret string) map[string]any {
			return optional(map[string]any{"kind": "openai_compatible", "baseUrl": "https://llm.example.com/v1", "models": []string{"m1"}, "enabled": true}, "apiKey", secret)
		},
		change: func(b map[string]any) { b["models"] = []string{"m1", "m2"} },
	}, {
		name: "channel", path: "/admin/channels/etag-channel",
		row: installation("channel", "etag-channel"), secretField: "token",
		body: func(secret string) map[string]any {
			return optional(map[string]any{"kind": "slack", "workspace": "acme", "signingSecret": "ETAG-SIGNING"}, "token", secret)
		},
		change: func(b map[string]any) { b["workspace"] = "acme two" },
	}, {
		name: "connector instance", path: "/admin/integrations/connectors/instances/etag-vault?scopeKind=area&company=acme&area=cyber",
		row: settings.Setting{
			ScopeKind: settings.ScopeArea, Scope: domain.Scope{Company: "acme", Area: "cyber"},
			Kind: settings.KindConnectorInstance, Name: "etag-vault",
		},
		secretField: "token",
		body: func(secret string) map[string]any {
			return optional(map[string]any{
				"connector": "vault", "scopeKind": "area", "company": "acme", "area": "cyber",
				"vault": map[string]any{"address": "https://vault.example.com", "mount": "kv", "allowedPathPrefixes": []string{"apps/"}},
			}, "token", secret)
		},
		change: func(b map[string]any) {
			b["vault"] = map[string]any{"address": "https://vault.example.com", "mount": "kv", "allowedPathPrefixes": []string{"other/"}}
		},
	}}
}

// held is everything an identical write must leave alone: who and when, the ciphertext and
// its nonce, so nothing was sealed again, and the trail.
func held(t *testing.T, r managed) string {
	t.Helper()
	var state string
	err := testdb.Pool(t).QueryRow(context.Background(), `
		select s.updated_at::text || '|' || s.updated_by || '|' ||
		       coalesce(encode(s.secret, 'hex'), '') || '|' || coalesce(encode(s.secret_nonce, 'hex'), '') || '|' ||
		       (select count(*) from admin_events where target = $5)::text
		from settings s
		where s.scope_kind = $1 and s.company_id = $2 and s.area_id = $3 and s.kind = $4 and s.name = $5`,
		string(r.row.ScopeKind), string(r.row.Scope.Company), string(r.row.Scope.Area), string(r.row.Kind), r.row.Name,
	).Scan(&state)
	if err != nil {
		t.Fatalf("read the row: %v", err)
	}
	return state
}

func sealedNonce(t *testing.T, r managed) string {
	t.Helper()
	var nonce string
	if err := testdb.Pool(t).QueryRow(context.Background(), `
		select encode(secret_nonce, 'hex') from settings
		where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5`,
		string(r.row.ScopeKind), string(r.row.Scope.Company), string(r.row.Scope.Area), string(r.row.Kind), r.row.Name,
	).Scan(&nonce); err != nil {
		t.Fatalf("read the nonce: %v", err)
	}
	return nonce
}

// stored writes the resource and removes it afterwards.
func stored(t *testing.T, w *readBackWorld, r managed, secret string) (etag string) {
	t.Helper()
	t.Cleanup(func() { w.call(t, etagCall{method: http.MethodDelete, path: r.path}) })
	put := w.call(t, etagCall{method: http.MethodPut, path: r.path, body: r.body(secret)})
	if put.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d: %s", put.Code, put.Body)
	}
	return put.Header().Get("ETag")
}

func TestSettingsETag_anIdenticalPut_changesNothing(t *testing.T) {
	w := newReadBackWorld(t)
	for _, r := range managedResources() {
		t.Run(r.name, func(t *testing.T) {
			etag := stored(t, w, r, "ETAG-SECRET-ONE")
			if !strings.HasPrefix(etag, `"v1-`) {
				t.Fatalf("ETag = %q", etag)
			}
			if got := w.call(t, etagCall{method: http.MethodGet, path: r.path}); got.Code != http.StatusOK || got.Header().Get("ETag") != etag {
				t.Fatalf("GET = %d with ETag %s, PUT said %s: %s", got.Code, got.Header().Get("ETag"), etag, got.Body)
			}
			before := held(t, r)

			for name, body := range map[string]map[string]any{
				"the secret again":    r.body("ETAG-SECRET-ONE"),
				"the secret left out": r.body(""),
			} {
				// Somebody else, so a write would show in updated_by too.
				again := w.call(t, etagCall{
					method: http.MethodPut, path: r.path, ifMatch: []string{etag}, body: body, by: "usr_ben",
				})
				if again.Code != http.StatusNoContent || again.Header().Get("ETag") != etag {
					t.Errorf("%s = %d with ETag %s, want %s: %s", name, again.Code, again.Header().Get("ETag"), etag, again.Body)
				}
				if after := held(t, r); after != before {
					t.Errorf("%s moved the row:\n %s\n %s", name, before, after)
				}
			}
		})
	}
}

func TestSettingsETag_aRealChange_movesItAndIsOnTheTrail(t *testing.T) {
	w := newReadBackWorld(t)
	for _, r := range managedResources() {
		t.Run(r.name, func(t *testing.T) {
			etag := stored(t, w, r, "ETAG-SECRET-ONE")
			before := held(t, r)

			body := r.body("")
			r.change(body)
			changed := w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{etag}, body: body, by: "usr_ben"})
			if changed.Code != http.StatusNoContent || changed.Header().Get("ETag") == etag {
				t.Fatalf("change = %d with ETag %s: %s", changed.Code, changed.Header().Get("ETag"), changed.Body)
			}
			if held(t, r) == before {
				t.Error("the row did not move")
			}
			if got := w.call(t, etagCall{method: http.MethodGet, path: r.path}); got.Header().Get("ETag") != changed.Header().Get("ETag") {
				t.Errorf("GET says %s, PUT said %s", got.Header().Get("ETag"), changed.Header().Get("ETag"))
			}

			// What it says, not when it was written: back to what it said, back to its name.
			// With the secret sent again, so it is sealed again under a new nonce, which
			// is what has to leave the fingerprint alone.
			sealedBefore := sealedNonce(t, r)
			back := w.call(t, etagCall{method: http.MethodPut, path: r.path, body: r.body("ETAG-SECRET-ONE")})
			if back.Header().Get("ETag") != etag {
				t.Errorf("the first configuration again = %s, was %s", back.Header().Get("ETag"), etag)
			}
			if sealedNonce(t, r) == sealedBefore {
				t.Error("nothing was sealed again, so this proves nothing about the fingerprint")
			}
		})
	}
}

func TestSettingsETag_anotherSecret_movesItWithoutSayingWhatItIs(t *testing.T) {
	w := newReadBackWorld(t)
	for _, r := range managedResources() {
		t.Run(r.name, func(t *testing.T) {
			etag := stored(t, w, r, "ETAG-SECRET-ONE")
			before := held(t, r)

			put := w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{etag}, body: r.body("ETAG-SECRET-TWO")})
			if put.Code != http.StatusNoContent || put.Header().Get("ETag") == etag {
				t.Fatalf("another secret = %d with ETag %s", put.Code, put.Header().Get("ETag"))
			}
			if held(t, r) == before {
				t.Error("the row did not move")
			}
			got := w.call(t, etagCall{method: http.MethodGet, path: r.path})
			for _, text := range []string{got.Body.String(), got.Header().Get("ETag"), put.Header().Get("ETag")} {
				if strings.Contains(text, "ETAG-SECRET") {
					t.Errorf("a response carries the secret: %s", text)
				}
			}
		})
	}
}

func TestSettingsETag_aStaleWrite_isRefusedAndMovesNothing(t *testing.T) {
	w := newReadBackWorld(t)
	for _, r := range managedResources() {
		t.Run(r.name, func(t *testing.T) {
			etag := stored(t, w, r, "ETAG-SECRET-ONE")
			before := held(t, r)
			changed := r.body("ETAG-SECRET-TWO")
			r.change(changed)

			for name, call := range map[string]etagCall{
				"changing":  {method: http.MethodPut, path: r.path, ifMatch: []string{`"v1-old"`}, body: changed},
				"identical": {method: http.MethodPut, path: r.path, ifMatch: []string{`"v1-old"`}, body: r.body("")},
				"removing":  {method: http.MethodDelete, path: r.path, ifMatch: []string{`"v1-old"`}},
			} {
				got := w.call(t, call)
				if got.Code != http.StatusPreconditionFailed || got.Header().Get("ETag") != etag {
					t.Errorf("%s = %d with ETag %s, want 412 with %s: %s", name, got.Code, got.Header().Get("ETag"), etag, got.Body)
				}
				if !strings.Contains(got.Body.String(), `"fuseone:precondition-failed"`) {
					t.Errorf("%s body = %s", name, got.Body)
				}
			}
			for name, call := range map[string]etagCall{
				"writing":  {method: http.MethodPut, path: r.path, ifMatch: []string{`v1-old`}, body: changed},
				"removing": {method: http.MethodDelete, path: r.path, ifMatch: []string{`v1-old`}},
			} {
				if got := w.call(t, call); got.Code != http.StatusBadRequest {
					t.Errorf("malformed, %s = %d: %s", name, got.Code, got.Body)
				}
			}
			if after := held(t, r); after != before {
				t.Errorf("a refused write moved the row:\n %s\n %s", before, after)
			}

			if got := w.call(t, etagCall{method: http.MethodDelete, path: r.path, ifMatch: []string{etag}}); got.Code != http.StatusNoContent {
				t.Fatalf("a current delete = %d: %s", got.Code, got.Body)
			}
			if got := w.call(t, etagCall{method: http.MethodDelete, path: r.path, ifMatch: []string{`*`}}); got.Code != http.StatusPreconditionFailed {
				t.Errorf("deleting nothing with * = %d", got.Code)
			}
			if got := w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{`*`}, body: r.body("x")}); got.Code != http.StatusPreconditionFailed {
				t.Errorf("writing over nothing with * = %d", got.Code)
			}
			if got := w.call(t, etagCall{method: http.MethodGet, path: r.path}); got.Code != http.StatusNotFound {
				t.Errorf("it was created: %d", got.Code)
			}
		})
	}
}

// What a refusal can reveal, in order: somebody with no authority learns nothing about the
// resource from If-Match, and a request that could never be stored is told so.
func TestSettingsETag_ifMatch_isJudgedAfterAuthorityAndValidation(t *testing.T) {
	w := newReadBackWorld(t)
	for _, r := range managedResources() {
		t.Run(r.name, func(t *testing.T) {
			stored(t, w, r, "ETAG-SECRET-ONE")
			got := w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{`"v1-old"`}, body: r.body(""), role: domain.RoleAuditor})
			if got.Code != http.StatusForbidden || got.Header().Get("ETag") != "" {
				t.Errorf("no authority = %d with ETag %q", got.Code, got.Header().Get("ETag"))
			}
		})
	}
	mcp := managedResources()[0]
	stored(t, w, mcp, "ETAG-SECRET-ONE")
	invalid := w.call(t, etagCall{method: http.MethodPut, path: mcp.path, ifMatch: []string{`"v1-old"`}, body: map[string]any{"transport": "http"}})
	if invalid.Code != http.StatusBadRequest {
		t.Errorf("an unstorable server = %d, want 400: %s", invalid.Code, invalid.Body)
	}
}

func TestSettingsETag_twoWritersWithOneETag_exactlyOneChangesIt(t *testing.T) {
	seeding := newReadBackWorld(t)
	for _, r := range managedResources() {
		t.Run(r.name, func(t *testing.T) {
			first := stored(t, seeding, r, "ETAG-SECRET-ONE")
			const writers = 4
			contention := testdb.NewContention(t, writers)
			w := readBackWorldOn(t, contention.Pool)

			codes := make([]int, writers)
			writes := make([]func() error, writers)
			for i := range writes {
				writes[i] = func() error {
					body := r.body("")
					r.change(body)
					body[r.secretField] = "ETAG-WRITER-" + string(rune('A'+i))
					codes[i] = w.call(t, etagCall{method: http.MethodPut, path: r.path, ifMatch: []string{first}, body: body}).Code
					return nil
				}
			}
			contention.Overlap(t, `select 1 from settings
				where scope_kind = $1 and company_id = $2 and area_id = $3 and kind = $4 and name = $5 for update`,
				[]any{string(r.row.ScopeKind), string(r.row.Scope.Company), string(r.row.Scope.Area), string(r.row.Kind), r.row.Name}, writes)

			won, refused := 0, 0
			for _, code := range codes {
				switch code {
				case http.StatusNoContent:
					won++
				case http.StatusPreconditionFailed:
					refused++
				default:
					t.Fatalf("a writer got %d", code)
				}
			}
			if won != 1 || refused != writers-1 {
				t.Fatalf("codes = %v", codes)
			}
		})
	}
}

// With no row there is nothing for a row lock to hold, and a second creator would fold onto
// nothing and drop what the first chose. The hold here is the guard itself.
func TestSettingsWrite_twoCreators_theSecondFoldsOntoTheFirst(t *testing.T) {
	seeding := newReadBackWorld(t)
	r := managedResources()[0]
	t.Cleanup(func() { seeding.call(t, etagCall{method: http.MethodDelete, path: r.path}) })
	contention := testdb.NewContention(t, 2)
	w := readBackWorldOn(t, contention.Pool)

	narrowing := map[string]any{"transport": "http", "url": "https://mcp.example.com/a", "surface": []string{"search"}}
	omitting := map[string]any{"transport": "http", "url": "https://mcp.example.com/a"}
	var codes [2]int
	contention.Overlap(t, `select pg_advisory_xact_lock(hashtextextended($1, 0)), 1`, []any{settings.GuardKey(r.row)}, []func() error{
		func() error {
			codes[0] = w.call(t, etagCall{method: http.MethodPut, path: r.path, body: narrowing}).Code
			return nil
		},
		func() error {
			codes[1] = w.call(t, etagCall{method: http.MethodPut, path: r.path, body: omitting}).Code
			return nil
		},
	})
	if codes != [2]int{http.StatusNoContent, http.StatusNoContent} {
		t.Fatalf("codes = %v", codes)
	}

	var got struct {
		Surface []string `json:"surface"`
	}
	read := seeding.call(t, etagCall{method: http.MethodGet, path: r.path})
	if err := json.Unmarshal(read.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Surface) != 1 {
		t.Fatalf("surface = %v: omitting it was meant to keep what the other chose", got.Surface)
	}
}

// A provider key or a connector token sealed under a lost master key could always be
// replaced, because those writes never needed it. Comparing must not take that away.
func TestSettingsWrite_aSecretThatNoLongerOpens_canStillBeReplaced(t *testing.T) {
	sealer := newReadBackWorld(t)
	other, err := vault.New(append(make([]byte, 31), 7), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	pool := testdb.Pool(t)
	store := settings.NewStore(pool, other)
	opened := &readBackWorld{route: apiHandlerFor(httpapi.NewServer(ledger.NewMemory(), "test").
		WithAdministration(admin.NewCurator(pool), nil, admin.NewIntegrations(pool, store)).
		WithConnectorInstances(admin.NewConnectorInstances(pool, store))), store: store}

	for _, r := range []managed{managedResources()[1], managedResources()[3]} {
		t.Run(r.name, func(t *testing.T) {
			stored(t, sealer, r, "ETAG-SECRET-ONE")

			if r.name == "connector instance" {
				if got := opened.call(t, etagCall{method: http.MethodGet, path: r.path}); got.Code != http.StatusOK || got.Header().Get("ETag") == "" {
					t.Fatalf("reading a declaration = %d with ETag %q: %s", got.Code, got.Header().Get("ETag"), got.Body)
				}
			}
			if got := opened.call(t, etagCall{method: http.MethodPut, path: r.path, body: r.body("ETAG-SECRET-NEW")}); got.Code != http.StatusNoContent {
				t.Fatalf("replacing it = %d: %s", got.Code, got.Body)
			}
			row, err := store.Reveal(t.Context(), r.row.ScopeKind, r.row.Scope, r.row.Kind, r.row.Name)
			if err != nil || row.Secret != "ETAG-SECRET-NEW" {
				t.Fatalf("Reveal = %q, %v", row.Secret, err)
			}
		})
	}
}
