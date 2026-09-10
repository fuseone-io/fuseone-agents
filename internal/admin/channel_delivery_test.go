package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A way of being reached that this version cannot honour is refused.

Normalised instead, it read as HTTP — the mode with an inbound door — so a typo
was accepted as a configuration, and a connection written by a newer version
came back as one this version opens. The same fail-open the conversation modes
had, one layer down: the door there is a mention, and here it is every ask
Slack posts to the callback.
*/
func TestPutChannel_aDeliveryModeThisVersionDoesNotKnow_isRefused(t *testing.T) {
	pool := freshPool(t)
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)), onlySlack{})

	err := channels.PutChannel(context.Background(), admin.ChannelWrite{
		Channel: admin.Channel{
			Name: "acme-slack", Kind: "slack",
			DeliveryMode: "a-future-mode", Enabled: true,
		},
		By: "usr_ana", Governs: true,
	})
	if !errors.Is(err, admin.ErrUnknownDeliveryMode) {
		t.Fatalf("err = %v, want ErrUnknownDeliveryMode", err)
	}
	// The caller's mistake, so the caller hears about it as one rather than as
	// a failure of this installation.
	if !admin.Invalid(err) {
		t.Error("the refusal is not reported as the request's fault")
	}
}

/*
And one already stored is handed back as it is.

Read through the display normalisation it came back as HTTP, and saving any
unrelated edit from that reading would store "http" and mean it — opening the
inbound door of a connection a newer version had put somewhere else. Empty is
the one value that is translated, because empty is defined: it is a connection
configured before Socket Mode existed.
*/
func TestList_aDeliveryModeThisVersionDoesNotKnow_isNotReadAsHTTP(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, onlySlack{})

	for _, one := range []struct{ name, stored, want string }{
		{"future-slack", `"a-future-mode"`, "a-future-mode"},
		{"legacy-slack", `""`, channel.DeliveryHTTP},
	} {
		if err := store.Put(context.Background(), settings.Setting{
			ScopeKind: settings.ScopeInstallation, Scope: domain.Scope{},
			Kind: channel.KindChannel, Name: one.name,
			Value: []byte(`{"kind":"slack","deliveryMode":` + one.stored + `}`),
			// Restored, migrated or hand-edited: the administration refuses to
			// produce this row, which is why the read has to survive it.
			Enabled: true, UpdatedBy: "restore",
		}); err != nil {
			t.Fatalf("write the %s row: %v", one.name, err)
		}
		if got := oneChannel(t, channels, one.name).DeliveryMode; got != one.want {
			t.Errorf("%s delivery mode = %q, want %q", one.name, got, one.want)
		}
	}
}

/*
An event nothing announces is refused.

The contract's enumeration is checked by the generated server and nowhere else,
so anything reaching the administration another way was stored: a subscription
to silence, which saves cleanly, shows as configured, and never tells anybody
the thing whoever typed it thought they had asked for. The console then cannot
draw it either — an unknown name is in no checkbox, and the form's own schema
refuses to save the conversation at all.
*/
func TestPutConversation_anEventThisVersionDoesNotAnnounce_isRefused(t *testing.T) {
	pool := freshPool(t)
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)), onlySlack{})

	err := channels.PutConversation(context.Background(), "acme-slack",
		admin.Conversation{
			ID: "C-quiet", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "ops"},
			Wants: []string{"parked", "a-future-event"},
		}, "usr_ana")
	if !errors.Is(err, admin.ErrUnknownEvent) {
		t.Fatalf("err = %v, want ErrUnknownEvent", err)
	}
	if !admin.Invalid(err) {
		t.Error("the refusal is not reported as the request's fault")
	}
}

// gate_refusal is not one either. It rides with failed rather than being
// chosen, so asking for it by name asks for something the fan-out never reads.
func TestPutConversation_gateRefusalByName_isRefused(t *testing.T) {
	pool := freshPool(t)
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)), onlySlack{})

	err := channels.PutConversation(context.Background(), "acme-slack",
		admin.Conversation{
			ID: "C-gate", Enabled: true,
			Scope: domain.Scope{Company: "acme", Area: "ops"},
			Wants: []string{"gate_refusal"},
		}, "usr_ana")
	if !errors.Is(err, admin.ErrUnknownEvent) {
		t.Fatalf("err = %v, want ErrUnknownEvent", err)
	}
}

