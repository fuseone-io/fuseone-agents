package main

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/githubapp/githubapptest"
	"github.com/fuseone/agents/internal/settings"
)

/*
A change to what is sealed has to show in the read, or a client cannot tell that
somebody changed it (#268).

Names and addresses come back as themselves. A value that may be a secret never
does, and moves a fingerprint instead. Each row changes one thing and nothing else.
*/
type driftCase struct {
	name   string
	before map[string]any
	after  map[string]any
	// secrets must appear in neither answer.
	secrets []string
}

func mcpHTTP(fields map[string]any) map[string]any {
	out := map[string]any{"transport": "http", "url": "https://mcp.example.test/mcp"}
	for field, value := range fields {
		out[field] = value
	}
	return out
}

func mcpLocal(fields map[string]any) map[string]any {
	out := map[string]any{"transport": "stdio", "command": "drift-server", "acceptsLocalExecution": true}
	for field, value := range fields {
		out[field] = value
	}
	return out
}

func oauthGrant(over map[string]any) map[string]any {
	grant := map[string]any{
		"accessToken": "drift-access-secret", "refreshToken": "drift-refresh-secret",
		"clientSecret": "drift-client-secret", "tokenURL": "https://idp.example.test/token",
		"clientID": "drift-client", "tokenType": "Bearer", "scopes": []string{"read"},
	}
	for field, value := range over {
		grant[field] = value
	}
	return map[string]any{"oauth": grant}
}

func gitHubApp(key string) map[string]any {
	return map[string]any{"githubApp": map[string]any{"appId": 11, "installationId": 22, "privateKey": key}}
}

func headers(pairs map[string]string) map[string]any { return map[string]any{"headers": pairs} }

func TestReadBack_mcpServer_changingOneSealedThing_changesTheAnswer(t *testing.T) {
	world := newReadBackWorld(t)
	path := "/admin/integrations/mcp-servers/readback-drift"
	world.removeAtTheEnd(t, path)
	oauthSecrets := []string{"drift-access-secret", "drift-refresh-secret", "drift-client-secret"}
	keyA, keyB := githubapptest.New(t, 11, 22).PKCS1(), githubapptest.New(t, 11, 22).PKCS1()

	for _, c := range []driftCase{{
		name:    "a bearer token",
		before:  mcpHTTP(map[string]any{"token": "drift-token-a"}),
		after:   mcpHTTP(map[string]any{"token": "drift-token-b"}),
		secrets: []string{"drift-token-a", "drift-token-b"},
	}, {
		name:    "a header's name",
		before:  mcpHTTP(headers(map[string]string{"Api-Key": "drift-header-secret"})),
		after:   mcpHTTP(headers(map[string]string{"X-Api-Key": "drift-header-secret"})),
		secrets: []string{"drift-header-secret"},
	}, {
		name:    "a header's value",
		before:  mcpHTTP(headers(map[string]string{"Api-Key": "drift-header-a"})),
		after:   mcpHTTP(headers(map[string]string{"Api-Key": "drift-header-b"})),
		secrets: []string{"drift-header-a", "drift-header-b"},
	}, {
		name:    "the toolsets configuring a GitHub App",
		before:  mcpHTTP(merged(gitHubApp(keyA), headers(map[string]string{"X-MCP-Toolsets": "drift-repos"}))),
		after:   mcpHTTP(merged(gitHubApp(keyA), headers(map[string]string{"X-MCP-Toolsets": "drift-issues"}))),
		secrets: []string{"PRIVATE KEY", "drift-repos", "drift-issues"},
	}, {
		name:    "a GitHub App's private key",
		before:  mcpHTTP(gitHubApp(keyA)),
		after:   mcpHTTP(gitHubApp(keyB)),
		secrets: []string{"PRIVATE KEY"},
	}, {
		name:    "an OAuth token address",
		before:  mcpHTTP(oauthGrant(nil)),
		after:   mcpHTTP(oauthGrant(map[string]any{"tokenURL": "https://idp.example.test/other"})),
		secrets: oauthSecrets,
	}, {
		name:    "an OAuth client id",
		before:  mcpHTTP(oauthGrant(nil)),
		after:   mcpHTTP(oauthGrant(map[string]any{"clientID": "drift-other-client"})),
		secrets: oauthSecrets,
	}, {
		name:    "OAuth scopes",
		before:  mcpHTTP(oauthGrant(nil)),
		after:   mcpHTTP(oauthGrant(map[string]any{"scopes": []string{"read", "write"}})),
		secrets: oauthSecrets,
	}, {
		name:    "an OAuth token type",
		before:  mcpHTTP(oauthGrant(nil)),
		after:   mcpHTTP(oauthGrant(map[string]any{"tokenType": "Token"})),
		secrets: oauthSecrets,
	}, {
		name:    "an OAuth client secret",
		before:  mcpHTTP(oauthGrant(nil)),
		after:   mcpHTTP(oauthGrant(map[string]any{"clientSecret": "drift-client-other"})),
		secrets: append([]string{"drift-client-other"}, oauthSecrets...),
	}, {
		name:    "a variable's name",
		before:  mcpLocal(map[string]any{"env": map[string]string{"DRIFT_A": "drift-env-secret"}}),
		after:   mcpLocal(map[string]any{"env": map[string]string{"DRIFT_B": "drift-env-secret"}}),
		secrets: []string{"drift-env-secret"},
	}, {
		name:    "a variable's value",
		before:  mcpLocal(map[string]any{"env": map[string]string{"DRIFT_A": "drift-env-a"}}),
		after:   mcpLocal(map[string]any{"env": map[string]string{"DRIFT_A": "drift-env-b"}}),
		secrets: []string{"drift-env-a", "drift-env-b"},
	}, {
		name:    "a configuration file",
		before:  mcpLocal(map[string]any{"configFile": "drift-file-a"}),
		after:   mcpLocal(map[string]any{"configFile": "drift-file-b"}),
		secrets: []string{"drift-file-a", "drift-file-b"},
	}} {
		t.Run(c.name, func(t *testing.T) {
			if code, answer := world.send(t, http.MethodDelete, path, nil); code >= 300 {
				t.Fatalf("reset = %d: %s", code, answer)
			}
			world.put(t, path, c.before)
			first, rawFirst := world.get(t, path)
			world.put(t, path, c.after)
			second, rawSecond := world.get(t, path)
			if reflect.DeepEqual(withoutMeta(first), withoutMeta(second)) {
				t.Errorf("the answer did not move: %s", rawSecond)
			}
			for _, secret := range c.secrets {
				if strings.Contains(string(rawFirst)+string(rawSecond), secret) {
					t.Errorf("an answer carries %q: %s\n%s", secret, rawFirst, rawSecond)
				}
			}
		})
	}
}

