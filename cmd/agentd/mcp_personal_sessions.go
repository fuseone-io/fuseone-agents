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
		if err != nil || !found {
			return "", err
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
	return func(ctx context.Context, who domain.UserID) (tools.Session, error) {
		transport, _, err := transportFor(ctx, server, creds, oauth, personal, policy, nil)
		if err != nil {
			return nil, err
		}
		streamable, ok := transport.(*mcp.StreamableClientTransport)
		if !ok {
			return nil, fmt.Errorf("%s: a person's session needs an HTTP transport", server.Name)
		}
		streamable.HTTPClient = onBehalfOf(streamable.HTTPClient, who)
		client := mcp.NewClient(&mcp.Implementation{Name: "fuseone-agents", Version: version}, nil)
		session, err := client.Connect(ctx, streamable, nil)
		if err != nil {
			return nil, fmt.Errorf("connect %s for %s: %w", server.Name, who, err)
		}
		return session, nil
	}
}

// onBehalfOf makes every request the client sends one made for who.
func onBehalfOf(client *http.Client, who domain.UserID) *http.Client {
	out := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	out.Transport = personRoundTripper{who: who, base: base}
	return &out
}

type personRoundTripper struct {
	who  domain.UserID
	base http.RoundTripper
}

func (p personRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := tools.WithInvocation(tools.WithCaller(r.Context(), p.who))
	return p.base.RoundTrip(r.WithContext(ctx))
}
