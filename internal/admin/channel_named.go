package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// ErrChannelUnknown means nobody configured a connection under that name.
var ErrChannelUnknown = errors.New("admin: no such channel")

/*
Named reads one connection, its conversations and the identities bound on it.

Only that connection's secret is opened. List opens one per connection to say
which are stored, which is what a screen showing all of them pays and what a
client refreshing them one at a time must not: decrypting every credential in
the installation in order to read one.

On Channels, not ChannelFacts: this is a configurer's read, and the processes
holding the read-only type have no use for it.

A secret that will not open is an error here, where List reports it as absent. A
client told a credential is gone would write it back; a listing shows the rest of
the installation, and this has nothing else to show.
*/
func (c *Channels) Named(ctx context.Context, name string) (Channel, []ChannelIdentity, error) {
	row, err := c.settings.Reveal(ctx, settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, name)
	if errors.Is(err, settings.ErrNotFound) {
		return Channel{}, nil, fmt.Errorf("%w: %s", ErrChannelUnknown, name)
	}
	if err != nil {
		return Channel{}, nil, fmt.Errorf("admin: read channel %s: %w", name, err)
	}
	var conn channel.Connection
	_ = json.Unmarshal(row.Value, &conn)
	creds := channel.ReadCredentials(row.Secret)

	conversations, err := c.settings.ListByChannel(ctx, channel.KindConversation, name)
	if err != nil {
		return Channel{}, nil, fmt.Errorf("admin: read conversations of %s: %w", name, err)
	}
	bound, err := c.identitiesOn(ctx, name)
	if err != nil {
		return Channel{}, nil, err
	}
	return Channel{
		Name: row.Name, Kind: conn.Kind, Workspace: conn.Workspace,
		DeliveryMode: channel.StoredDeliveryMode(conn.DeliveryMode),
		Enabled:      row.Enabled, HasCredential: creds.Token != "",
		HasSigning: creds.Signing != "", HasAppToken: creds.AppToken != "",
		// Over the document this row would be re-sealed as, so a row still holding
		// a bare token does not appear to change the next time it is saved.
		SecretFingerprint: c.settings.Fingerprint(row, []byte(creds.Sealed())),
		Conversations:     conversationsOf(name, conversations),
	}, bound, nil
}

// identitiesOn reads the bindings keyed under one connection, wherever stored.
func (c *Channels) identitiesOn(ctx context.Context, name string) ([]ChannelIdentity, error) {
	stored, err := c.settings.ListUnderPrefix(ctx, KindChannelIdentity, name+"/")
	if err != nil {
		return nil, fmt.Errorf("admin: read identities on %s: %w", name, err)
	}
	out := make([]ChannelIdentity, 0, len(stored))
	for _, row := range stored {
		// The first slash ends the channel's half of a key, so a connection
		// named with one in it does not own what the range also caught.
		if keyChannel(row.Name) == name {
			out = append(out, identityOfRow(row))
		}
	}
	return out, nil
}
