package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
The platform's egress policy for a server outside the installation (NT-019).

The server's URL is checked before anything is sent, and every request is
checked again as it leaves: a redirect is a new request through the same
client, and the clients that refresh an OAuth grant or mint a GitHub App
token carry the same guard. Nil is the installation's server, which keeps the
reach it always had.
*/

// egressGuard refuses any request to an origin the policy does not list.
type egressGuard struct {
	server string
	egress *domain.MCPEgress
	base   http.RoundTripper
}

func (g egressGuard) RoundTrip(r *http.Request) (*http.Response, error) {
	if !g.egress.Allows(r.URL) {
		return nil, egressRefused(g.server, r.URL)
	}
	return g.base.RoundTrip(r)
}

func egressRefused(server string, u *url.URL) error {
	return fmt.Errorf("%s: %s://%s is not an origin the platform allows this server's scope (NT-019)",
		server, u.Scheme, u.Host)
}

// guardEgress wraps base in the policy, or returns it as it is for the
// installation's server.
func guardEgress(server string, egress *domain.MCPEgress, base http.RoundTripper) http.RoundTripper {
	if egress == nil {
		return base
	}
	return egressGuard{server: server, egress: egress, base: base}
}

// effectiveEgress is the policy a server is held to. Only a server with no
// scope, a --mcp flag's, or the installation's may carry none. Any other
// server whose policy got lost on the way allows nothing: a missing field
// fails closed, never into the installation's reach.
func effectiveEgress(server domain.MCPServer) *domain.MCPEgress {
	if server.Egress != nil {
		return server.Egress
	}
	if server.Scope == (domain.Scope{}) || server.Scope.IsInstallation() {
		return nil
	}
	return &domain.MCPEgress{}
}

// withEgress holds a server's whole client to its policy, outermost, so every
// decoration underneath and every redirect passes through it.
func withEgress(server domain.MCPServer, client *http.Client) *http.Client {
	egress := effectiveEgress(server)
	if egress == nil {
		return client
	}
	if client == nil {
		client = &http.Client{Transport: baseHTTPTransport(), Timeout: 60 * time.Second}
	}
	out := *client
	base := out.Transport
	if base == nil {
		base = baseHTTPTransport()
	}
	out.Transport = guardEgress(server.Name, egress, base)
	return &out
}

// egressAllowsServer refuses, before any request, a server whose own URL its
// policy does not list, and one whose policy allows nothing.
func egressAllowsServer(server domain.MCPServer) error {
	egress := effectiveEgress(server)
	if egress == nil {
		return nil
	}
	u, err := url.Parse(server.URL)
	if err != nil {
		return fmt.Errorf("%s: parse url: %w", server.Name, err)
	}
	if len(egress.Origins) == 0 {
		return fmt.Errorf("%s: the platform allows this server's scope no origin (NT-019)", server.Name)
	}
	if !egress.Allows(u) {
		return egressRefused(server.Name, u)
	}
	return nil
}

// egressFingerprint tells apart the installation's server, one with no
// policy, and each list of origins.
func egressFingerprint(egress *domain.MCPEgress) string {
	if egress == nil {
		return "installation"
	}
	return "origins:" + strings.Join(egress.Origins, ",")
}
