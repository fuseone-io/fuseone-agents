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
	merged := stored.Merge(domain.MCPCredentials{Env: map[string]string{"REGION": "us"}})

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

	if kept := stored.Merge(domain.MCPCredentials{Token: "t"}); kept.Env["REGION"] != "eu" {
		t.Errorf("env = %v, want an omitted map to leave the stored one", kept.Env)
	}
	cleared := stored.Merge(domain.MCPCredentials{Env: map[string]string{}})
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
