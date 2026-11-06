package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

func aConnection(t *testing.T, store *settings.Store, name string) {
	t.Helper()
	if err := store.Put(context.Background(), settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: channel.KindChannel, Name: name,
		Value: []byte(`{"kind":"slack"}`), Enabled: true, UpdatedBy: "test",
	}); err != nil {
		t.Fatalf("connection: %v", err)
	}
}

// A binding hands whoever holds an account somebody's authority, including deciding
// approvals. A connection recreated under an old name must not inherit one: nobody
// looking at the new connection decided that, and nothing on it says so.
func TestDeleteChannel_aConnectionRecreatedUnderItsName_inheritsNobody(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()
	aConnection(t, store, "acme-slack")
	if err := channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel: "acme-slack", Account: "U1", Principal: "usr_ana",
	}, "usr_admin"); err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}

	if err := channels.DeleteChannel(ctx, "acme-slack", "usr_admin", true); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}
	aConnection(t, store, "acme-slack")

	principal, bound, err := channels.PrincipalFor(ctx, "acme-slack", "U1")
	if err != nil {
		t.Fatalf("PrincipalFor: %v", err)
	}
	if bound {
		t.Fatalf("the account inherited %q", principal)
	}
}

// Only that connection's, and every position it is stored in: a copy somewhere it means
// nothing today is adopted by whatever moves it tomorrow.
func TestDeleteChannel_takesItsOwnBindingsWhereverStored_andNoOtherConnections(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()
	for _, name := range []string{"acme-slack", "acme-slack-2"} {
		aConnection(t, store, name)
		if err := channels.BindIdentity(ctx, admin.ChannelIdentity{
			Channel: name, Account: "U1", Principal: "usr_ana",
		}, "usr_admin"); err != nil {
			t.Fatalf("BindIdentity: %v", err)
		}
	}
	misplaced := domain.Scope{Company: "acme", Area: "cx"}
	if err := store.Put(ctx, settings.Setting{
		ScopeKind: settings.ScopeArea, Scope: misplaced, Kind: admin.KindChannelIdentity,
		Name: "acme-slack/U2", Value: []byte(`{"channel":"acme-slack","account":"U2","principal":"usr_bia"}`),
		Enabled: true, UpdatedBy: "restore",
	}); err != nil {
		t.Fatalf("misplaced: %v", err)
	}

	if err := channels.DeleteChannel(ctx, "acme-slack", "usr_admin", true); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}

	stored, err := store.List(ctx, admin.KindChannelIdentity)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stored) != 1 || stored[0].Name != "acme-slack-2/U1" {
		t.Fatalf("bindings left = %+v, want only the other connection's", stored)
	}
}

// Withdrawing somebody's authority is the kind of thing an auditor asks about, and the
// connection going is not an answer to "who could approve through it until when".
func TestDeleteChannel_recordsWhichBindingsItWithdrew(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()
	pool := freshPool(t)
	aConnection(t, store, "acme-slack")
	if err := channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel: "acme-slack", Account: "U1", Principal: "usr_ana",
	}, "usr_admin"); err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}

	if err := channels.DeleteChannel(ctx, "acme-slack", "usr_admin", true); err != nil {
		t.Fatalf("DeleteChannel: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `
		select count(*) from admin_events
		where action = 'channel.identity.unbound' and target = 'acme-slack/U1'
		  and principal_id = 'usr_admin' and detail->>'reason' = 'connection removed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("events = %d, %v", count, err)
	}
}

// With no connection there is nothing to speak on, and the binding would wait there for
// the next connection to take the name.
func TestBindIdentity_onAConnectionThatDoesNotExist_isRefused(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()

	err := channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel: "nobody-slack", Account: "U1", Principal: "usr_ana",
	}, "usr_admin")

	if !errors.Is(err, admin.ErrNoConnection) {
		t.Fatalf("err = %v, want refused", err)
	}
	stored, listErr := store.List(ctx, admin.KindChannelIdentity)
	if listErr != nil || len(stored) != 0 {
		t.Fatalf("bindings = %+v, %v", stored, listErr)
	}
}

// The race: a bind that read the connection, then a delete that finishes, then the bind
// writes. With the delete holding the connection's lock, the bind has to wait and then
// see that the connection is gone.
func TestBindIdentity_waitsForADeleteInProgress_andThenSeesItGone(t *testing.T) {
	channels, store := boundChannels(t)
	pool := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	aConnection(t, store, "acme-slack")

	barrier := hold(t, pool, "channel:acme-slack")
	bound := make(chan error, 1)
	go func() {
		bound <- channels.BindIdentity(ctx, admin.ChannelIdentity{
			Channel: "acme-slack", Account: "U1", Principal: "usr_ana",
		}, "usr_admin")
	}()
	barrier.waitFor(t, 1)
	if err := store.Delete(ctx, settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, "acme-slack"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	barrier.release(t)

	if err := <-bound; !errors.Is(err, admin.ErrNoConnection) {
		t.Fatalf("bind after the delete: err = %v, want refused", err)
	}
	stored, err := store.List(ctx, admin.KindChannelIdentity)
	if err != nil || len(stored) != 0 {
		t.Fatalf("bindings = %+v, %v", stored, err)
	}
}

// Whichever goes first, the invariant holds: no binding outlives its connection.
func TestBindAndDelete_racing_neverLeaveABindingWithoutAConnection(t *testing.T) {
	channels, store := boundChannels(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for range 25 {
		aConnection(t, store, "acme-slack")
		done := make(chan error, 2)
		go func() {
			err := channels.BindIdentity(ctx, admin.ChannelIdentity{
				Channel: "acme-slack", Account: "U1", Principal: "usr_ana",
			}, "usr_admin")
			if errors.Is(err, admin.ErrNoConnection) {
				err = nil
			}
			done <- err
		}()
		go func() { done <- channels.DeleteChannel(ctx, "acme-slack", "usr_admin", true) }()
		for range 2 {
			if err := <-done; err != nil {
				t.Fatalf("racing: %v", err)
			}
		}
		stored, err := store.List(ctx, admin.KindChannelIdentity)
		if err != nil || len(stored) != 0 {
			t.Fatalf("a binding outlived its connection: %+v, %v", stored, err)
		}
	}
}