func merged(parts ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, part := range parts {
		for field, value := range part {
			out[field] = value
		}
	}
	return out
}

// What is not secret comes back as itself, so a client compares it with what it
// wrote rather than only learning that something moved.
func TestReadBack_mcpServer_namesAndAddresses_comeBackAsThemselves(t *testing.T) {
	world := newReadBackWorld(t)
	remote := "/admin/integrations/mcp-servers/readback-names-remote"
	local := "/admin/integrations/mcp-servers/readback-names-local"
	world.removeAtTheEnd(t, remote)
	world.removeAtTheEnd(t, local)

	world.put(t, remote, mcpHTTP(headers(map[string]string{"X-B": "drift-b", "X-A": "drift-a"})))
	got, _ := world.get(t, remote)
	if want := []any{"X-A", "X-B"}; !reflect.DeepEqual(got["headerNames"], want) {
		t.Errorf("headerNames = %v, want %v", got["headerNames"], want)
	}

	world.put(t, remote, mcpHTTP(oauthGrant(nil)))
	got, _ = world.get(t, remote)
	want := map[string]any{
		"tokenURL": "https://idp.example.test/token", "clientID": "drift-client",
		"tokenType": "Bearer", "scopes": []any{"read"},
	}
	if !reflect.DeepEqual(got["oauthClient"], want) {
		t.Errorf("oauthClient = %v, want %v", got["oauthClient"], want)
	}

	world.put(t, local, mcpLocal(map[string]any{"env": map[string]string{"DRIFT_B": "x", "DRIFT_A": "y"}}))
	got, _ = world.get(t, local)
	if want := []any{"DRIFT_A", "DRIFT_B"}; !reflect.DeepEqual(got["envNames"], want) {
		t.Errorf("envNames = %v, want %v", got["envNames"], want)
	}
}

// A token address is the one shown value somebody can paste a credential into.
// Shown without those parts; a change to them still moves the fingerprint.
func TestReadBack_mcpServer_tokenAddressCarryingACredential_showsItWithout(t *testing.T) {
	world := newReadBackWorld(t)
	path := "/admin/integrations/mcp-servers/readback-address"
	world.removeAtTheEnd(t, path)

	address := func(secret string) map[string]any {
		return mcpHTTP(oauthGrant(map[string]any{
			"tokenURL": "https://drift-user:" + secret + "@idp.example.test/token?key=" + secret + "#" + secret,
		}))
	}
	world.put(t, path, address("drift-url-secret-a"))
	first, raw := world.get(t, path)
	if strings.Contains(string(raw), "drift-url-secret-a") || strings.Contains(string(raw), "drift-user") {
		t.Errorf("the address came back with its credential: %s", raw)
	}
	client, _ := first["oauthClient"].(map[string]any)
	if client["tokenURL"] != "https://idp.example.test/token" {
		t.Errorf("tokenURL = %v", client["tokenURL"])
	}

	world.put(t, path, address("drift-url-secret-b"))
	second, _ := world.get(t, path)
	if first["secretFingerprint"] == nil || first["secretFingerprint"] == second["secretFingerprint"] {
		t.Errorf("fingerprint %v then %v, want it to move", first["secretFingerprint"], second["secretFingerprint"])
	}
}

