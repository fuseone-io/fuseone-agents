package admin_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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

// Bindings a delete already left behind are waiting for somebody to reuse the name, and
// a fix to the delete does not reach them.
func TestOrphanedBindingsMigration_takesOnlyBindingsWithNoConnection(t *testing.T) {
	channels, store := boundChannels(t)
	pool := freshPool(t)
	ctx := context.Background()
	aConnection(t, store, "kept-slack")
	if err := channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel: "kept-slack", Account: "U1", Principal: "usr_ana",
	}, "usr_admin"); err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}
	seedOrphan(t, pool, "gone-slack/U2", "usr_bia")

	migration, err := os.ReadFile("../ledger/sql/0100_orphaned_channel_identities.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply: %v", err)
	}

	stored, err := store.List(ctx, admin.KindChannelIdentity)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stored) != 1 || stored[0].Name != "kept-slack/U1" {
		t.Fatalf("bindings left = %+v", stored)
	}
	// Run again it changes nothing, so an installation that re-applies it is safe.
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply again: %v", err)
	}
	var events int
	if err := pool.QueryRow(ctx, `
		select count(*) from admin_events
		where action = 'channel.identity.unbound' and target = 'gone-slack/U2'
		  and detail->>'principal' = 'usr_bia'`).Scan(&events); err != nil || events != 1 {
		t.Fatalf("events = %d, %v", events, err)
	}
}

// seedOrphan writes what an earlier version could leave behind and the schema now
// refuses, so the rule has to be off for the write.
func seedOrphan(t *testing.T, pool *pgxpool.Pool, key, principal string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, statement := range []string{
		`alter table settings disable trigger settings_binding_needs_connection`,
		`insert into settings (scope_kind, kind, name, value) values ('installation', 'channel_identity', '` +
			key + `', '{"principal":"` + principal + `"}')`,
		`alter table settings enable trigger settings_binding_needs_connection`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// What the previous version sends, word for word. During a rolling upgrade a replica
// still running it writes against this schema, and it knows nothing of the new locks.
const (
	oldBind = `insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('installation', '', '', 'channel_identity', $1, '{"principal":"usr_ana"}', true, 'old')
		on conflict (scope_kind, company_id, area_id, kind, name) do update set value = excluded.value`
	oldDelete = `delete from settings
		where scope_kind = 'installation' and company_id = '' and area_id = '' and kind = 'channel' and name = $1`
)

func TestOldReplica_bindingWithNoConnection_isRefusedByTheSchema(t *testing.T) {
	_, store := boundChannels(t)
	pool := freshPool(t)
	ctx := context.Background()
	aConnection(t, store, "acme-slack")

	if _, err := pool.Exec(ctx, oldBind, "nobody-slack/U1"); err == nil {
		t.Fatal("a binding with no connection was stored")
	}
	if _, err := pool.Exec(ctx, oldBind, "acme-slack/U1"); err != nil {
		t.Fatalf("a binding on a real connection: %v", err)
	}
}

func TestOldReplica_deletingAConnectionWithBindings_isRefusedByTheSchema(t *testing.T) {
	_, store := boundChannels(t)
	pool := freshPool(t)
	ctx := context.Background()
	aConnection(t, store, "kept-slack")
	if _, err := pool.Exec(ctx, oldBind, "kept-slack/U1"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	if _, err := pool.Exec(ctx, oldDelete, "kept-slack"); err == nil {
		t.Fatal("the connection was deleted with a binding on it")
	}
	if _, err := store.Get(ctx, settings.ScopeInstallation, domain.Scope{}, channel.KindChannel, "kept-slack"); err != nil {
		t.Fatalf("the connection is gone: %v", err)
	}

	aConnection(t, store, "empty-slack")
	if _, err := pool.Exec(ctx, oldDelete, "empty-slack"); err != nil {
		t.Fatalf("a connection with nothing bound: %v", err)
	}
}

// Editing a connection is not removing it. A rule that treated it so would stop anybody
// rotating a credential on a connection people are bound to.
func TestEditingAConnectionWithBindings_isAllowed(t *testing.T) {
	channels, store := boundChannels(t)
	ctx := context.Background()
	if err := channels.BindIdentity(ctx, admin.ChannelIdentity{
		Channel: "acme-slack", Account: "U1", Principal: "usr_ana",
	}, "usr_admin"); err != nil {
		t.Fatalf("BindIdentity: %v", err)
	}

	aConnection(t, store, "acme-slack")

	if _, bound, err := channels.PrincipalFor(ctx, "acme-slack", "U1"); err != nil || !bound {
		t.Fatalf("the binding after editing: bound=%v, %v", bound, err)
	}
}

// Neither order of the race leaves a binding without its connection, though neither
// writer takes the new lock: the row itself is what they serialise on.
func TestOldReplica_bindRacingDelete_oneOfThemIsRefused(t *testing.T) {
	_, store := boundChannels(t)
	pool := freshPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	blocked := func(t *testing.T, first, second string, name string) error {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("Begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		firstArg, secondArg := name+"/U1", name
		if first == oldDelete {
			firstArg, secondArg = name, name+"/U1"
		}
		if _, err := tx.Exec(ctx, first, firstArg); err != nil {
			t.Fatalf("first: %v", err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := pool.Exec(ctx, second, secondArg)
			done <- err
		}()
		select {
		case err := <-done:
			t.Fatalf("the second writer did not wait: %v", err)
		case <-time.After(300 * time.Millisecond):
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		return <-done
	}

	aConnection(t, store, "bind-first")
	if err := blocked(t, oldBind, oldDelete, "bind-first"); err == nil {
		t.Error("bind first: the delete went through with a binding committed")
	}
	aConnection(t, store, "delete-first")
	if err := blocked(t, oldDelete, oldBind, "delete-first"); err == nil {
		t.Error("delete first: the bind went through with the connection gone")
	}
	var orphaned int
	if err := pool.QueryRow(ctx, `
		select count(*) from settings s where s.kind = 'channel_identity' and not exists (
			select 1 from settings c where c.kind = 'channel' and c.scope_kind = 'installation'
			  and c.name = channel_identity_connection(s.name))`).Scan(&orphaned); err != nil || orphaned != 0 {
		t.Fatalf("orphaned = %d, %v", orphaned, err)
	}
}
