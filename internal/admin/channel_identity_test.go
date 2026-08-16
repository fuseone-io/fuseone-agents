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

/*
A binding the runtime denounces is a binding the screen shows.

The listing used to skip a row it could not read, so an operator met an error
naming a binding the console said did not exist — the platform disagreeing with
itself about the very row they had been sent to fix. What names it is recovered
from the key, which is outside the value that failed to parse, so it can still
be removed.
*/
func TestIdentities_aStoredBindingNobodyCanRead_isListedAsBroken(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U404",
		// Valid JSON with no principal. The column is JSONB, so a value that
		// is not JSON never reaches it — the corruption that can occur is a
		// row that parses and says nothing.
		Value:   []byte(`{"channel":"acme-slack","account":"U404"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the broken row: %v", err)
	}

	listed, err := channels.Identities(ctx)
	if err != nil {
		t.Fatalf("Identities: %v", err)
	}

	var found *admin.ChannelIdentity
	for i, one := range listed {
		if one.Account == "U404" {
			found = &listed[i]
		}
	}
	if found == nil {
		t.Fatalf("listed = %+v, want the broken row shown", listed)
	}
	if !found.Unreadable {
		t.Error("the broken row is listed as though it were fine")
	}
	// Recovered from the key, so the row can be pointed at and removed.
	if found.Channel != "acme-slack" {
		t.Errorf("channel = %q, want it recovered from the key", found.Channel)
	}
}

/*
A row that kept half of itself is still removable.

Recovered as a pair, a value that held its channel and lost its account had the
recovered account thrown away with it — and the account is the half the console
removes a row by, so the row became unremovable by exactly the screen sent to
remove it.
*/
func TestIdentities_aBrokenRowMissingItsAccount_recoversItFromTheKey(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U505",
		// It kept the channel and lost everything else.
		Value:   []byte(`{"channel":"acme-slack"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the half-broken row: %v", err)
	}

	listed, err := channels.Identities(ctx)
	if err != nil {
		t.Fatalf("Identities: %v", err)
	}

	for _, one := range listed {
		if one.Channel == "acme-slack" && one.Account == "U505" {
			if !one.Unreadable {
				t.Error("the row is listed as though it were fine")
			}
			return
		}
	}
	t.Errorf("listed = %+v, want the row nameable enough to remove", listed)
}

/*
A corrupted value does not get to say where its row lives.

Preferring the value where it said something was still trusting the part that
is broken: a row keyed under one channel whose contents claim another would be
listed on the wrong card, and then deleted by a request naming a channel it was
never under — so the row survives every attempt to remove it, from the one
screen that offers to.
*/
func TestIdentities_aBrokenRowClaimingAnotherChannel_isNamedByItsKey(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      admin.KindChannelIdentity,
		Name:      "acme-slack/U505",
		Value:     []byte(`{"channel":"old-slack","display":"Ana"}`),
		Enabled:   true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("write the lying row: %v", err)
	}

	listed, err := channels.Identities(ctx)
	if err != nil {
		t.Fatalf("Identities: %v", err)
	}

	for _, one := range listed {
		if one.Account != "U505" {
			continue
		}
		if one.Channel != "acme-slack" {
			t.Errorf("channel = %q, want the one the key says", one.Channel)
		}
		// The hint survives, because it names nothing and helps somebody
		// decide whether to remove the row.
		if one.Display != "Ana" {
			t.Errorf("display = %q, want the hint kept", one.Display)
		}
		return
	}
	t.Errorf("listed = %+v, want the row under the channel it is stored beneath", listed)
}
