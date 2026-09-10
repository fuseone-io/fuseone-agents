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
func TestChannelFacts_offersExactlyWhatItIsFor(t *testing.T) {
	t.Parallel()

	/*
		The whole exported surface, listed.

		A denylist of six names was the wrong shape: it says which acts are
		forbidden today, and a method added tomorrow — or one already there and
		not thought about, as `Secrets` was — walks straight past it. The
		boundary is what this type may do, so the test is that list and nothing
		else, and adding a method means deciding here which side it is on.
	*/
	want := map[string]string{
		"List":            "the connections and the conversations on them",
		"Identities":      "which accounts are bound, including the broken rows",
		"PrincipalFor":    "who an account speaks for",
		"AccountsOn":      "where people are reachable on a connection",
		"SeenAccounts":    "which accounts have been seen",
		"MarkAccountSeen": "that an account was seen, which governs nothing",
	}

	facts := reflect.TypeOf(&admin.ChannelFacts{})
	for i := range facts.NumMethod() {
		name := facts.Method(i).Name
		if _, expected := want[name]; !expected {
			t.Errorf("ChannelFacts offers %s, which nothing here decided it should", name)
		}
		delete(want, name)
	}
	for name, what := range want {
		t.Errorf("ChannelFacts cannot answer %s, which is %s", name, what)
	}
}

/*
And the acts that govern are still reachable where they belong, so the split is
a boundary and not a deletion.
*/
func TestChannels_keepsEveryActThatConfigures(t *testing.T) {
	t.Parallel()

	channels := reflect.TypeOf(&admin.Channels{})
	for name, what := range map[string]string{
		"PutChannel":         "configures a connection, and its credentials",
		"DeleteChannel":      "removes a connection and every conversation on it",
		"PutConversation":    "points a scope's runs at a conversation",
		"DeleteConversation": "stops a scope reporting somewhere",
		"BindIdentity":       "grants one person's authority to a channel account",
		"UnbindIdentity":     "withdraws it",
	} {
		if _, offered := channels.MethodByName(name); !offered {
			t.Errorf("Channels no longer offers %s, which %s", name, what)
		}
	}
}

/*
The credentials are the door's, and the door's alone.

`Secrets` hands back the token this installation posts as and the secret that
decides which requests are genuine. It sat on the type three read-only processes
hold, which is not a fact about channels an ordinary reader should have — the
boundary said "cannot configure" and quietly meant "except it can read every
credential you own".
*/
func TestChannelDoor_isTheOnlyOneThatReadsCredentials(t *testing.T) {
	t.Parallel()

	if _, offered := reflect.TypeOf(&admin.ChannelFacts{}).MethodByName("Secrets"); offered {
		t.Error("ChannelFacts reads channel credentials")
	}
	if _, offered := reflect.TypeOf(&admin.ChannelDoor{}).MethodByName("Secrets"); !offered {
		t.Error("the door cannot read the secret it has to verify requests with")
	}
	// And the door configures nothing either: it is reached by a stranger.
	if _, offered := reflect.TypeOf(&admin.ChannelDoor{}).MethodByName("BindIdentity"); offered {
		t.Error("the door can bind an identity")
	}
}
