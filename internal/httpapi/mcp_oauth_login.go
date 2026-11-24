package httpapi

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// oauthLoginFromRequest reads the browser sign-in a write sends: absent keeps
// the stored one, an empty object removes it, and anything else is the
// configuration, whole, with a secret left out kept.
func oauthLoginFromRequest(in *openapi.MCPOAuthLoginInput) *domain.MCPOAuthLoginPatch {
	if in == nil {
		return nil
	}
	if *in == (openapi.MCPOAuthLoginInput{}) {
		return &domain.MCPOAuthLoginPatch{Remove: true}
	}
	patch := &domain.MCPOAuthLoginPatch{
		ClientID:     valueOr(in.ClientID),
		AuthorizeURL: valueOr(in.AuthorizeURL),
		TokenURL:     valueOr(in.TokenURL),
		Resource:     valueOr(in.Resource),
		ClientSecret: in.ClientSecret,
	}
	if in.Scopes != nil {
		patch.Scopes = slices.Clone(*in.Scopes)
	}
	if in.TokenAuthMethod != nil {
		patch.TokenAuthMethod = string(*in.TokenAuthMethod)
	}
	return patch
}

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
