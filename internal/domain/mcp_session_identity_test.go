package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A person's session is opened under one identity, and a server that fixes who
is calling at initialize keeps it for the session's life. So the identity moves
when somebody deliberately changes the credential — a new token, new headers,
new tokens pasted or signed in — and never when the worker rotates the access
token, which would reopen every session on every refresh.
*/
func TestSessionIdentity_movesOnADeliberateChangeOnly(t *testing.T) {
	t.Parallel()
	grant := domain.MCPOAuthGrant{
		AccessToken: "access-1", RefreshToken: "refresh-1", TokenURL: "https://idp.example.com/token",
		ClientID: "agents", ExpiresAtUnix: 100, Generation: "gen-1",
	}
	rotated := grant
	rotated.AccessToken, rotated.RefreshToken, rotated.ExpiresAtUnix = "access-2", "refresh-2", 200
	repasted := grant
	repasted.Generation = "gen-2"
	of := func(c domain.MCPCredentials) string { return c.SessionIdentity() }

	cases := []struct {
		name       string
		a, b       domain.MCPCredentials
		wantSameID bool
	}{
		{"the worker rotated the grant", domain.MCPCredentials{OAuth: &grant}, domain.MCPCredentials{OAuth: &rotated}, true},
		{"new tokens were written", domain.MCPCredentials{OAuth: &grant}, domain.MCPCredentials{OAuth: &repasted}, false},
		{"another token", domain.MCPCredentials{Token: "first"}, domain.MCPCredentials{Token: "second"}, false},
		{"the same token", domain.MCPCredentials{Token: "first"}, domain.MCPCredentials{Token: "first"}, true},
		{"another header value", domain.MCPCredentials{Headers: map[string]string{"X-Key": "a"}},
			domain.MCPCredentials{Headers: map[string]string{"X-Key": "b"}}, false},
		{"a token and a grant", domain.MCPCredentials{Token: "first"}, domain.MCPCredentials{OAuth: &grant}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if same := of(tc.a) == of(tc.b); same != tc.wantSameID {
				t.Errorf("same identity = %v, want %v", same, tc.wantSameID)
			}
		})
	}
}

// The identity is a digest: it can sit beside a session without holding the
// credential it names.
func TestSessionIdentity_holdsNoSecret(t *testing.T) {
	t.Parallel()
	id := domain.MCPCredentials{Token: "ghp_secret"}.SessionIdentity()
	if id == "" || id == "ghp_secret" || len(id) != 64 {
		t.Errorf("identity = %q, want a 64-hex digest", id)
	}
}
