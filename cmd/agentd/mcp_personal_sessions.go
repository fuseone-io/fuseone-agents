package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
)

/*
personalSessions opens the server's session for one person: every request on
it — its initialize, its calls, the DELETE that closes it — is made on that
person's behalf, as a tool call, so the credential policy finds their own
credential or refuses. It never falls back to the shared one: a session opened
with it would answer the person as nobody.
*/
func personalSessions(
	server domain.MCPServer, creds domain.MCPCredentials,
	oauth OAuthGrantStore, personal MCPUserCredentialStore, policy credentialPolicy,
) tools.ServerOption {
	if !policy.requirePersonal || server.TransportOf() != domain.TransportHTTP {
		return tools.WithPersonalSessions(tools.PersonalSessions{})
	}
	return tools.WithPersonalSessions(tools.PersonalSessions{
		Open:     personalOpener(server, creds, oauth, personal, policy),
		Identity: personalIdentity(server, personal),
	})
}

// personalIdentity is who the person's credential speaks as now. None stored
// is an identity too, the empty one: the open that follows is refused.
func personalIdentity(
	server domain.MCPServer, personal MCPUserCredentialStore,
) func(context.Context, domain.UserID) (string, error) {
	return func(ctx context.Context, who domain.UserID) (string, error) {
		if personal == nil {
			return "", nil
		}
		creds, found, err := personal.MCPPersonalCredential(ctx, server.Scope, server.Name, who)
		if err != nil {
			// The same refusal the transport gives when it reads it.
			return "", credentialFailure(tools.CodeMCPPersonalCredentialRead, server.Name,
				fmt.Sprintf("read personal MCP credential for %s", who), err)
		}
		if !found {
			return "", nil
		}
		return creds.ForTransport(domain.TransportHTTP).SessionIdentity(), nil
	}
}

// personalOpener is the open itself. Whatever context it is handed — a call's,
// or one that names nobody — the session it opens is the person's.
func personalOpener(
	server domain.MCPServer, creds domain.MCPCredentials,
	oauth OAuthGrantStore, personal MCPUserCredentialStore, policy credentialPolicy,
) tools.PersonalOpener {
	return func(ctx context.Context, who domain.UserID, identity string) (tools.Session, error) {
		transport, _, err := transportFor(ctx, server, creds, oauth, personal, policy, nil)
		if err != nil {
			return nil, err
		}
		streamable, ok := transport.(*mcp.StreamableClientTransport)
		if !ok {
			return nil, fmt.Errorf("%s: a person's session needs an HTTP transport", server.Name)
		}
		streamable.HTTPClient = onBehalfOf(streamable.HTTPClient, who, identity)
		client := mcp.NewClient(&mcp.Implementation{Name: "fuseone-agents", Version: version}, nil)
		session, err := client.Connect(ctx, streamable, nil)
		if err != nil {
			return nil, fmt.Errorf("connect %s for %s: %w", server.Name, who, err)
		}
		return session, nil
	}
}

// onBehalfOf makes every request the client sends one made for who, on a
// session opened as identity: one whose credential has moved on is refused.
func onBehalfOf(client *http.Client, who domain.UserID, identity string) *http.Client {
	out := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	out.Transport = personRoundTripper{who: who, identity: identity, base: base}
	return &out
}

type personRoundTripper struct {
	who      domain.UserID
	identity string
	base     http.RoundTripper
}

func (p personRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := tools.WithInvocation(tools.WithCaller(r.Context(), p.who))
	ctx = tools.WithSessionIdentity(ctx, p.identity)
	return p.base.RoundTrip(r.WithContext(ctx))
}

/*
personalCredentials is the person's own credential, as it is now. On a session
opened as one identity of it (tools.WithSessionIdentity), a credential that
speaks as another is refused before the request is sent: the server tied the
session to the old one, and would run it as the old account.
*/
func (a *credentialAuth) personalCredentials(
	ctx context.Context, principal domain.UserID,
) (domain.MCPCredentials, bool, error) {
	creds, found, err := a.personal.MCPPersonalCredential(ctx, a.scope, a.server, principal)
	if err != nil {
		return domain.MCPCredentials{}, false, credentialFailure(
			tools.CodeMCPPersonalCredentialRead, a.server,
			fmt.Sprintf("read personal MCP credential for %s", principal), err)
	}
	if !found {
		return domain.MCPCredentials{}, false, nil
	}
	creds = creds.ForTransport(domain.TransportHTTP)
	if opened, ok := tools.SessionIdentityFrom(ctx); ok && creds.SessionIdentity() != opened {
		return domain.MCPCredentials{}, false, fmt.Errorf("%s for %s: %w", a.server, principal, tools.ErrSessionIdentityChanged)
	}
	if domain.CheckOAuthUsable(creds.OAuth) != nil {
		return domain.MCPCredentials{}, false, credentialFailure(
			tools.CodeMCPPersonalCredentialInvalid, a.server,
			"personal oauth grant has no access token and cannot refresh", nil)
	}
	return creds, true, nil
}
