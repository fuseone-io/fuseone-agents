package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
)

/*
Discovery on a person's behalf (#483, delivery 3).

A server that takes each person's own credential is discovered with the shared
one, or as nobody when there is none, as any other. When that cannot work — the
shared grant holds nothing a request could carry, or the server refused the
discovery with 401 or 403 — it is discovered as whoever connected their account
to it last (admin.MCPPersonalDiscoverer). Every worker asks the same store and
discovers as the same person, each on its own pass, so a restart or a worker
that started later finds it too.

The session opened that way only lists tools. A call never runs on it: one with
no person, a schedule's, would otherwise run as whoever discovered.
*/

// errNobodyToDiscoverAs is a server only a person can discover, and nobody has
// connected an account to it.
var errNobodyToDiscoverAs = errors.New("no person has connected an account to discover this server")

// errDiscoveryRefused is the server answering a discovery that carried no
// person's credential with 401 or 403.
var errDiscoveryRefused = errors.New("the server refused discovery without a person's credential")

// discoverer is the person a session was discovered as, and the identity of
// their credential it was opened with.
type discoverer struct {
	who      domain.UserID
	identity string
}

type personalDiscoverers interface {
	MCPPersonalDiscoverer(ctx context.Context, at domain.Scope, name string) (domain.UserID, bool, error)
}

var _ personalDiscoverers = (*admin.Integrations)(nil)

type discoveryTrail interface {
	RecordMCPDiscoveredOnBehalf(ctx context.Context, d admin.MCPDiscoveredOnBehalf) error
}

var _ discoveryTrail = (*admin.Integrations)(nil)

// serverLink is what one connection of a server is made with.
type serverLink struct {
	server   domain.MCPServer
	creds    domain.MCPCredentials
	oauth    OAuthGrantStore
	personal MCPUserCredentialStore
	policy   credentialPolicy
}

// forCalls is the policy of a person's own sessions: each is opened as its
// person, never as whoever discovered.
func (p credentialPolicy) forCalls() credentialPolicy {
	p.discoverAs = nil
	return p
}

// linkFor is what the server is connected with: the stores a credential is
// read and renewed through, and whether each call acts as its caller — the
// recipe saying only people hold credentials for it, or a browser sign-in
// configured on it.
func (r *reconciler) linkFor(server domain.MCPServer, creds domain.MCPCredentials) serverLink {
	l := serverLink{server: server, creds: creds}
	if store, ok := r.servers.(OAuthGrantStore); ok {
		l.oauth = store
	}
	if store, ok := r.servers.(MCPUserCredentialStore); ok {
		l.personal = store
	}
	userOnly := r.policy != nil && r.policy.RequiresPersonalCredential(server.Name)
	l.policy = credentialPolicy{requirePersonal: server.TakesPersonalCredential(userOnly)}
	return l
}

// reach connects the server, and says who it was discovered as: nil is with
// the shared credential, or as nobody.
func (r *reconciler) reach(ctx context.Context, l serverLink) (*discoverer, error) {
	if !l.policy.requirePersonal || l.server.TransportOf() != domain.TransportHTTP {
		return nil, r.dial(ctx, l)
	}
	if domain.CheckOAuthUsable(l.creds.OAuth) == nil {
		err := r.dial(ctx, l)
		if !errors.Is(err, errDiscoveryRefused) {
			return nil, err
		}
	}
	as, err := r.discovererOf(ctx, l.server, l.personal)
	if err != nil {
		return nil, err
	}
	l.policy.discoverAs = as
	return as, r.dial(ctx, l)
}

func (r *reconciler) dial(ctx context.Context, l serverLink) error {
	return r.connectTo(ctx, r.catalog, l.server, l.creds, l.oauth, l.personal, l.policy, r.egress)
}

// discovererOf is who the server is discovered as now, with the identity their
// credential speaks as.
func (r *reconciler) discovererOf(
	ctx context.Context, server domain.MCPServer, personal MCPUserCredentialStore,
) (*discoverer, error) {
	finder, ok := r.servers.(personalDiscoverers)
	if !ok || personal == nil {
		return nil, errNobodyToDiscoverAs
	}
	who, found, err := finder.MCPPersonalDiscoverer(ctx, server.Scope, server.Name)
	if err != nil {
		return nil, fmt.Errorf("read who can discover %s: %w", server.Name, err)
	}
	if !found {
		return nil, errNobodyToDiscoverAs
	}
	identity, err := personalIdentity(server, personal)(ctx, who)
	if err != nil {
		return nil, err
	}
	return &discoverer{who: who, identity: identity}, nil
}

