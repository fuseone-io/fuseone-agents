package channel_test

import (
	"testing"

	"github.com/fuseone/agents/internal/channel"
)

/*
Which modes let a message start a run.

Read as a denylist — anything that is not "watch" starts from mentions — the
answer for a mode nobody has added yet is *yes*. That is the wrong default for
a question about who may start work: a value the platform does not recognise
should reach for the narrowest reading, not the widest, and a mode added later
would start runs on the day it was named.

The empty string is the one exception and it is deliberate: conversations
configured before modes existed stored nothing, and they took mentions.
*/
func TestStartsFromMentions_everyMode_saysWhetherAMessageMayStart(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		mode     string
		mentions bool
		watch    bool
	}{
		{channel.ConversationMentions, true, false},
		{channel.ConversationWatch, false, true},
		{channel.ConversationBoth, true, true},
		// Announces and starts nothing. Both answers are no.
		{channel.ConversationAnnounce, false, false},
		// Configured before modes existed.
		{"", true, false},
		// Not a mode this platform knows. It must not start anything.
		{"whatever-comes-next", false, false},
	} {
		if got := channel.StartsFromMentions(c.mode); got != c.mentions {
			t.Errorf("StartsFromMentions(%q) = %v, want %v", c.mode, got, c.mentions)
		}
		if got := channel.StartsFromWatch(c.mode); got != c.watch {
			t.Errorf("StartsFromWatch(%q) = %v, want %v", c.mode, got, c.watch)
		}
	}
}

/*
Two connections cannot produce one key.

Joined with a separator, a connection called "workspace" holding "team/C" and
one called "workspace/team" holding "C" are the same string — and in one scope
that is one row, so the second write is the first one's grave. Both halves are
names somebody typed or a vendor chose; neither can be promised free of the
separator.
*/
func TestConversationKey_partsThatShareASeparator_areStillTwoKeys(t *testing.T) {
	t.Parallel()

	first := channel.ConversationKey("workspace", "team/C-SAME")
	second := channel.ConversationKey("workspace/team", "C-SAME")
	if first == second {
		t.Fatalf("both connections key to %q", first)
	}
	// And each still says which conversation it is.
	if got := channel.ConversationID("workspace", first); got != "team/C-SAME" {
		t.Errorf("id = %q, want the whole id back", got)
	}
	if got := channel.ConversationID("workspace/team", second); got != "C-SAME" {
		t.Errorf("id = %q, want the whole id back", got)
	}
}

/*
A row from before the key carried a connection is read as the id it is.

Its name has no prefix to strip. An id that merely looks like a key — one that
happens to begin with this connection's prefix — is still an id, so the prefix
is matched whole and never sniffed for.
*/
func TestConversationID_aNameWithoutThisConnectionsPrefix_isTheIdItself(t *testing.T) {
	t.Parallel()

	for _, one := range []struct{ channel, name, want string }{
		{"acme-slack", "C07", "C07"},
		{"acme-slack", "acme-slack/C07", "acme-slack/C07"},
		{"acme-slack", "10:acme-slack/C07", "C07"},
		{"acme-slack", "9:acme-slac/C07", "9:acme-slac/C07"},
	} {
		if got := channel.ConversationID(one.channel, one.name); got != one.want {
			t.Errorf("ConversationID(%q, %q) = %q, want %q",
				one.channel, one.name, got, one.want)
		}
	}
}
