package admin

import (
	"errors"

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
var ErrInstallationArea = errors.New(
	"admin: the installation is the scope above every company and has no area")

/*
announcesOnly strips what a conversation for the whole installation cannot use.

Coerced rather than refused, in the same shape as every other field a choice
does not consume here. The agent goes too, even though it also narrows what the
room hears: agent ids belong to a company, so one named here could not be
checked against anything, and the endpoint that validates it has no scope to
look in.

What survives is what the room is for — the events it hears, and whether it also
tells the people who may decide, which is the whole reason to have one.
*/
func announcesOnly(conv Conversation) (Conversation, string) {
	conv.Mode = channel.ConversationAnnounce
	conv.Agent, conv.RunAs, conv.ThreadContext = "", "", false
	conv.Sources = nil
	return conv, conv.Mode
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