/*
retireMovedDiscoverers ends every session discovered as a person who is no
longer the one to discover as: they changed their credential, removed it, or
somebody configured theirs since. The pass below discovers it again. A session
opened as an identity the person has moved on from would be refused on its next
request, and one opened as a person who left would keep their catalogue.

A store that cannot say keeps the session: not knowing is not a change. It
reports whether any session ended, so the catalogue is published without the
tools learned as them even when nobody takes their place.
*/
func (r *reconciler) retireMovedDiscoverers(ctx context.Context, wanted map[string]domain.MCPServer) bool {
	personal, _ := r.servers.(MCPUserCredentialStore)
	retired := false
	for name, was := range r.discovering {
		server, still := wanted[name]
		if !still {
			continue
		}
		now, err := r.discovererOf(ctx, r.withProtocol(server), personal)
		switch {
		case errors.Is(err, errNobodyToDiscoverAs):
		case err != nil:
			slog.Warn("could not read who discovers a tool server; keeping its session", "server", name, "err", err)
			continue
		case *now == was:
			continue
		}
		if err := r.catalog.RemoveServer(name); err != nil {
			slog.Error("could not disconnect a tool server", "server", name, "err", err)
		}
		delete(r.connected, name)
		delete(r.generations, name)
		delete(r.discovering, name)
		retired = true
	}
	return retired
}

// discoveredAs remembers who the server was discovered as, and owes the trail
// a record the first time this worker discovers it as that person: once per
// server and person for the life of the worker, whoever came between.
func (r *reconciler) discoveredAs(server domain.MCPServer, as *discoverer) {
	if as == nil {
		delete(r.discovering, server.Name)
		return
	}
	r.discovering[server.Name] = *as
	key := server.Name + "\x00" + string(as.who)
	if _, said := r.announced[key]; said {
		return
	}
	r.announced[key] = struct{}{}
	r.owedTrail[key] = admin.MCPDiscoveredOnBehalf{
		Server: server.Name, Scope: server.Scope, Principal: as.who, Holder: r.holder,
	}
}

// settleTrail writes every record owed, and keeps each that fails for the
// next pass: a person who discovered later takes nobody's place in it.
func (r *reconciler) settleTrail(ctx context.Context) {
	trail, ok := r.servers.(discoveryTrail)
	if !ok {
		clear(r.owedTrail)
		return
	}
	for key, d := range r.owedTrail {
		if err := trail.RecordMCPDiscoveredOnBehalf(ctx, d); err != nil {
			slog.Warn("could not record a discovery on a person's behalf; will try again",
				"server", d.Server, "err", err)
			continue
		}
		delete(r.owedTrail, key)
	}
}

// discoveryOnly is a session discovered as a person: it lists tools and never
// calls one. Every call reaches here only with no person to run it as.
type discoveryOnly struct {
	tools.Session
	server string
}

func (d discoveryOnly) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return nil, credentialFailure(tools.CodeMCPPersonalCredentialCaller, d.server,
		"this MCP server requires a personal credential, but the run has no person to resolve one for", nil)
}

// refusingDiscovery turns a 401 or 403 to a request made for nobody into
// errDiscoveryRefused, so the reconciler can tell a server that wants a person
// from one that is down. A person's request passes as it came.
type refusingDiscovery struct{ base http.RoundTripper }

func (d refusingDiscovery) RoundTrip(r *http.Request) (*http.Response, error) {
	res, err := d.base.RoundTrip(r)
	if err != nil || tools.IsInvocation(r.Context()) {
		return res, err
	}
	if res.StatusCode != http.StatusUnauthorized && res.StatusCode != http.StatusForbidden {
		return res, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	_ = res.Body.Close()
	return nil, fmt.Errorf("%w: HTTP %d", errDiscoveryRefused, res.StatusCode)
}

// discoveryClient is the client a server that takes each person's credential
// is reached with: refusals told apart, and, discovering as a person, every
// request made as them.
func discoveryClient(client *http.Client, policy credentialPolicy) *http.Client {
	if !policy.requirePersonal || client == nil {
		return client
	}
	if policy.discoverAs != nil {
		return onBehalfOf(client, policy.discoverAs.who, policy.discoverAs.identity)
	}
	out := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	out.Transport = refusingDiscovery{base: base}
	return &out
}
