package domain_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
What is refused, what is questioned, and what must pass untouched.

The third column is the one that decides whether this ships. A rule that refuses
a digest refuses the platform's own evidence, and a rule that refuses a
certificate name refuses somebody recording which certificate an incident was
about — both of which teach people to work around the check, which is worse than
not having it.
*/
func TestLooksLikeSecret(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		text string
		want domain.SecretRisk
	}{
		{"a private key header", "-----BEGIN RSA PRIVATE KEY-----", domain.SecretCertain},
		{"an openssh key header", "-----BEGIN OPENSSH PRIVATE KEY-----", domain.SecretCertain},
		{"a github token", "ghp_" + strings.Repeat("a1B2", 9), domain.SecretCertain},
		{"a slack token", "xoxb-1234567890-abcdefghij", domain.SecretCertain},
		{"an aws access key id", "AKIAIOSFODNN7EXAMPLE", domain.SecretCertain},
		{"a stripe live key", "sk_live_" + strings.Repeat("Xy1", 8), domain.SecretCertain},
		{"a google api key", "AIza" + strings.Repeat("aB1", 11) + "xy", domain.SecretCertain},
		{"a token inside a sentence", "the runbook says use ghp_" + strings.Repeat("a1B2", 9) + " here",
			domain.SecretCertain},

		// A public certificate opens exactly like a private key and is not one.
		{"a certificate header", "-----BEGIN CERTIFICATE-----", domain.SecretSuspected},
		{"a truncated token", "the key starts with ghp_ and I did not copy it", domain.SecretSuspected},
		{"an unrecognised opaque run", "aB3" + strings.Repeat("xY7z", 10), domain.SecretSuspected},
		// The dots between the segments are what let this through as prose until
		// the shape was named.
		{"a bearer token", "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0." +
			"dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk", domain.SecretSuspected},

		// Everything a memory is actually made of.
		{"a claim", "restart the api deployment after the datasource error clears", domain.SecretNone},
		{"a signature", "grafana.datasource.down", domain.SecretNone},
		{"a whole digest", strings.Repeat("ab12cd34", 8), domain.SecretNone},
		{"a prefixed digest", "sha256:" + strings.Repeat("ab12cd34", 8), domain.SecretNone},
		// Mixed case, so the character classes alone would call it opaque. It is
		// the hexadecimal rule that has to answer this one.
		{"a digest somebody pasted in mixed case", strings.Repeat("aB12Cd34", 8), domain.SecretNone},
		{"a run id", "run-2026-08-27-triage-0001", domain.SecretNone},
		{"a uuid", "3f2504e0-4f89-11d3-9a0c-0305e82c3301", domain.SecretNone},
		{"a long sentence", strings.Repeat("the datasource token expires nightly ", 4), domain.SecretNone},
		{"a url", "https://grafana.internal.example.com/d/abc/datasource-health", domain.SecretNone},
		// Long, and carrying all three character classes. What keeps it out is
		// the punctuation no token alphabet uses.
		{"a url with a mixed-case path", "https://grafana.example.com/d/AbC123/datasource-health",
			domain.SecretNone},
		{"a path", "/var/log/Fuseone/agents-2026-08-27/worker-0.log", domain.SecretNone},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.LooksLikeSecret(c.text); got != c.want {
				t.Errorf("LooksLikeSecret(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

// The worst of the fields, not the first. A subject nobody would look at twice
// beside a claim carrying a key is still a memory carrying a key.
func TestLooksLikeSecret_reportsTheWorstFieldNotTheFirst(t *testing.T) {
	t.Parallel()
	got := domain.LooksLikeSecret(
		"grafana datasource",
		"grafana.datasource.down",
		"-----BEGIN EC PRIVATE KEY-----",
	)
	if got != domain.SecretCertain {
		t.Errorf("risk = %q, want the field carrying a key to decide", got)
	}
}
