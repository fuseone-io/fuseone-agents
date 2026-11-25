package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
An OAuth grant with neither an access token nor a refresh token it can spend is
refused when it is written, not when it is used.

The worker cannot send it and cannot renew it, so it refuses to build the
server's client at all — and before this, the write stored it, replacing
whatever worked. The client typed into the manual grant, as if it were the
browser sign-in's configuration, is how one gets written: saved without
tokens, the server stopped answering with no screen saying why.
*/

// The client typed in, no token: what the bug accepted.
var clientOnly = domain.MCPOAuthGrant{
	ClientID: "agents", TokenURL: "https://idp.example.com/token", Scopes: []string{"openid"},
}

func TestPutMCPServer_anOAuthGrantWithNothingToSend_isRefusedAndKeepsWhatWorked(t *testing.T) {
	i := newIntegrations(t)
	ctx := context.Background()
	server := domain.MCPServer{
		Name: "remote", Transport: domain.TransportHTTP,
		URL: "https://tools.example.com/mcp", Enabled: true,
	}
	if err := i.PutMCPServer(ctx, "usr_ana", platform, server,
		domain.MCPCredentialPatch{Token: ptr("working")}); err != nil {
		t.Fatalf("PutMCPServer: %v", err)
	}

	grant := clientOnly
	err := i.PutMCPServer(ctx, "usr_ana", platform, server, domain.MCPCredentialPatch{OAuth: &grant})
	if !errors.Is(err, domain.ErrMCPOAuthUnusable) {
		t.Fatalf("err = %v, want ErrMCPOAuthUnusable", err)
	}
	creds, err := i.MCPCredentials(ctx, "remote")
	if err != nil || creds.Token != "working" || creds.OAuth != nil {
		t.Fatalf("creds = %+v, %v; want the working token kept", creds, err)
	}
}

func TestPutMCPPersonalCredential_anOAuthGrantWithNothingToSend_isRefused(t *testing.T) {
	w := withAreaServer(t)
	// Allowed, so the grant's emptiness is the only thing left to refuse it.
	allowEgress(t, w.integrations, cxScope, copilotOrigin, "https://idp.example.com")

	if err := putPersonalGrant(w, clientOnly); !errors.Is(err, domain.ErrMCPOAuthUnusable) {
		t.Fatalf("err = %v, want ErrMCPOAuthUnusable", err)
	}
	if _, found, _ := w.integrations.MCPPersonalCredential(
		context.Background(), cxScope, "github", "usr_ana"); found {
		t.Error("the refused grant was stored")
	}
}
