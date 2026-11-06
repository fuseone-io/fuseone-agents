package admin_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
A read of one connection says what the listing says about it.

The listing is the older answer and the one the screen and the runtime agree
with, so it is what a shorter path is held to, on the rows that are easy to lose
by asking narrowly: a conversation written before the connection joined the key,
one switched off, a binding stored where it means nothing, one that cannot be
read, and neighbours whose names a careless match would take for this one.
*/
func TestNamed_connectionWithAwkwardRows_answersWhatTheListingAnswers(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, onlySlack{})
	ctx := context.Background()
	const target = "acme_slack"

	for _, name := range []string{target, "acmexslack", "acme_slack2", "acme_slack/nested"} {
		if err := channels.PutChannel(ctx, admin.ChannelWrite{
			Channel: admin.Channel{
				Name: name, Kind: "slack", Workspace: "W " + name,
				DeliveryMode: channel.DeliveryHTTP, Enabled: true,
			},
			Credentials: channel.Credentials{Token: "bot-" + name, Signing: "sign-" + name},
			By:          "usr_ana", Governs: true,
		}); err != nil {
			t.Fatalf("put %s: %v", name, err)
		}
		if err := channels.PutConversation(ctx, name, admin.Conversation{
			ID: "C-" + name, Scope: domain.Scope{Company: "acme"}, Enabled: true,
		}, "usr_ana"); err != nil {
			t.Fatalf("put conversation on %s: %v", name, err)
		}
		if err := channels.BindIdentity(ctx, admin.ChannelIdentity{
			Channel: name, Account: "U1", Principal: "usr_ana",
		}, "usr_ana"); err != nil {
			t.Fatalf("bind on %s: %v", name, err)
		}
	}

	company := domain.Scope{Company: "acme"}
	for _, row := range []settings.Setting{{
		// Written before the connection joined the key: named by the id alone.
		ScopeKind: settings.ScopeCompany, Scope: company, Kind: channel.KindConversation, Name: "C-LEGACY",
		Value: []byte(`{"channel":"acme_slack"}`), Enabled: true,
	}, {
		ScopeKind: settings.ScopeCompany, Scope: company, Kind: channel.KindConversation, Name: "C-OFF",
		Value: []byte(`{"channel":"acme_slack"}`), Enabled: false,
	}, {
		// Stored where it grants nobody anything.
		ScopeKind: settings.ScopeCompany, Scope: company, Kind: admin.KindChannelIdentity, Name: target + "/U-MISPLACED",
		Value: []byte(`{"channel":"acme_slack","account":"U-MISPLACED","principal":"usr_bia"}`), Enabled: true,
	}, {
		ScopeKind: settings.ScopeInstallation, Kind: admin.KindChannelIdentity, Name: target + "/U-BROKEN",
		Value: []byte(`{"channel":"somewhere-else"}`), Enabled: true,
	}} {
		if err := store.Put(ctx, row); err != nil {
			t.Fatalf("seed %s: %v", row.Name, err)
		}
	}

	listed, err := channels.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	all, err := channels.Identities(ctx)
	if err != nil {
		t.Fatalf("Identities: %v", err)
	}
	var want admin.Channel
	for _, one := range listed {
		if one.Name == target {
			want = one
		}
	}
	wantBound := []admin.ChannelIdentity{}
	for _, id := range all {
		if id.Channel == target {
			wantBound = append(wantBound, id)
		}
	}
	// Anchors: a comparison of two empty answers would pass with the read broken.
	// Four bindings, not three: a key is split at its first slash, so the listing
	// files acme_slack/nested's binding under acme_slack, and this read has to agree.
	if len(want.Conversations) != 3 || len(wantBound) != 4 {
		t.Fatalf("seeded %d conversations and %d identities, want 3 and 4",
			len(want.Conversations), len(wantBound))
	}

	got, bound, err := channels.Named(ctx, target)
	if err != nil {
		t.Fatalf("Named: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Named answers\n %+v\nthe listing answers\n %+v", got, want)
	}
	if !reflect.DeepEqual(bound, wantBound) {
		t.Errorf("identities\n %+v\nthe listing answers\n %+v", bound, wantBound)
	}
}

func TestNamed_nameNobodyConfigured_isUnknown(t *testing.T) {
	pool := freshPool(t)
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)), onlySlack{})
	_, _, err := channels.Named(context.Background(), "nothing")
	if !errors.Is(err, admin.ErrChannelUnknown) {
		t.Fatalf("err = %v, want ErrChannelUnknown", err)
	}
}

// A listing shows the rest of the installation around a secret it cannot open.
// This read has nothing else to show, and a client told a credential is gone
// would write it back.
func TestNamed_ownSecretWillNotOpen_failsRatherThanSayingItIsAbsent(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, onlySlack{})
	ctx := context.Background()
	if err := channels.PutChannel(ctx, admin.ChannelWrite{
		Channel:     admin.Channel{Name: "sealed", Kind: "slack", DeliveryMode: channel.DeliveryHTTP, Enabled: true},
		Credentials: channel.Credentials{Token: "bot"}, By: "usr_ana", Governs: true,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`update settings set secret = '\x00'::bytea where kind = 'channel' and name = 'sealed'`); err != nil {
		t.Fatalf("break the ciphertext: %v", err)
	}
	if _, _, err := channels.Named(ctx, "sealed"); err == nil || errors.Is(err, admin.ErrChannelUnknown) {
		t.Fatalf("err = %v, want the failure to open", err)
	}
}
