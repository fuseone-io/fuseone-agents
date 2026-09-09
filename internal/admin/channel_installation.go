package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A conversation that speaks for the whole installation.

The scope above every company contains every one of them, so such a room hears
about a run in any company — which is the point, and is why it is the answer
for an area nobody has mapped to a channel of its own.

It is also why it may not start anything. Containment is right for hearing and
wrong for asking: a room that hears about every company is a reasonable thing to
configure, and one from which anybody could start an agent in every company is a
different grant entirely. The read side refuses it twice over; this is the third
lock, on the way in, so the configuration never describes something that will
not happen.
*/

// ErrInstallationArea means a conversation named the installation and an area.
//
// The two together reach nothing: containment short circuits on the sentinel
// and requires the area to be empty, so the row announces to no scope at all
// while looking configured — the quietest way to own a room that never speaks.
// ErrUnknownMode means a conversation named a mode this version cannot honour.
//
// Refused rather than normalised. Read as "mentions" — which is what the
// display normalisation answers for anything it does not recognise — a room a
// newer version had set to start nothing would come back startable by anybody
// who can type in it.
var ErrUnknownMode = errors.New(
	"admin: that conversation mode is not one this version knows")

var ErrInstallationArea = errors.New(
	"admin: the installation is the scope above every company and has no area")

/*
announcesOnly strips what a conversation that starts nothing cannot use.

Coerced rather than refused, in the same shape as every other field a choice
does not consume here. The agent goes too, even though on the installation it
would also narrow what the room hears: agent ids belong to a company, so one
named there could not be checked against anything, and the endpoint that
validates it has no scope to look in. At an ordinary scope the reason is
simpler — a conversation that starts nothing has no agent to start, and a
binding left behind is a field nothing reads until somebody sets the room back
to taking mentions and it silently comes into force.

What survives is what the room is for — the events it hears, and whether it also
tells the people who may decide, which is the whole reason to have one.
*/
func announcesOnly(conv Conversation) (Conversation, string) {
	conv.Mode = channel.ConversationAnnounce
	conv.Agent, conv.RunAs, conv.ThreadContext = "", "", false
	conv.Sources = nil
	return conv, conv.Mode
}

/*
ErrInstallationAuthority means the caller may not touch what carries the room.

A conversation for the whole installation needs authority over the installation
to exist and to be removed. The connection under it is the same reach by another
route: removing it takes the room with it, and disabling or re-credentialling it
silences the room or sends its messages out through another token.
*/
var ErrInstallationAuthority = errors.New(
	"admin: that connection carries the room for the whole installation")

/*
Invalid answers whether an error is the request's fault.

The alternative is what this replaced: everything that was not one named
sentinel became "bad request", so a lock that could not be taken, a database
that was away or a vault that would not open came back as a validation failure
— sending somebody to fix a field that was never wrong, and putting operational
text in a reply to a browser.

A sentinel missing from this list answers false, and the caller reports a
failure rather than a refusal. That is the safe way round: loud, and never a
refusal somebody has no way to act on.
*/
func Invalid(err error) bool {
	for _, sentinel := range []error{
		ErrNoChannelKind, ErrNoCompany, ErrInstallationArea, ErrUnknownMode,
		ErrNoWatchSource, ErrNoWatchAgent, ErrNoWatchRunAs, ErrConversationMapped,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

/*
lockChannel serialises every write about one connection.

The precondition and the write have to be one decision. Checked before the
transaction, a curator passes the check on a connection carrying no room, the
room is attached by somebody who may, and the write lands anyway — a 204 for
exactly the act that had just been refused.

An advisory lock rather than a row lock, because the two writes touch different
rows: the connection's and the conversation's. What they share is the name.
*/
func lockChannel(ctx context.Context, conn settings.DB, name string) error {
	if _, err := conn.Exec(ctx,
		`select pg_advisory_xact_lock(hashtext($1))`, "channel:"+name); err != nil {
		return fmt.Errorf("admin: lock the connection %s: %w", name, err)
	}
	return nil
}

/*
guardInstallationRoom refuses a write about a connection that carries the room,
from a caller who does not govern the installation.

Read from the conversation rows rather than from the assembled listing: that one
hangs conversations off the connections that exist, so a room whose connection
is gone — restored, migrated, or left behind by a half-finished delete — is
invisible to it, and recreating the connection would quietly adopt it.
*/
func (c *Channels) guardInstallationRoom(
	ctx context.Context, conn settings.DB, name string, governs bool,
) error {
	if err := lockChannel(ctx, conn, name); err != nil {
		return err
	}
	if governs {
		return nil
	}
	stored, err := c.settings.ListTx(ctx, conn, channel.KindConversation)
	if err != nil {
		return fmt.Errorf("admin: list conversations: %w", err)
	}
	for _, conv := range conversationsOf(name, stored) {
		if conv.Scope.IsInstallation() {
			return fmt.Errorf("%w: %s", ErrInstallationAuthority, name)
		}
	}
	return nil
}

// conversationScopeKind is where a conversation is stored.
//
// Written once because it is asked twice — when a conversation is saved and
// when it is removed — and the two disagreeing would make a delete match
// nothing, report no error, and leave the room receiving.
func conversationScopeKind(scope domain.Scope) settings.ScopeKind {
	if scope.Area != "" {
		return settings.ScopeArea
	}
	return settings.ScopeCompany
}
