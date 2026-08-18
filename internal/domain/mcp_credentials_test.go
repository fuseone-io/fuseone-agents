package domain_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
What a tool server is given, and what a write leaves alone.

The document is a credential in every shape. A token is obviously one; a
variable or config file is one often enough, because the reason a server needs
either is usually that it contains a key, DSN or grant.
*/

// An installation configured before this existed holds a bare token. Refusing
// to read one would take a working server away to add a field it does not use.
func TestReadMCPCredentials_aBareToken_isReadAsTheToken(t *testing.T) {
	t.Parallel()

	got := domain.ReadMCPCredentials("ghp_something")
	if got.Token != "ghp_something" || len(got.Env) != 0 {
		t.Errorf("got %+v, want the stored string read as the bearer", got)
	}
}

func TestMCPCredentials_survivesTheVaultAndComesBackWhole(t *testing.T) {
	t.Parallel()

	sealed := domain.MCPCredentials{
		Token:      "ghp_something",
		Headers:    map[string]string{"Api-Key": "nr_secret"},
		OAuth:      &domain.MCPOAuthGrant{AccessToken: "oauth_access", RefreshToken: "oauth_refresh", TokenURL: "https://issuer.example/token", ClientID: "client", ClientSecret: "secret", Scopes: []string{"sheets.readonly"}},
		Env:        map[string]string{"GITHUB_TOKEN": "ghp_other"},
		ConfigFile: "sources: []\n",
	}.Sealed()

	got := domain.ReadMCPCredentials(sealed)
	if got.Token != "ghp_something" ||
		got.Headers["Api-Key"] != "nr_secret" ||
		got.OAuth == nil ||
		got.OAuth.AccessToken != "oauth_access" ||
		got.OAuth.RefreshToken != "oauth_refresh" ||
		got.OAuth.Scopes[0] != "sheets.readonly" ||
		got.Env["GITHUB_TOKEN"] != "ghp_other" ||
		got.ConfigFile != "sources: []\n" {
		t.Errorf("got %+v, want every sealed shape back", got)
	}
}

/*
Correcting one half does not drop the other.

The failure this prevents is quiet: somebody fixes an address, the token they
never had to hand goes with it, and the server stops answering for a reason
nothing on the screen mentions.
*/
func TestMerge_aWriteThatOmitsTheToken_keepsTheStoredOne(t *testing.T) {
	t.Parallel()

	stored := domain.MCPCredentials{
		Token: "ghp_stored", Env: map[string]string{"REGION": "eu"},
	}
	merged := domain.MCPCredentialPatch{Env: map[string]string{"REGION": "us"}}.Apply(stored)

	if merged.Token != "ghp_stored" {
		t.Errorf("token = %q, want the stored one kept", merged.Token)
	}
	if merged.Env["REGION"] != "us" {
		t.Errorf("env = %v, want the written variables", merged.Env)
	}
}

// Absent is unchanged, and empty is a removal. They are different requests and
// reading them alike makes one of the two impossible to express.
func TestMerge_anEmptyEnvIsARemoval_andAnAbsentOneIsNot(t *testing.T) {
	t.Parallel()
	stored := domain.MCPCredentials{Env: map[string]string{"REGION": "eu"}}

	if kept := (domain.MCPCredentialPatch{Token: ptr("t")}).Apply(stored); kept.Env["REGION"] != "eu" {
		t.Errorf("env = %v, want an omitted map to leave the stored one", kept.Env)
	}
	cleared := domain.MCPCredentialPatch{Env: map[string]string{}}.Apply(stored)
	if len(cleared.Env) != 0 {
		t.Errorf("env = %v, want an empty map to clear it", cleared.Env)
	}
}

func TestApply_anEmptyConfigFileClearsIt_andAnAbsentOneDoesNot(t *testing.T) {
	t.Parallel()
	stored := domain.MCPCredentials{ConfigFile: "sources: []\n"}

	if kept := (domain.MCPCredentialPatch{}).Apply(stored); kept.ConfigFile != "sources: []\n" {
		t.Errorf("config file = %q, want an unmentioned one kept", kept.ConfigFile)
	}
	if cleared := (domain.MCPCredentialPatch{ConfigFile: ptr("")}).Apply(stored); cleared.ConfigFile != "" {
		t.Errorf("config file = %q, want it removable", cleared.ConfigFile)
	}
}

/*
The environment a child is given is built the same way twice.

Go randomises map order deliberately. An environment assembled straight from
the map reshuffles between calls, which turns a log line, a digest or a test
into something that disagrees with itself for no reason anybody can find.
*/
func TestEnviron_isTheSameOrderEveryTime(t *testing.T) {
	t.Parallel()

	creds := domain.MCPCredentials{Env: map[string]string{
		"ZONE": "c", "ALPHA": "a", "MIDDLE": "b",
	}}
	first := creds.Environ()

	if !slices.IsSorted(first) {
		t.Errorf("environ = %v, want a stable order", first)
	}
	for range 20 {
		if !slices.Equal(creds.Environ(), first) {
			t.Fatalf("environ changed between calls: %v then %v", first, creds.Environ())
		}
	}
}

/*
An omitted token keeps what is stored; an empty one takes it back.

A string cannot hold both facts. With only "empty means keep", a credential
could be written and never removed — and "I am not mentioning the token" is a
different sentence from "there is no longer a token", said by different people
for different reasons.
*/
func TestApply_anEmptyTokenClearsIt_andAnAbsentOneDoesNot(t *testing.T) {
	t.Parallel()
	stored := domain.MCPCredentials{Token: "ghp_stored"}

	if kept := (domain.MCPCredentialPatch{}).Apply(stored); kept.Token != "ghp_stored" {
		t.Errorf("token = %q, want an unmentioned one kept", kept.Token)
	}
	if cleared := (domain.MCPCredentialPatch{Token: ptr("")}).Apply(stored); cleared.Token != "" {
		t.Errorf("token = %q, want it removable", cleared.Token)
	}
}

