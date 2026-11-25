package domain

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

/*
MCPOAuthLogin is how a person signs in to a remote server from their browser,
as the server's administrator configured it: whose client, which scopes, and,
where discovery does not find them, the authorization and token endpoints.

None of it is secret. The client ID travels in the authorization address the
browser opens, and the endpoints are where it goes. The client secret, when
the client has one, is held apart and sealed; this says only whether there
is one.
*/
type MCPOAuthLogin struct {
	ClientID     string
	Scopes       []string
	AuthorizeURL string
	TokenURL     string
	// Resource is the RFC 8707 resource indicator sent with every request for
	// a token. Empty is the server's own address.
	Resource        string
	TokenAuthMethod string
	HasClientSecret bool
}

// How the token endpoint is told which client asks.
const (
	TokenAuthNone              = "none"
	TokenAuthClientSecretPost  = "client_secret_post"
	TokenAuthClientSecretBasic = "client_secret_basic"
)

// ErrMCPOAuthLoginInvalid is a sign-in configuration no sign-in could use.
var ErrMCPOAuthLoginInvalid = errors.New("domain: invalid OAuth sign-in configuration")

/*
MCPOAuthLoginPatch is what one write says about the configuration.

The zero value of the pointer that carries it is silence, and keeps what is
stored. Remove is somebody taking it away, with its secret. Otherwise the
fields given are the configuration, whole: they replace the stored ones, all
but the secret, which has a gesture of its own. A nil ClientSecret keeps the
one held, and an empty one removes it, as the server's own token does.
*/
type MCPOAuthLoginPatch struct {
	Remove          bool
	ClientID        string
	Scopes          []string
	AuthorizeURL    string
	TokenURL        string
	Resource        string
	TokenAuthMethod string
	ClientSecret    *string
}

// LoginSecretChange is what a patch does to the sealed client secret: Set
// stores a new one, Remove takes the held one away, neither keeps it.
type LoginSecretChange struct {
	Set    *string
	Remove bool
}

// Equal reports the same change.
func (c LoginSecretChange) Equal(o LoginSecretChange) bool {
	if c.Remove != o.Remove || (c.Set == nil) != (o.Set == nil) {
		return false
	}
	return c.Set == nil || *c.Set == *o.Set
}

// Apply folds the patch onto what is stored, and says what becomes of the
// secret. Nil is no configuration: removed, or never written.
func (p MCPOAuthLoginPatch) Apply(stored *MCPOAuthLogin) (*MCPOAuthLogin, LoginSecretChange, error) {
	if p.Remove {
		// The secret goes with it, held or not: nothing is left to hold it for.
		return nil, LoginSecretChange{Remove: true}, nil
	}
	hasSecret := stored != nil && stored.HasClientSecret
	var change LoginSecretChange
	if p.ClientSecret != nil {
		hasSecret = *p.ClientSecret != ""
		if hasSecret {
			change.Set = p.ClientSecret
		} else {
			change.Remove = true
		}
	}
	method := p.TokenAuthMethod
	if method == "" {
		method = TokenAuthNone
		if hasSecret {
			method = TokenAuthClientSecretBasic
		}
	}
	login := &MCPOAuthLogin{
		ClientID:        strings.TrimSpace(p.ClientID),
		Scopes:          slices.Clone(p.Scopes),
		AuthorizeURL:    strings.TrimSpace(p.AuthorizeURL),
		TokenURL:        strings.TrimSpace(p.TokenURL),
		Resource:        strings.TrimSpace(p.Resource),
		TokenAuthMethod: method,
		HasClientSecret: hasSecret,
	}
	if err := login.valid(); err != nil {
		return nil, LoginSecretChange{}, err
	}
	return login, change, nil
}

func (l *MCPOAuthLogin) valid() error {
	switch {
	case l.ClientID == "":
		return fmt.Errorf("%w: a client ID is required", ErrMCPOAuthLoginInvalid)
	case l.TokenAuthMethod != TokenAuthNone &&
		l.TokenAuthMethod != TokenAuthClientSecretPost &&
		l.TokenAuthMethod != TokenAuthClientSecretBasic:
		return fmt.Errorf("%w: the token endpoint method is none, client_secret_post or client_secret_basic",
			ErrMCPOAuthLoginInvalid)
	case l.TokenAuthMethod != TokenAuthNone && !l.HasClientSecret:
		return fmt.Errorf("%w: %s sends a client secret, and none is held",
			ErrMCPOAuthLoginInvalid, l.TokenAuthMethod)
	}
	for _, address := range []openRule{
		// The browser opens it, and a policy name may ride in its query.
		{what: "authorization endpoint", raw: l.AuthorizeURL, query: true, host: true},
		// Codes and refresh tokens go there in a body; a query is where a
		// key would hide.
		{what: "token endpoint", raw: l.TokenURL, host: true},
		// RFC 8707: any absolute URI, a URN among them, with a query allowed
		// and a fragment refused. It names the server and is never fetched.
		{what: "resource", raw: l.Resource, query: true},
	} {
		if err := address.check(); err != nil {
			return err
		}
	}
	return nil
}

/*
openRule says what one address of a sign-in may be. check refuses one that
cannot be stored in the open and read back by anyone who reads the server: one
carrying a user or password, a fragment, or, where none belongs, a query. The
refusal never repeats the address, since what makes it wrong may be a
credential.
*/
type openRule struct {
	what, raw string
	// query allows one; host requires one, for an address that is fetched.
	query, host bool
}

func (r openRule) check() error {
	what, raw, query := r.what, r.raw, r.query
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	switch {
	case err != nil || !u.IsAbs() || (r.host && u.Host == ""):
		return fmt.Errorf("%w: the %s is not an absolute address", ErrMCPOAuthLoginInvalid, what)
	case u.User != nil:
		return fmt.Errorf("%w: the %s carries a user or password, and is stored and read in the open",
			ErrMCPOAuthLoginInvalid, what)
	case u.Fragment != "" || strings.Contains(raw, "#"):
		return fmt.Errorf("%w: the %s has a fragment", ErrMCPOAuthLoginInvalid, what)
	case !query && (u.RawQuery != "" || strings.Contains(raw, "?")):
		return fmt.Errorf("%w: the %s takes no query, and is stored and read in the open",
			ErrMCPOAuthLoginInvalid, what)
	}
	return nil
}

// Equal reports the same configuration; nil is equal only to nil.
func (l *MCPOAuthLogin) Equal(o *MCPOAuthLogin) bool {
	if l == nil || o == nil {
		return l == o
	}
	return l.ClientID == o.ClientID && slices.Equal(l.Scopes, o.Scopes) &&
		l.AuthorizeURL == o.AuthorizeURL && l.TokenURL == o.TokenURL &&
		l.Resource == o.Resource && l.TokenAuthMethod == o.TokenAuthMethod &&
		l.HasClientSecret == o.HasClientSecret
}

/*
TakesPersonalCredential is whether every tool call on the server acts as the
caller, with their own credential and never the shared one: its recipe says
only people hold credentials for it (userOnly), or people sign in to it from
their browser. Discovery may still use the shared credential; a call may not.
A local process has nobody's credential to take.
*/
func (s MCPServer) TakesPersonalCredential(userOnly bool) bool {
	return s.TransportOf() == TransportHTTP && (userOnly || s.OAuthLogin != nil)
}
