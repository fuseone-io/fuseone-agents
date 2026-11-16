package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// ErrConversationOutsideConnection is a connection or a conversation that
// would leave a conversation on a connection whose scope does not contain it.
var ErrConversationOutsideConnection = errors.New(
	"admin: a conversation is outside the scope of the connection it names")

/*
connectionStoredAt is where a connection write lands (NT-019, phase 4 writers): the
scope its writer named, the installation when none was named, as every write
was before areas had their own. A scope no reader knows is refused before
anything is written, since every reader would refuse the row.
*/
func connectionStoredAt(kind settings.ScopeKind, stored domain.Scope) (settings.ScopeKind, domain.Scope, domain.Scope, error) {
	if kind == "" {
		kind = settings.ScopeInstallation
	}
	serves, known := channel.ConnectionScope(kind, stored)
	if !known {
		return "", domain.Scope{}, domain.Scope{}, fmt.Errorf("%w: %s %s", channel.ErrConnectionScopeUnknown, kind, stored)
	}
	return kind, stored, serves, nil
}

// trailScope is the scope a connection's acts are recorded at: the
// installation's as before, an area's or a company's own.
func trailScope(kind settings.ScopeKind, stored domain.Scope) domain.Scope {
	if kind == settings.ScopeInstallation {
		return domain.Scope{}
	}
	return stored
}

// refuseConversationsOutside stops a connection being stored where it would
// carry nothing for a conversation already attached to its name. Read under
// the connection's lock, which every conversation write takes too.
func (c *Channels) refuseConversationsOutside(
	ctx context.Context, conn settings.DB, name string, serves domain.Scope,
) error {
	stored, err := c.settings.ListByChannelTx(ctx, conn, channel.KindConversation, name)
	if err != nil {
		return fmt.Errorf("admin: read conversations of %s: %w", name, err)
	}
	for _, conv := range conversationsOf(name, stored) {
		if !serves.Contains(conv.Scope) {
			return fmt.Errorf("%w: %s", ErrConversationOutsideConnection, conv.ID)
		}
	}
	return nil
}

// channelStoredAtOn is where a connection's row lives, read on db. found is
// false for a name no row holds.
func channelStoredAtOn(
	ctx context.Context, db settings.DB, name string,
) (kind settings.ScopeKind, stored domain.Scope, found bool, err error) {
	var k, company, area string
	err = db.QueryRow(ctx, `
		select scope_kind, company_id, area_id from settings
		where kind = $1 and name = $2`, string(channel.KindChannel), name,
	).Scan(&k, &company, &area)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings.ScopeInstallation, domain.Scope{}, false, nil
	}
	if err != nil {
		return "", domain.Scope{}, false, fmt.Errorf("admin: find channel %s: %w", name, err)
	}
	return settings.ScopeKind(k), domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}, true, nil
}

// ChannelStoredAt is where a connection's row lives, the installation's for a
// name no row holds: where a door authorizes an act on it (NT-019).
func (c *Channels) ChannelStoredAt(ctx context.Context, name string) (settings.ScopeKind, domain.Scope, error) {
	kind, stored, _, err := channelStoredAtOn(ctx, c.pool, name)
	return kind, stored, err
}
