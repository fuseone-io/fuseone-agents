package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/githubapp/githubapptest"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/vault"
)

/*
What a declarative client does to a resource it manages (#268): PUT it, GET it,
PUT what GET answered, and expect that to have changed nothing.

Through the router serve mounts and the stores serve builds, because a field
dies in the conversions: a handler test against a fake holds the right value in
the fake, and passes while the real row has lost it. A difference a client
cannot explain reads to it as permanent drift, so each case names every field
GET answers differently from what PUT was given.
*/
type readBack struct {
	name string
	// path is the resource, under the API's base.
	path string
	// sealed is the settings kind holding the row's secrets. Empty for a case
	// that stores none.
	sealed settings.Kind
	// key is the row's name in that kind.
	key  string
	body map[string]any
	// writeOnly is every accepted field GET cannot return, and the field that
	// reports one is stored.
	writeOnly map[string]string
	// normalised is every accepted field stored as something other than what
	// was sent, and what GET answers instead. Nil means GET leaves it out.
	normalised map[string]any
	// secrets must appear in no response and survive the second PUT.
	secrets []string
}

// readBackMeta moves on every write, by design: it says who last wrote and when,
// not what the resource is. Whether an unchanged write should move it is for
// the slice that introduces ETag; everything else here has to hold still.
var readBackMeta = []string{"updatedAt", "updatedBy"}

type readBackWorld struct {
	route http.Handler
	store *settings.Store
}

func newReadBackWorld(t *testing.T) *readBackWorld {
	t.Helper()
	pool := testdb.Pool(t)
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	store := settings.NewStore(pool, v)
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAdministration(admin.NewCurator(pool), nil, admin.NewIntegrations(pool, store)).
		WithChannels(admin.NewChannels(pool, store, readBackDrivers{}), nil).
		WithPolicies(policy.NewStore(pool))
	return &readBackWorld{route: apiHandlerFor(api), store: store}
}

type readBackDrivers struct{}

func (readBackDrivers) Kinds() []string { return []string{"slack"} }

// send acts as somebody who holds every authority these writes can ask for:
// this walks what the resource does, and who may do it is held in httpapi.
func (w *readBackWorld) send(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequestWithContext(t.Context(), method, "/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: domain.Installation}, Role: domain.RoleAdmin}},
	}))
	rec := httptest.NewRecorder()
	w.route.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func (w *readBackWorld) put(t *testing.T, path string, body any) {
	t.Helper()
	if code, answer := w.send(t, http.MethodPut, path, body); code >= 300 {
		t.Fatalf("PUT %s = %d: %s", path, code, answer)
	}
}

func (w *readBackWorld) get(t *testing.T, path string) (map[string]any, []byte) {
	t.Helper()
	code, raw := w.send(t, http.MethodGet, path, nil)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, code, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out, raw
}

// sealedHalf opens the row's secrets. Empty for a case that stores none.
func (w *readBackWorld) sealedHalf(t *testing.T, c readBack) string {
	t.Helper()
	if c.sealed == "" {
		return ""
	}
	row, err := w.store.Reveal(t.Context(), settings.ScopeInstallation, domain.Scope{}, c.sealed, c.key)
	if err != nil {
		t.Fatalf("reveal %s: %v", c.key, err)
	}
	return row.Secret
}

func (w *readBackWorld) walk(t *testing.T, c readBack) {
	t.Helper()
	t.Cleanup(func() { w.send(t, http.MethodDelete, c.path, nil) })

	w.put(t, c.path, c.body)
	first, raw := w.get(t, c.path)
	assertReadsBackWhatWasAccepted(t, c, first)
	assertNoSecretIn(t, c, raw)
	assertNothingIsNull(t, first)

	sealed := w.sealedHalf(t, c)
	for _, secret := range c.secrets {
		if !strings.Contains(sealed, secret) {
			t.Fatalf("%q was never stored, so nothing here proves it survives", secret)
		}
	}

	w.put(t, c.path, first)
	second, raw := w.get(t, c.path)
	assertNoSecretIn(t, c, raw)
	if a, b := withoutMeta(first), withoutMeta(second); !reflect.DeepEqual(a, b) {
		t.Errorf("putting back what GET answered changed it\n before: %v\n  after: %v", a, b)
	}
	if after := w.sealedHalf(t, c); after != sealed {
		t.Error("putting back what GET answered changed the stored secrets")
	}

	if code, answer := w.send(t, http.MethodDelete, c.path, nil); code >= 300 {
		t.Fatalf("DELETE %s = %d: %s", c.path, code, answer)
	}
	if code, answer := w.send(t, http.MethodGet, c.path, nil); code != http.StatusNotFound {
		t.Errorf("GET after DELETE = %d: %s; want 404", code, answer)
	}
}

