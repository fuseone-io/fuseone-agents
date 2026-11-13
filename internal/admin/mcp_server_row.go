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
