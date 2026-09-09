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
