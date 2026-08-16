package domain_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
What a tool server is given, and what a write leaves alone.

The document is a credential in both halves. A token is obviously one; a
variable is one nearly always, because the reason a server needs a variable is
usually that the variable is a key.
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
		Token: "ghp_something",
		Env:   map[string]string{"GITHUB_TOKEN": "ghp_other"},
	}.Sealed()

	got := domain.ReadMCPCredentials(sealed)
	if got.Token != "ghp_something" || got.Env["GITHUB_TOKEN"] != "ghp_other" {
		t.Errorf("got %+v, want both halves back", got)
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
A credential is shaped to the transport that can use it.

A bearer belongs to an address and a program the worker starts has none;
variables belong to a process and there is none when the platform calls a URL.
Switching transport and keeping both leaves live material sealed for a shape
that can never send it — invisible, unusable, and never revoked.
*/
func TestForTransport_switchingShape_dropsTheHalfThatCannotBeUsed(t *testing.T) {
	t.Parallel()
	both := domain.MCPCredentials{
		Token: "ghp_stored", Env: map[string]string{"GITHUB_TOKEN": "ghp_other"},
	}

	if local := both.ForTransport(domain.TransportStdio); local.Token != "" {
		t.Errorf("a local server kept a bearer token: %+v", local)
	}
	if remote := both.ForTransport(domain.TransportHTTP); len(remote.Env) != 0 {
		t.Errorf("a remote server kept process variables: %+v", remote)
	}
}

func ptr[T any](v T) *T { return &v }
