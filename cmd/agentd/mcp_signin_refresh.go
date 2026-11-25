package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/mcpauth"
)

// SignInClientStore is where a grant got through a server's browser sign-in
// finds the client it refreshes as: the server's configuration as it is now,
// and its secret.
type SignInClientStore interface {
	MCPOAuthLoginClient(ctx context.Context, runtime domain.Scope, server string) (domain.MCPOAuthLogin, string, error)
}

var _ SignInClientStore = (*admin.Integrations)(nil)

// errSignInClientChanged is a server whose sign-in configuration is no longer
// the one the person's grant was got under: another client, endpoint, method
// or resource. Its refresh token belongs to that configuration, and sending
// the secret now in force to the old endpoint would hand it to whoever holds it.
var errSignInClientChanged = errors.New("the server's sign-in configuration changed since this grant was got; sign in again")

// refreshed is the grant after a refresh: a grant pasted by a person carries
// its own client, and one got through the sign-in asks the server for it.
func (t *oauthTransport) refreshed(ctx context.Context) (domain.MCPOAuthGrant, error) {
	if !t.grant.SignIn {
		return refreshOAuthGrant(ctx, t.refresh, t.grant)
	}
	clients, ok := t.store.(SignInClientStore)
	if !ok {
		return domain.MCPOAuthGrant{}, errors.New("oauth refresh: no store answers a sign-in's client")
	}
	login, secret, err := clients.MCPOAuthLoginClient(ctx, t.scope, t.server)
	if err != nil {
		return domain.MCPOAuthGrant{}, fmt.Errorf("oauth refresh: %w", err)
	}
	return refreshSignInGrant(ctx, t.refresh, t.grant, signInClient{login: login, secret: secret})
}

type signInClient struct {
	login  domain.MCPOAuthLogin
	secret string
}

func refreshSignInGrant(
	ctx context.Context, hc *http.Client, grant domain.MCPOAuthGrant, client signInClient,
) (domain.MCPOAuthGrant, error) {
	// The configuration the grant was got under, the secret aside: a token
	// endpoint changed with the client kept must not receive the new secret.
	if grant.SignInConfig == "" || client.login.Identity() != grant.SignInConfig {
		return domain.MCPOAuthGrant{}, errSignInClientChanged
	}
	tokens, err := mcpauth.Refresh(ctx, hc, mcpauth.TokenRequest{
		TokenURL: grant.TokenURL,
		Client:   mcpauth.Client{ID: grant.ClientID, Secret: client.secret, Method: client.login.TokenAuthMethod},
		Resource: grant.Resource,
	}, grant.RefreshToken)
	if err != nil {
		return domain.MCPOAuthGrant{}, fmt.Errorf("oauth refresh: %w", err)
	}
	/*
		A provider that rotates refresh tokens spends the old one here; the
		caller persists this before sending a request with it, as for any grant.
	*/
	next := grant
	next.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		next.RefreshToken = tokens.RefreshToken
	}
	if tokens.TokenType != "" {
		next.TokenType = strings.TrimSpace(tokens.TokenType)
	}
	next.ExpiresAtUnix = 0
	if tokens.ExpiresIn > 0 {
		next.ExpiresAtUnix = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix()
	}
	if len(tokens.Scopes) > 0 {
		next.Scopes = tokens.Scopes
	}
	return next, nil
}
