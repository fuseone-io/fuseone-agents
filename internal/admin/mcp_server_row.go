package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
What a worker reads about the server it connected, it reads of that row
(NT-019).

A worker reads the servers, then opens each one's credential, then may write a
refreshed OAuth grant back. A name is unique, but not forever: removed from one
area and stored again in another between those steps, it is another server,
and its credential must not travel with the URL the worker read. So every
such read and write names the row, by where it is stored and the revision the
worker read.
*/

// ErrMCPServerChanged is a server whose row is not the one a worker read: gone
// from where it was stored, or revised since. The worker's next pass reads it
// again.
var ErrMCPServerChanged = errors.New("admin: MCP server changed since it was read")

// MCPServerCredentials opens what a server was given, from the row the worker
// read: stored where server.Scope says, at the revision server.UpdatedAt.
func (i *Integrations) MCPServerCredentials(
	ctx context.Context, server domain.MCPServer,
) (domain.MCPCredentials, error) {
	kind, stored := storedAt(server.Scope)
	set, err := i.settings.Reveal(ctx, kind, stored, settings.KindMCPServer, server.Name)
	switch {
	case errors.Is(err, settings.ErrNotFound):
		return domain.MCPCredentials{}, fmt.Errorf("%w: %s is no longer stored at %s", ErrMCPServerChanged, server.Name, server.Scope)
	case err != nil:
		return domain.MCPCredentials{}, err
	case !set.UpdatedAt.Equal(server.UpdatedAt):
		return domain.MCPCredentials{}, fmt.Errorf("%w: %s was revised", ErrMCPServerChanged, server.Name)
	}
	return domain.ReadMCPCredentials(set.Secret), nil
}

// MCPOAuthRefresh is a grant a worker refreshed, for the server row it
// connected: Scope is where that row is stored, zero for a server no row
// describes. Principal is the person whose own credential it was, or empty for
// the server's.
type MCPOAuthRefresh struct {
	Server    string
	Scope     domain.Scope
	Principal domain.UserID
	Was, Next domain.MCPOAuthGrant
}

// sameServerRow compares where two descriptions of one name say the server is
// stored. Zero, a server no row describes, is the installation's.
func sameServerRow(a, b domain.Scope) bool {
	installation := domain.Scope{Company: domain.Installation}
	if a == (domain.Scope{}) {
		a = installation
	}
	if b == (domain.Scope{}) {
		b = installation
	}
	return a == b
}

// serverRow is what a person's credential stores about its server's row.
func serverRow(kind settings.ScopeKind, stored domain.Scope) *requestedAt {
	return &requestedAt{ScopeKind: string(kind), Company: string(stored.Company), Area: string(stored.Area)}
}

// serverRuntime is where the server a credential was stored for is: the
// installation's for one stored before credentials named a row.
func (c storedMCPUserCredential) serverRuntime() domain.Scope {
	if c.ServerAt == nil || c.ServerAt.ScopeKind == "" {
		return domain.Scope{Company: domain.Installation}
	}
	return storedRuntimeScope(c.ServerAt.ScopeKind, c.ServerAt.Company, c.ServerAt.Area)
}