/*
Bearer and OAuth are two HTTP credential modes, not two credentials to send.

Keeping both after a rotation leaves old material sealed and invisible. The
active mode is the one the write names; removing one mode does not remove the
other by accident, because revocation also has to say what it revokes.
*/
func TestApply_bearerAndOAuthReplaceEachOtherWhenWritten(t *testing.T) {
	t.Parallel()

	oauth := domain.MCPOAuthGrant{
		AccessToken: "access", RefreshToken: "refresh",
		TokenURL: "https://issuer.example/token",
	}
	withOAuth := (domain.MCPCredentialPatch{OAuth: &oauth}).Apply(
		domain.MCPCredentials{Token: "ghp_stored"})
	if withOAuth.Token != "" || withOAuth.OAuth == nil || withOAuth.OAuth.AccessToken != "access" {
		t.Fatalf("oauth write = %+v, want it to replace the bearer", withOAuth)
	}

	withBearer := (domain.MCPCredentialPatch{Token: ptr("ghp_new")}).Apply(withOAuth)
	if withBearer.Token != "ghp_new" || withBearer.OAuth != nil {
		t.Fatalf("bearer write = %+v, want it to replace oauth", withBearer)
	}
}

func TestApply_headersReplaceOtherRemoteCredentialsWhenWritten(t *testing.T) {
	t.Parallel()

	oauth := &domain.MCPOAuthGrant{AccessToken: "access"}
	withHeaders := (domain.MCPCredentialPatch{
		Headers: map[string]string{"Api-Key": "nr_secret"},
	}).Apply(domain.MCPCredentials{Token: "old", OAuth: oauth})

	if withHeaders.Token != "" || withHeaders.OAuth != nil ||
		withHeaders.Headers["Api-Key"] != "nr_secret" {
		t.Fatalf("headers write = %+v, want it to replace bearer and oauth", withHeaders)
	}

	withBearer := (domain.MCPCredentialPatch{Token: ptr("new")}).Apply(withHeaders)
	if withBearer.Token != "new" || len(withBearer.Headers) != 0 {
		t.Fatalf("bearer write = %+v, want it to replace headers", withBearer)
	}
}

func TestApply_emptyHeadersClearThemAndAbsentHeadersDoNot(t *testing.T) {
	t.Parallel()
	stored := domain.MCPCredentials{Headers: map[string]string{"Api-Key": "nr_secret"}}

	if kept := (domain.MCPCredentialPatch{}).Apply(stored); kept.Headers["Api-Key"] != "nr_secret" {
		t.Fatalf("omitted headers = %+v, want the stored header kept", kept)
	}
	if cleared := (domain.MCPCredentialPatch{Headers: map[string]string{}}).Apply(stored); len(cleared.Headers) != 0 {
		t.Fatalf("headers = %+v, want an empty map to clear them", cleared.Headers)
	}
}

func TestApply_anEmptyOAuthGrantClearsIt_andAnAbsentOneDoesNot(t *testing.T) {
	t.Parallel()
	stored := domain.MCPCredentials{OAuth: &domain.MCPOAuthGrant{AccessToken: "access"}}

	if kept := (domain.MCPCredentialPatch{}).Apply(stored); kept.OAuth == nil {
		t.Fatal("omitted oauth cleared the stored grant")
	}
	if cleared := (domain.MCPCredentialPatch{OAuth: &domain.MCPOAuthGrant{}}).Apply(stored); cleared.OAuth != nil {
		t.Fatalf("oauth = %+v, want an empty grant to clear it", cleared.OAuth)
	}
}

/*
A credential is shaped to the transport that can use it.

A bearer belongs to an address and a program the worker starts has none;
variables belong to a process and there is none when the platform calls a URL.
Switching transport and keeping both leaves live material sealed for a shape
that can never send it — invisible, unusable, and never revoked.
*/
func TestForTransport_switchingShape_dropsTheHalfThatCannotBeUsed(t *testing.T) {
	t.Parallel()
	both := domain.MCPCredentials{
		Token: "ghp_stored", Headers: map[string]string{"Api-Key": "nr_secret"},
		OAuth:      &domain.MCPOAuthGrant{AccessToken: "access"},
		Env:        map[string]string{"GITHUB_TOKEN": "ghp_other"},
		ConfigFile: "sources: []\n",
	}

	if local := both.ForTransport(domain.TransportStdio); local.Token != "" || local.OAuth != nil || len(local.Headers) != 0 {
		t.Errorf("a local server kept a remote credential: %+v", local)
	} else if local.ConfigFile == "" {
		t.Errorf("a local server dropped its managed config file: %+v", local)
	}
	if remote := both.ForTransport(domain.TransportHTTP); len(remote.Env) != 0 {
		t.Errorf("a remote server kept process variables: %+v", remote)
	} else if remote.ConfigFile != "" {
		t.Errorf("a remote server kept a local config file: %+v", remote)
	} else if remote.OAuth == nil {
		t.Errorf("a remote server dropped its oauth grant: %+v", remote)
	} else if remote.Headers["Api-Key"] != "nr_secret" {
		t.Errorf("a remote server dropped its headers: %+v", remote)
	}
}

func ptr[T any](v T) *T { return &v }
