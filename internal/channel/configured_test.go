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

Joined with a separator alone, a connection called "workspace" holding "team/C"
and one called "workspace/team" holding "C" are the same string — and in one
scope that is one row, so the second write is the first one's grave. Both halves
are names somebody typed or a vendor chose; neither can be promised free of the
separator.
*/
func TestConversationKey_partsThatShareASeparator_areStillTwoKeys(t *testing.T) {
	t.Parallel()

	first := channel.ConversationKey("workspace", "team/C-SAME")
	second := channel.ConversationKey("workspace/team", "C-SAME")
	if first == second {
		t.Fatalf("both connections key to %q", first)
	}
	for _, one := range []struct{ channel, key, want string }{
		{"workspace", first, "team/C-SAME"},
		{"workspace/team", second, "C-SAME"},
	} {
		got, legible := channel.ConversationIDOf(
			channel.KeyVersionConnection, one.channel, one.key)
		if !legible || got != one.want {
			t.Errorf("id of %q = %q (legible %v), want %q",
				one.key, got, legible, one.want)
		}
	}
}

/*
Which shape a row is in is the row's own word, never its appearance.

A stored id may look like anything a vendor chose — a Teams conversation id
begins with digits and a colon — so a reader deciding by appearance would take
somebody's id apart and answer as a different conversation. And a row claiming
the new shape without carrying it is nobody's conversation rather than a guess.
*/
func TestConversationIDOf_theShapeIsDeclared_notInferred(t *testing.T) {
	t.Parallel()

	for _, one := range []struct {
		name       string
		keyVersion int
		channel    string
		stored     string
		want       string
		legible    bool
	}{
		{"the name is the id", 0, "acme-slack", "C07", "C07", true},
		{"an id that looks like a key is still an id", 0, "acme-slack",
			"10:acme-slack/C07", "10:acme-slack/C07", true},
		{"the new shape is taken apart", channel.KeyVersionConnection,
			"acme-slack", "10:acme-slack/C07", "C07", true},
		{"a row claiming a shape it does not carry is illegible",
			channel.KeyVersionConnection, "acme-slack", "C07", "", false},
	} {
		t.Run(one.name, func(t *testing.T) {
			got, legible := channel.ConversationIDOf(one.keyVersion, one.channel, one.stored)
			if legible != one.legible || got != one.want {
				t.Errorf("= %q, %v; want %q, %v", got, legible, one.want, one.legible)
			}
		})
	}
}
