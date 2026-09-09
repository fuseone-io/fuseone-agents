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
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)))

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
	channels := admin.NewChannels(pool, store)

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
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)))

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
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)))

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
