package admin_test

import (
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/admin"
)

/*
A process that only reads holds a type that cannot configure.

Three of them do: the socket door noticing which accounts have been seen, the
fan-out asking where people are reachable, the consumer asking who an account
speaks for. Given the type that configures, none of them was configuring — they
were trusted not to, and a later edit could do it by mistake with nothing in the
way.

Asserted by name rather than by shape, because the point is which acts are
absent. A method added to the wrong type is how the boundary comes back, and it
comes back silently: everything still compiles.
*/
func TestChannelFacts_offersNoActThatConfigures(t *testing.T) {
	t.Parallel()

	governed := map[string]string{
		"PutChannel":         "configures a connection, and its credentials",
		"DeleteChannel":      "removes a connection and every conversation on it",
		"PutConversation":    "points a scope's runs at a conversation",
		"DeleteConversation": "stops a scope reporting somewhere",
		"BindIdentity":       "grants one person's authority to a channel account",
		"UnbindIdentity":     "withdraws it",
	}

	facts := reflect.TypeOf(&admin.ChannelFacts{})
	for name, what := range governed {
		if _, offered := facts.MethodByName(name); offered {
			t.Errorf("ChannelFacts offers %s, which %s", name, what)
		}
	}

	// And the whole thing is still reachable where it belongs, so the split is
	// a boundary rather than a deletion.
	channels := reflect.TypeOf(&admin.Channels{})
	for name := range governed {
		if _, offered := channels.MethodByName(name); !offered {
			t.Errorf("Channels no longer offers %s", name)
		}
	}
	for _, name := range []string{
		"List", "Identities", "PrincipalFor", "AccountsOn", "SeenAccounts",
		"MarkAccountSeen",
	} {
		if _, offered := facts.MethodByName(name); !offered {
			t.Errorf("ChannelFacts cannot %s, which is what it is for", name)
		}
	}
}