func assertReadsBackWhatWasAccepted(t *testing.T, c readBack, got map[string]any) {
	t.Helper()
	for field, want := range roundTrip(t, c.body) {
		if presence, secret := c.writeOnly[field]; secret {
			if _, leaked := got[field]; leaked {
				t.Errorf("%s is write-only and GET returned it", field)
			}
			if got[presence] != true {
				t.Errorf("%s was stored and %s = %v, want true", field, presence, got[presence])
			}
			continue
		}
		if stored, differs := c.normalised[field]; differs {
			want = roundTrip(t, map[string]any{field: stored})[field]
		}
		if have := got[field]; !reflect.DeepEqual(have, want) {
			t.Errorf("%s: GET = %#v, want %#v", field, have, want)
		}
	}
}

func assertNoSecretIn(t *testing.T, c readBack, raw []byte) {
	t.Helper()
	for _, secret := range c.secrets {
		if strings.Contains(string(raw), secret) {
			t.Errorf("a response carries %q: %s", secret, raw)
		}
	}
}

// assertEveryAcceptedFieldIsWalked reads the contract rather than trusting the
// table: a field added to a PUT later has to be walked here, or written down as
// one GET cannot return, before this passes. That is the moment somebody decides
// whether a client can read it back.
func assertEveryAcceptedFieldIsWalked(t *testing.T, path string, cases []readBack) {
	t.Helper()
	spec, err := openapi.GetSwagger()
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	item := spec.Paths.Find(path)
	if item == nil || item.Put == nil {
		t.Fatalf("the contract has no PUT %s", path)
	}
	body := item.Put.RequestBody.Value.Content.Get("application/json").Schema.Value
	if len(body.Properties) == 0 {
		t.Fatalf("PUT %s declares no fields, so nothing here checks anything", path)
	}
	walked := map[string]bool{}
	for _, c := range cases {
		for field := range c.body {
			walked[field] = true
		}
	}
	for field := range body.Properties {
		if !walked[field] {
			t.Errorf("PUT %s accepts %s and no case walks it", path, field)
		}
	}
}

// A field is present or absent. The schema has no null for a client to compare
// with, and decoding cannot tell the two apart in the assertions above.
func assertNothingIsNull(t *testing.T, got map[string]any) {
	t.Helper()
	for field, value := range got {
		if value == nil {
			t.Errorf("%s is null", field)
		}
	}
}

func withoutMeta(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for field, value := range in {
		out[field] = value
	}
	for _, field := range readBackMeta {
		delete(out, field)
	}
	return out
}

