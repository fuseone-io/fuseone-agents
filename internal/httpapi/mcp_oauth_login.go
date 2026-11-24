package httpapi

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// oauthLoginToResponse is the configuration as read: never the secret, only
// whether one is held.
func oauthLoginToResponse(login *domain.MCPOAuthLogin) *openapi.MCPOAuthLogin {
	if login == nil {
		return nil
	}
	out := &openapi.MCPOAuthLogin{
		ClientID:        login.ClientID,
		HasClientSecret: login.HasClientSecret,
		TokenAuthMethod: openapi.MCPOAuthLoginTokenAuthMethod(login.TokenAuthMethod),
		AuthorizeURL:    emptyAsNil(login.AuthorizeURL),
		TokenURL:        emptyAsNil(login.TokenURL),
		Resource:        emptyAsNil(login.Resource),
	}
	if len(login.Scopes) > 0 {
		scopes := slices.Clone(login.Scopes)
		out.Scopes = &scopes
	}
	return out
}

func emptyAsNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
