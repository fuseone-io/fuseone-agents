package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// errOAuthLoginShape is a sign-in in the write's body that is not one: what
// it says is refused rather than guessed at.
var errOAuthLoginShape = errors.New("oauthLogin")

// oauthLoginFields is every property the write knows. hasClientSecret is what
// a read answers, accepted and ignored so a read sent back changes nothing.
var oauthLoginFields = map[string]bool{
	"clientID": true, "clientSecret": true, "scopes": true, "authorizeURL": true,
	"tokenURL": true, "resource": true, "tokenAuthMethod": true, "hasClientSecret": true,
}

/*
oauthLoginFromRequest reads the browser sign-in a write sends, from the bytes
as sent. Absent keeps the stored one. An object with nothing in it removes it,
and only that: a decoded struct cannot tell {} from {"clientID":null} or from
an object of misspelt fields, and reading either as a removal would take a
working sign-in and its secret away on a typo. Those are refused, as is null
for the whole object. Otherwise it is the configuration, whole, with a secret
left out kept. A refusal names a field, never a value.
*/
func oauthLoginFromRequest(raw json.RawMessage) (*domain.MCPOAuthLoginPatch, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("%w must be an object; send {} to remove it", errOAuthLoginShape)
	}
	if len(fields) == 0 {
		return &domain.MCPOAuthLoginPatch{Remove: true}, nil
	}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		switch {
		case !oauthLoginFields[name]:
			return nil, fmt.Errorf("%w has no field %q", errOAuthLoginShape, name)
		case bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")):
			return nil, fmt.Errorf("%w.%s is null; leave it out to keep what is stored", errOAuthLoginShape, name)
		}
	}
	var in openapi.MCPOAuthLoginInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("%w has a field of the wrong type", errOAuthLoginShape)
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
	return patch, nil
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