// roundTrip gives a literal the types decoding gives a response.
func roundTrip(t *testing.T, in map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestReadBack_mcpServer_putWhatGetAnsweredChangesNothing(t *testing.T) {
	world := newReadBackWorld(t)
	const base = "/admin/integrations/mcp-servers/"
	cases := []readBack{{
		name: "remote with a bearer token and every tuning field",
		path: base + "readback-remote", sealed: settings.KindMCPServer, key: "readback-remote",
		body: map[string]any{
			"transport": "http", "url": "https://mcp.example.test/mcp", "protocolMode": "legacy",
			"token":              "readback-bearer-secret",
			"surface":            []string{"search", "fetch"},
			"rateLimit":          map[string]any{"ratePerSecond": 2.5, "burst": 5},
			"callTimeoutSeconds": 90,
			"cache":              map[string]any{"ttlSeconds": 30, "maxEntries": 10},
			"enabled":            false,
		},
		writeOnly: map[string]string{"token": "hasSecret"},
		secrets:   []string{"readback-bearer-secret"},
	}, {
		name: "remote with headers and a surface chosen as empty",
		path: base + "readback-headers", sealed: settings.KindMCPServer, key: "readback-headers",
		body: map[string]any{
			"transport": "http", "url": "https://mcp.example.test/mcp",
			"headers": map[string]string{"Api-Key": "readback-header-secret"},
			"surface": []string{},
		},
		writeOnly: map[string]string{"headers": "hasSecret"},
		secrets:   []string{"readback-header-secret"},
	}, {
		name: "remote with an OAuth grant",
		path: base + "readback-oauth", sealed: settings.KindMCPServer, key: "readback-oauth",
		body: map[string]any{
			"transport": "http", "url": "https://mcp.example.test/mcp",
			"oauth": map[string]any{
				"accessToken": "readback-access-secret", "refreshToken": "readback-refresh-secret",
				"clientSecret": "readback-client-secret", "tokenURL": "https://idp.example.test/token",
				"clientID": "readback-client", "scopes": []string{"read"},
			},
		},
		writeOnly: map[string]string{"oauth": "hasOAuth"},
		secrets:   []string{"readback-access-secret", "readback-refresh-secret", "readback-client-secret"},
	}, {
		name: "remote as a GitHub App, with the headers that configure it",
		path: base + "readback-app", sealed: settings.KindMCPServer, key: "readback-app",
		body: map[string]any{
			"transport": "http", "url": "https://mcp.example.test/mcp",
			"headers": map[string]string{"X-MCP-Toolsets": "readback-toolset"},
			"githubApp": map[string]any{
				"appId": 11, "installationId": 22, "privateKey": githubapptest.New(t, 11, 22).PKCS1(),
				"repositories": []string{"one", "two"}, "apiBaseURL": "https://ghe.example.test/api/v3",
			},
		},
		writeOnly: map[string]string{"headers": "hasSecret"},
		normalised: map[string]any{"githubApp": map[string]any{
			"appId": 11, "installationId": 22, "hasPrivateKey": true,
			"repositories": []string{"one", "two"}, "apiBaseURL": "https://ghe.example.test/api/v3",
		}},
		secrets: []string{"PRIVATE KEY", "readback-toolset"},
	}, {
		name: "local process with variables, a file and proxied egress",
		path: base + "readback-local", sealed: settings.KindMCPServer, key: "readback-local",
		body: map[string]any{
			"transport": "stdio", "command": "readback-server", "args": []string{"--stdio", "--verbose"},
			"acceptsLocalExecution": true,
			"env":                   map[string]string{"READBACK_KEY": "readback-env-secret"},
			"configFile":            "readback-file-secret",
			"configFileEnv":         "READBACK_CONFIG",
			"stdioEgress": map[string]any{
				"mode": "proxied",
				"allowedDestinations": []map[string]any{
					{"host": "Api.Example.Test", "port": 443},
					{"host": "api.example.test", "port": 443},
				},
			},
		},
		writeOnly: map[string]string{"env": "hasVariables", "configFile": "hasConfigFile"},
		// A hostname is stored in one spelling, once.
		normalised: map[string]any{"stdioEgress": map[string]any{
			"mode":                "proxied",
			"allowedDestinations": []map[string]any{{"host": "api.example.test", "port": 443}},
		}},
		secrets: []string{"readback-env-secret", "readback-file-secret"},
	}, {
		name: "the fewest fields a local process can take",
		path: base + "readback-bare",
		body: map[string]any{"command": "readback-server", "acceptsLocalExecution": true},
	}, {
		name: "fields that switch something off, which GET then leaves out",
		path: base + "readback-off",
		body: map[string]any{
			"transport": "stdio", "command": "readback-server", "acceptsLocalExecution": true,
			"rateLimit":          map[string]any{"ratePerSecond": 0, "burst": 0},
			"cache":              map[string]any{"ttlSeconds": 0, "maxEntries": 0},
			"callTimeoutSeconds": 0,
			"stdioEgress":        map[string]any{"mode": "inherit"},
			"args":               []string{},
		},
		normalised: map[string]any{
			"rateLimit": nil, "cache": nil, "callTimeoutSeconds": nil, "stdioEgress": nil, "args": nil,
		},
	}, {
		name: "a cache given no size gets the default one",
		path: base + "readback-cache",
		body: map[string]any{
			"transport": "http", "url": "https://mcp.example.test/mcp",
			"cache": map[string]any{"ttlSeconds": 30},
		},
		normalised: map[string]any{"cache": map[string]any{"ttlSeconds": 30, "maxEntries": 256}},
	}}
	assertEveryAcceptedFieldIsWalked(t, base+"{name}", cases)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { world.walk(t, c) })
	}
}
