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

One snapshot, because the answer takes three statements and a client compares all
of it with what it wants. Read apart, a write landing between them describes a
connection that never existed: this one's address with conversations it does not
have yet, or a removal that reads as a failure halfway through.
*/
func (c *Channels) Named(ctx context.Context, name string) (Channel, []ChannelIdentity, error) {
	var found Channel
	var bound []ChannelIdentity
	err := c.settings.ReadSnapshot(ctx, func(db settings.DB) error {
		var err error
		found, bound, err = c.namedOn(ctx, db, name)
		return err
	})
	if err != nil {
		return Channel{}, nil, err
	}
	return found, bound, nil
}

func (c *Channels) namedOn(ctx context.Context, db settings.DB, name string) (Channel, []ChannelIdentity, error) {
	row, err := c.settings.RevealOn(ctx, db, settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, name)
	if errors.Is(err, settings.ErrNotFound) {
		return Channel{}, nil, fmt.Errorf("%w: %s", ErrChannelUnknown, name)
	}
	if err != nil {
		return Channel{}, nil, fmt.Errorf("admin: read channel %s: %w", name, err)
	}
	var conn channel.Connection
	_ = json.Unmarshal(row.Value, &conn)
	creds := channel.ReadCredentials(row.Secret)

	held, err := c.attachedOn(ctx, db, name)
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
		Revision:          c.revisionOf(row, held),
		Conversations:     held.Conversations,
	}, held.Identities, nil
}

// attached is what a channel's answer carries besides the connection, as its revision counts
// it. Its writers take the connection's lock, so read under that lock it is exact. None and
// an empty list are the same answer, and name the same revision.
type attached struct {
	Conversations []Conversation    `json:"conversations,omitempty"`
	Identities    []ChannelIdentity `json:"identities,omitempty"`
}

func (c *Channels) attachedOn(ctx context.Context, db settings.DB, name string) (attached, error) {
	conversations, err := c.settings.ListByChannelTx(ctx, db, channel.KindConversation, name)
	if err != nil {
		return attached{}, fmt.Errorf("admin: read conversations of %s: %w", name, err)
	}
	bound, err := c.identitiesOn(ctx, db, name)
	if err != nil {
		return attached{}, err
	}
	return attached{Conversations: conversationsOf(name, conversations), Identities: bound}, nil
}

/*
revisionOf names a connection by its configuration, the document its secrets would be sealed
as, and what is attached to it. The secrets as they would be sealed, so a row still holding a
bare token does not read as changed once it is saved. What is attached, because the answer
carries it: a validator that held still while a conversation arrived would let a client
holding the older answer remove the connection, and the conversation with it.
*/
func (c *Channels) revisionOf(set settings.Setting, held attached) domain.Revision {
	return c.settings.Revision(set, []byte(channel.ReadCredentials(set.Secret).Sealed()), held)
}

// channelRevised is a write's precondition on one connection. Read must run under the
// connection's lock and before anything is judged.
type channelRevised struct {
	c    *Channels
	held attached
}

func (r *channelRevised) read(ctx context.Context, db settings.DB, name string) error {
	var err error
	r.held, err = r.c.attachedOn(ctx, db, name)
	return err
}

func (r *channelRevised) of(expect domain.IfMatch) *revised {
	return &revised{expect: expect, lockedByGuard: true, of: func(set settings.Setting) domain.Revision {
		return r.c.revisionOf(set, r.held)
	}}
}

// identitiesOn reads the bindings keyed under one connection, wherever stored.
func (c *Channels) identitiesOn(ctx context.Context, db settings.DB, name string) ([]ChannelIdentity, error) {
	stored, err := c.settings.ListUnderPrefixTx(ctx, db, KindChannelIdentity, name+"/")
	if err != nil {
		return nil, fmt.Errorf("admin: read identities on %s: %w", name, err)
	}
	out := make([]ChannelIdentity, 0, len(stored))
	for _, row := range stored {
		// The last slash ends the channel's half of a key, so the range also
		// catches the bindings of a connection named under this one's, which are
		// not this one's.
		if keyChannel(row.Name) == name {
			out = append(out, identityOfRow(row))
		}
	}
	return out, nil
}