// A worker refreshes a grant on its own. Read as drift, every server on OAuth
// would change hourly with nobody having written anything.
func TestReadBack_mcpServer_grantRotatedByAWorker_answerHoldsStill(t *testing.T) {
	world := newReadBackWorld(t)
	path := "/admin/integrations/mcp-servers/readback-rotation"
	world.removeAtTheEnd(t, path)

	world.put(t, path, mcpHTTP(oauthGrant(nil)))
	first, _ := world.get(t, path)
	world.put(t, path, mcpHTTP(oauthGrant(map[string]any{
		"accessToken": "drift-access-rotated", "refreshToken": "drift-refresh-rotated", "expiresAtUnix": 4102444800,
	})))
	second, raw := world.get(t, path)
	if !reflect.DeepEqual(withoutMeta(first), withoutMeta(second)) {
		t.Errorf("rotation read as drift\n before: %v\n  after: %v", first, second)
	}
	if strings.Contains(string(raw), "drift-access-rotated") {
		t.Errorf("the answer carries the token: %s", raw)
	}
}

// Tied to the resource, so nobody can seal a guess on a server of their own and
// compare: confirming a guess would mean replacing the secret they are guessing.
func TestReadBack_mcpServer_sameSecretOnAnotherServer_differentFingerprint(t *testing.T) {
	world := newReadBackWorld(t)
	one, other := "/admin/integrations/mcp-servers/readback-one", "/admin/integrations/mcp-servers/readback-other"
	world.removeAtTheEnd(t, one)
	world.removeAtTheEnd(t, other)
	world.put(t, one, mcpHTTP(map[string]any{"token": "drift-shared"}))
	world.put(t, other, mcpHTTP(map[string]any{"token": "drift-shared"}))

	a, _ := world.get(t, one)
	b, _ := world.get(t, other)
	if a["secretFingerprint"] == nil || a["secretFingerprint"] == b["secretFingerprint"] {
		t.Errorf("fingerprints %v and %v, want two different ones", a["secretFingerprint"], b["secretFingerprint"])
	}
}

// A row stored before the document existed holds the bare token and no read-model
// hints, and nothing migrates it. It reads as the document it would become, so the
// next unrelated save does not look like somebody changed the credential.
func TestReadBack_mcpServer_rowStoredBeforeTheDocument_readsAsItWouldAfter(t *testing.T) {
	world := newReadBackWorld(t)
	path := "/admin/integrations/mcp-servers/readback-legacy"
	world.removeAtTheEnd(t, path)

	if err := world.store.Put(t.Context(), settings.Setting{
		ScopeKind: settings.ScopeInstallation, Scope: domain.Scope{},
		Kind: settings.KindMCPServer, Name: "readback-legacy",
		Value:  []byte(`{"transport":"http","url":"https://mcp.example.test/mcp"}`),
		Secret: "drift-bare-token", Enabled: true,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	stored, raw := world.get(t, path)
	if strings.Contains(string(raw), "drift-bare-token") {
		t.Fatalf("the answer carries the token: %s", raw)
	}
	if stored["secretFingerprint"] == nil || stored["hasSecret"] != true {
		t.Fatalf("a stored token reads as %s", raw)
	}

	world.put(t, path, stored)
	after, _ := world.get(t, path)
	if !reflect.DeepEqual(withoutMeta(stored), withoutMeta(after)) {
		t.Errorf("re-sealing the row moved the answer\n before: %v\n  after: %v", stored, after)
	}
	row, err := world.store.Reveal(t.Context(), settings.ScopeInstallation, domain.Scope{},
		settings.KindMCPServer, "readback-legacy")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if !strings.HasPrefix(row.Secret, "{") {
		t.Fatalf("nothing re-sealed the row as a document, so this proves nothing: %q", row.Secret)
	}
}

// Nothing the fingerprint or the names need is stored outside the seal.
func TestReadBack_mcpServer_namesAreNotCopiedOutOfTheSeal(t *testing.T) {
	world := newReadBackWorld(t)
	path := "/admin/integrations/mcp-servers/readback-seal"
	world.removeAtTheEnd(t, path)
	world.put(t, path, mcpHTTP(merged(oauthGrant(nil), headers(map[string]string{"X-Drift-Name": "v"}))))
	row, err := world.store.Get(t.Context(), settings.ScopeInstallation, domain.Scope{},
		settings.KindMCPServer, "readback-seal")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	for _, sealed := range []string{"X-Drift-Name", "drift-client", "idp.example.test"} {
		if strings.Contains(string(row.Value), sealed) {
			t.Errorf("%q left the seal: %s", sealed, row.Value)
		}
	}
}
