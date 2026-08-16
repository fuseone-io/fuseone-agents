package admin_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/settings"
)

/*
Who an account speaks for, and the three answers.

Nobody is the ordinary one: most people in a workspace have never been bound,
and a message from one of them is somebody the platform does not know rather
than something going wrong. What must never happen is guessing — an unbound
account acts as no one at all.
*/

func TestPrincipalFor_anAccountNobodyBound_isNobodyAndNotAnError(t *testing.T) {
	channels, _ := boundChannels(t)

	who, bound, err := channels.PrincipalFor(context.Background(), "acme-slack", "U-stranger")
	if err != nil {
		t.Fatalf("PrincipalFor: %v", err)
	}
	if bound || who != "" {
		t.Errorf("got %q (%v), want nobody", who, bound)
	}
}

func TestPrincipalFor_aBoundAccount_answersWho(t *testing.T) {
	channels, _ := boundChannels(t)
	ctx := context.Background()

	if err := channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel: "acme-slack", Account: "U024", Principal: "usr_ana",
	}, "usr_admin"); err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}

	who, bound, err := channels.PrincipalFor(ctx, "acme-slack", "U024")
	if err != nil || !bound || who != "usr_ana" {
		t.Errorf("got %q (%v, %v), want usr_ana", who, bound, err)
	}
}

/*
A row that exists, is enabled and cannot be read is corrupted configuration.

Answered as "nobody", somebody goes and links an account that already has a
row — and the row that is wrong stays wrong, because nothing ever said it was
there. Writing one is refused, so it arrives another way: a restore, a
migration, a hand-edited database. Which is exactly why the read has to defend
itself rather than trust the write.
*/
func TestPrincipalFor_aStoredBindingNobodyCanRead_isAnError(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U404",
		Value:     []byte(`{"channel":"acme-slack","account":"U404"}`),
		Enabled:   true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the broken row: %v", err)
	}

	if _, bound, err := channels.PrincipalFor(ctx, "acme-slack", "U404"); err == nil {
		t.Errorf("bound = %v with no error, want the corrupt row reported", bound)
	}
}

func boundChannels(t *testing.T) (*admin.Channels, *settings.Store) {
	t.Helper()
	pool := freshPool(t)
	store := settings.NewStore(pool, nil)
	return admin.NewChannels(pool, store), store
}
