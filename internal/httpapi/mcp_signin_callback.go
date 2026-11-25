package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/mcpauth"
)

// What a return from a browser sign-in tells the console, and nothing else:
// a closed set, so nothing the provider or the request carried is repeated.
const (
	signInConnected = "connected"
	signInDenied    = "denied"
	signInExpired   = "expired"
	signInNotYours  = "not_yours"
	signInChanged   = "changed"
	signInRefused   = "refused"
	signInFailed    = "failed"
)

/*
SignInCallback is where the browser comes back from a server's sign-in. It is
outside the API's contract, as a sign-in's return always is, and inside the
session: whoever returns is judged against whoever began. Mounted behind the
session middleware by the caller.

The sign-in in flight is taken first, whatever the answer, so one browser
coming back is one try. The person must be the one who began it, the server
the same row at the same revision, and the issuer, when the authorization
server says it names itself, the one it was found at. Only then is the code
exchanged, as the server's client, and the person's own credential stored.
*/
func (s *Server) SignInCallback() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		server, outcome := s.completeSignIn(r.Context(), r.URL.Query())
		back := url.Values{"oauth": {outcome}}
		if server != "" {
			back.Set("server", server)
		}
		http.Redirect(w, r, "/integrations/credentials?"+back.Encode(), http.StatusSeeOther)
	})
}

func (s *Server) completeSignIn(ctx context.Context, q url.Values) (string, string) {
	caller, ok := auth.PrincipalFrom(ctx)
	if !ok || s.mcpSignIn == nil || q.Get("state") == "" {
		return "", signInExpired
	}
	cfg := s.mcpSignIn
	now := cfg.Now()
	flow, err := cfg.Store.ConsumeMCPOAuthFlow(ctx, q.Get("state"), caller.ID, now)
	switch {
	case errors.Is(err, admin.ErrMCPOAuthFlowNotYours):
		return "", signInNotYours
	case err != nil:
		return "", signInExpired
	case q.Get("error") != "":
		return flow.Server, signInDenied
	case !issuerHolds(flow, q.Get("iss")):
		return flow.Server, signInRefused
	}
	server, err := cfg.Store.MCPSignInServer(ctx, flow.Server, caller.ScopesFor(domain.PermToolRead))
	if err != nil || server.Scope != flow.ServerScope || !server.UpdatedAt.Equal(flow.ServerAt) {
		return flow.Server, signInChanged
	}
	grant, outcome := s.exchange(ctx, server, flow, q.Get("code"))
	if outcome != "" {
		return flow.Server, outcome
	}
	if err := s.integrations.WriteMCPPersonalCredential(ctx, admin.PersonalCredentialWrite{
		By: caller.ID, Scope: firstVisibleScope(caller), Visible: caller.ScopesFor(domain.PermToolRead),
		Server: flow.Server, Given: domain.MCPCredentialPatch{OAuth: &grant},
	}); err != nil {
		return flow.Server, signInFailed
	}
	return flow.Server, signInConnected
}

// issuerHolds is RFC 9207: an authorization server that names itself in its
// answers must name the one the sign-in was found at, and one that names
// another is refused even where naming is optional.
func issuerHolds(flow admin.MCPOAuthFlow, iss string) bool {
	if iss == "" {
		return !flow.IssParameter
	}
	return flow.Issuer == "" || iss == flow.Issuer
}

// exchange turns the code into the person's grant, as the server's client as
// it is now. The grant holds no client secret: every refresh reads it again.
func (s *Server) exchange(
	ctx context.Context, server domain.MCPServer, flow admin.MCPOAuthFlow, code string,
) (domain.MCPOAuthGrant, string) {
	cfg := s.mcpSignIn
	/*
		Read with the secret, in one snapshot, and resolved again: the
		configuration the secret is for, and the token endpoint and resource
		in effect now (found again where none is configured), must be those
		the sign-in began under, or the secret now in force would go to an
		endpoint of another. A change between this read and the server's
		revision checked above is caught here, and so is metadata that moved
		the endpoint since the start.
	*/
	client, err := cfg.Store.MCPOAuthLoginClient(ctx, server.Scope, server.Name)
	if err != nil {
		return domain.MCPOAuthGrant{}, signInChanged
	}
	hc := cfg.Client(server)
	tokenURL, resource, err := mcpauth.InEffect(ctx, hc, client.ServerURL, client.Login)
	if err != nil || client.Login.Identity(tokenURL, resource) != flow.SignInConfig {
		return domain.MCPOAuthGrant{}, signInChanged
	}
	tokens, err := mcpauth.ExchangeCode(ctx, hc, mcpauth.CodeExchange{
		Token: mcpauth.TokenRequest{
			TokenURL: tokenURL, Resource: resource,
			Client: mcpauth.Client{ID: flow.ClientID, Secret: client.Secret, Method: client.Login.TokenAuthMethod},
		},
		Code: code, Verifier: flow.Verifier, RedirectURI: flow.RedirectURI,
	})
	if err != nil {
		return domain.MCPOAuthGrant{}, signInRefused
	}
	grant := domain.MCPOAuthGrant{
		AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
		TokenURL: flow.TokenURL, ClientID: flow.ClientID, TokenType: tokens.TokenType,
		Scopes: tokens.Scopes, SignIn: true, Resource: flow.Resource, SignInConfig: flow.SignInConfig,
	}
	if tokens.ExpiresIn > 0 {
		grant.ExpiresAtUnix = cfg.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix()
	}
	return grant, ""
}
