package domain_test

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestCheckOAuthUsable_aGrantWithNothingToSend_isRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		grant *domain.MCPOAuthGrant
		want  error
	}{
		{"no grant", nil, nil},
		{"a removal", &domain.MCPOAuthGrant{}, nil},
		{"an access token alone", &domain.MCPOAuthGrant{AccessToken: "access"}, nil},
		{"a refresh token and where to spend it", &domain.MCPOAuthGrant{
			RefreshToken: "refresh", TokenURL: "https://idp.example.com/token"}, nil},
		// What a connection form leaves when it fills the client in by itself.
		{"the client and nothing else", &domain.MCPOAuthGrant{
			ClientID: "agents", TokenURL: "https://idp.example.com/token",
			Scopes: []string{"openid"}}, domain.ErrMCPOAuthUnusable},
		{"a refresh token with nowhere to spend it", &domain.MCPOAuthGrant{
			RefreshToken: "refresh", ClientID: "agents"}, domain.ErrMCPOAuthUnusable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := domain.CheckOAuthUsable(tc.grant); !errors.Is(err, tc.want) {
				t.Fatalf("CheckOAuthUsable = %v, want %v", err, tc.want)
			}
		})
	}
}
