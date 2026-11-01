package trigger_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/fuseone/agents/internal/trigger"
)

func signed(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

/*
A GitHub delivery proves itself by its signature, not by carrying a secret.

The HMAC is over the raw body: a body re-encoded on the way, a signature for
another secret, the SHA-1 header GitHub also sends, or a signature with no
algorithm prefix are each a delivery nobody can vouch for.
*/
func TestVerifyGitHubSignature(t *testing.T) {
	t.Parallel()

	const secret, body = "hook-secret", `{"action":"opened"}`
	cases := []struct {
		name   string
		body   string
		header string
		want   bool
	}{
		{"signed with the secret", body, signed(secret, body), true},
		{"another secret", body, signed("other", body), false},
		{"the body changed after signing", `{"action":"closed"}`, signed(secret, body), false},
		{"no signature", body, "", false},
		{"no algorithm prefix", body, signed(secret, body)[len("sha256="):], false},
		{"the SHA-1 form", body, "sha1=" + signed(secret, body)[len("sha256="):], false},
		{"not hex", body, "sha256=zz", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := trigger.VerifyGitHubSignature([]byte(secret), []byte(c.body), c.header); got != c.want {
				t.Errorf("VerifyGitHubSignature = %v, want %v", got, c.want)
			}
		})
	}
}

// A signature checked against an empty secret proves nothing: anybody can
// compute the HMAC of a body under no key.
func TestVerifyGitHubSignature_withNoSecret_acceptsNothing(t *testing.T) {
	t.Parallel()

	body := `{"action":"opened"}`
	if trigger.VerifyGitHubSignature(nil, []byte(body), signed("", body)) {
		t.Error("a delivery signed under an empty secret was accepted")
	}
}

/*
A GitHub hook starts a run only for the deliveries its trigger names.

`pull_request` takes every action of the event; `pull_request.labeled` only
that one. GitHub sends everything the hook subscribes to, so the filter is what
keeps a busy repository from starting a run per comment.
*/
func TestGitHubWants(t *testing.T) {
	t.Parallel()

	declared := []string{"pull_request.opened", "pull_request.labeled", "issues"}
	cases := []struct {
		event, action string
		want          bool
	}{
		{"pull_request", "opened", true},
		{"pull_request", "labeled", true},
		{"pull_request", "closed", false},
		{"pull_request", "", false},
		{"issues", "edited", true},
		{"issues", "", true},
		{"push", "", false},
		{"issue_comment", "created", false},
	}
	for _, c := range cases {
		if got := trigger.GitHubWants(declared, c.event, c.action); got != c.want {
			t.Errorf("GitHubWants(%q, %q) = %v, want %v", c.event, c.action, got, c.want)
		}
	}
}

func TestGitHubAction(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		`{"action":"labeled","number":7}`: "labeled",
		`{"ref":"refs/heads/main"}`:       "",
		`{"action":7}`:                    "",
		`not json`:                        "",
	}
	for body, want := range cases {
		if got := trigger.GitHubAction([]byte(body)); got != want {
			t.Errorf("GitHubAction(%s) = %q, want %q", body, got, want)
		}
	}
}