/*
Nothing inbound may be configured on a connection this version cannot reach.

The runtime opens no door there, so the rule would do nothing today — and that
is the danger: stored now, a mention rule comes into force the day a version
that recognises the delivery mode reads it, with nobody having decided anything
in between. A room on such a connection may only announce.
*/
func TestPutConversation_onAConnectionThisVersionCannotReach_mayOnlyAnnounce(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, onlySlack{})
	ctx := context.Background()

	// Restored, migrated, or written by a newer version: the administration
	// refuses to produce it, which is why this has to survive it.
	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Scope: domain.Scope{},
		Kind: channel.KindChannel, Name: "future-slack",
		Value:   []byte(`{"kind":"slack","deliveryMode":"a-future-mode"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the connection: %v", err)
	}

	scope := domain.Scope{Company: "acme", Area: "ops"}
	err := channels.PutConversation(ctx, "future-slack", admin.Conversation{
		ID: "C-mentions", Enabled: true, Scope: scope, Wants: []string{"parked"},
	}, "usr_ana")
	if !errors.Is(err, admin.ErrConnectionOnlyAnnounces) {
		t.Fatalf("err = %v, want ErrConnectionOnlyAnnounces", err)
	}

	// Announcing is what such a room is for, and it is still allowed.
	if err := channels.PutConversation(ctx, "future-slack", admin.Conversation{
		ID: "C-reports", Enabled: true, Scope: scope, Wants: []string{"parked"},
		Mode: channel.ConversationAnnounce,
	}, "usr_ana"); err != nil {
		t.Fatalf("announcing was refused too: %v", err)
	}
}

/*
Unreadable is unreachable, and so is a vendor nothing here can talk to.

Two shapes slipped through the first version of this guard. A connection whose
value this binary cannot decode said nothing about itself — least of all that it
was safe — and was skipped. And one naming a vendor with no driver was treated
as reachable, though the runtime refuses to build it: an inbound rule written
under it lies dormant and comes into force the day somebody adds the driver.
*/
func TestPutConversation_onAConnectionThatCannotBeBuilt_mayOnlyAnnounce(t *testing.T) {
	for _, one := range []struct{ name, value string }{
		{"unreadable-slack", `{"kind":"slack","deliveryMode":["http"]}`},
		{"teams-someday", `{"kind":"teams","deliveryMode":"http"}`},
	} {
		t.Run(one.name, func(t *testing.T) {
			pool := freshPool(t)
			store := settings.NewStore(pool, testVault(t))
			channels := admin.NewChannels(pool, store, onlySlack{})
			ctx := context.Background()

			if err := store.Put(ctx, settings.Setting{
				ScopeKind: settings.ScopeInstallation, Scope: domain.Scope{},
				Kind: channel.KindChannel, Name: one.name,
				Value:   []byte(one.value),
				Enabled: true, UpdatedBy: "restore",
			}); err != nil {
				t.Fatalf("write the connection: %v", err)
			}

			err := channels.PutConversation(ctx, one.name, admin.Conversation{
				ID: "C-mentions", Enabled: true, Wants: []string{"parked"},
				Scope: domain.Scope{Company: "acme", Area: "ops"},
			}, "usr_ana")
			if !errors.Is(err, admin.ErrConnectionOnlyAnnounces) {
				t.Fatalf("err = %v, want ErrConnectionOnlyAnnounces", err)
			}
		})
	}
}

// And a vendor this binary does have stays configurable, which is the whole
// point of asking the driver table rather than a list written here.
func TestPutConversation_onAConnectionThatCanBeBuilt_isConfigured(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, onlySlack{})
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Scope: domain.Scope{},
		Kind: channel.KindChannel, Name: "acme-slack",
		Value:   []byte(`{"kind":"slack","deliveryMode":"http"}`),
		Enabled: true, UpdatedBy: "usr_ana",
	}); err != nil {
		t.Fatalf("write the connection: %v", err)
	}

	if err := channels.PutConversation(ctx, "acme-slack", admin.Conversation{
		ID: "C-mentions", Enabled: true, Wants: []string{"parked"},
		Scope: domain.Scope{Company: "acme", Area: "ops"},
	}, "usr_ana"); err != nil {
		t.Fatalf("PutConversation: %v", err)
	}
}

type onlySlack struct{}

func (onlySlack) Kinds() []string { return []string{"slack"} }

/*
The same rule from the other side: the connection arrives second.

A conversation may be configured before the connection exists — that has always
worked, and refusing it would make the order of two administrative acts matter.
So the check runs again when the connection arrives. Without it the sequence is
one act apart: write an inbound rule on a name nothing holds yet, then give that
name a vendor nothing can build, and the rule sits there until the driver ships.

An announce-only conversation is no reason to refuse. It starts nothing, and
preparing a connection for a vendor this binary cannot build yet is a reasonable
thing to do.
*/
func TestPutChannel_takingAVendorWithNoDriver_isRefusedWhileConversationsStartRuns(t *testing.T) {
	for _, one := range []struct {
		name    string
		mode    string
		already string
		refused bool
	}{
		{"a conversation that takes mentions", channel.ConversationMentions, "", true},
		{"one that only reports", channel.ConversationAnnounce, "", false},
		{"a connection changing vendor under a mention rule",
			channel.ConversationMentions, "slack", true},
	} {
		t.Run(one.name, func(t *testing.T) {
			pool := freshPool(t)
			store := settings.NewStore(pool, testVault(t))
			channels := admin.NewChannels(pool, store, onlySlack{})
			ctx := context.Background()

			if one.already != "" {
				if err := channels.PutChannel(ctx, admin.ChannelWrite{
					Channel: admin.Channel{
						Name: "someday", Kind: one.already, Enabled: true,
					},
					By: "usr_ana", Governs: true,
				}); err != nil {
					t.Fatalf("configure the connection: %v", err)
				}
			}
			if err := channels.PutConversation(ctx, "someday", admin.Conversation{
				ID: "C07", Enabled: true, Mode: one.mode, Wants: []string{"parked"},
				Scope: domain.Scope{Company: "acme", Area: "ops"},
			}, "usr_ana"); err != nil {
				t.Fatalf("map the conversation: %v", err)
			}

			err := channels.PutChannel(ctx, admin.ChannelWrite{
				Channel: admin.Channel{Name: "someday", Kind: "teams", Enabled: true},
				By:      "usr_ana", Governs: true,
			})
			switch {
			case one.refused && !errors.Is(err, admin.ErrConversationsStartRuns):
				t.Fatalf("err = %v, want ErrConversationsStartRuns", err)
			case !one.refused && err != nil:
				t.Fatalf("PutChannel: %v", err)
			}
			if !one.refused {
				return
			}
			// And nothing was stored under the vendor that was refused, which
			// is what the error alone does not say. Absent is a right answer
			// here: the first case had no connection to begin with.
			listed, err := channels.List(ctx)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			for _, got := range listed {
				if got.Name == "someday" && got.Kind == "teams" {
					t.Error("the connection took the vendor anyway")
				}
			}
		})
	}
}
