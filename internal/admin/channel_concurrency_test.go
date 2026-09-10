package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
Two writes about one connection, arriving together.

Both rules here were read before the transaction and enforced after it, which
is not enforcement at all: whatever the read saw is a state somebody else is
free to change while the write waits for its turn. Held apart by an advisory
lock taken from outside, the two requests are made to arrive in exactly the
order that used to be a race.
*/

// TestPutConversation_twoScopesForOneConversationAtOnce_onlyOneIsStored proves
// the uniqueness rule survives it. A conversation that speaks for two scopes is
// the disclosure the scope exists to prevent: the same Slack channel receiving
// two companies' runs, and nobody able to say which one governs a message
// written in it.
func TestPutConversation_twoScopesForOneConversationAtOnce_onlyOneIsStored(t *testing.T) {
	pool := freshPool(t)
	channels := admin.NewChannels(pool, settings.NewStore(pool, testVault(t)), onlySlack{})

	barrier := hold(t, pool, "channel:acme-slack")

	results := make(chan error, 2)
	for _, area := range []domain.AreaID{"ops", "finance"} {
		go func() {
			results <- channels.PutConversation(context.Background(), "acme-slack",
				admin.Conversation{
					ID: "C-shared", Enabled: true, Wants: []string{"parked"},
					Scope: domain.Scope{Company: "acme", Area: area},
				}, "usr_ana")
		}()
	}
	// Both are inside their transactions and queued on the lock. Whatever they
	// read about each other, they read before this line.
	barrier.waitFor(t, 2)
	barrier.release(t)

	stored, refused := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			stored++
		case errors.Is(err, admin.ErrConversationMapped):
			refused++
		default:
			t.Fatalf("PutConversation: %v", err)
		}
	}
	if stored != 1 || refused != 1 {
		t.Fatalf("stored %d, refused %d, want one of each", stored, refused)
	}
}

/*
And two partial rotations keep both halves.

A request may carry one credential and mean "leave the other". Read before the
transaction, that fold is a lost update waiting for two people: both read the
old pair, both write their own half onto it, and the second commit puts the
other's back the way it was — reported as success, with the console showing
both credentials present.
*/
func TestPutChannel_twoPartialRotationsAtOnce_keepBoth(t *testing.T) {
	pool := freshPool(t)
	store := settings.NewStore(pool, testVault(t))
	channels := admin.NewChannels(pool, store, onlySlack{})
	ctx := context.Background()

	if err := channels.PutChannel(ctx, admin.ChannelWrite{
		Channel:     admin.Channel{Name: "acme-slack", Kind: "slack", Enabled: true},
		Credentials: channel.Credentials{Token: "token-old", Signing: "signing-old"},
		By:          "usr_ana", Governs: true,
	}); err != nil {
		t.Fatalf("configure the connection: %v", err)
	}

	barrier := hold(t, pool, "channel:acme-slack")

	results := make(chan error, 2)
	for _, creds := range []channel.Credentials{
		{Token: "token-new"}, {Signing: "signing-new"},
	} {
		go func() {
			results <- channels.PutChannel(ctx, admin.ChannelWrite{
				Channel:     admin.Channel{Name: "acme-slack", Kind: "slack", Enabled: true},
				Credentials: creds, By: "usr_ana", Governs: true,
			})
		}()
	}
	barrier.waitFor(t, 2)
	barrier.release(t)

	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("PutChannel: %v", err)
		}
	}

	held, err := store.Reveal(ctx, settings.ScopeInstallation, domain.Scope{},
		channel.KindChannel, "acme-slack")
	if err != nil {
		t.Fatalf("read the credentials back: %v", err)
	}
	got := channel.ReadCredentials(held.Secret)
	if got.Token != "token-new" || got.Signing != "signing-new" {
		t.Fatalf("token=%q signing=%q, want both rotations kept", got.Token, got.Signing)
	}
}

/*
barrier holds the connection's lock from outside, so a test can decide when two
writers are let go — and can tell that they are both actually waiting.

Asked of Postgres rather than timed. A sleep long enough to be reliable is long
enough to be slow, and one short enough to be quick is a test that passes for
the wrong reason on a loaded machine: a writer that had not started yet reads
the other's result, sees the state it was supposed to race, and agrees with the
broken implementation.
*/
type barrier struct {
	pool *pgxpool.Pool
	conn *pgxpool.Conn
	lock advisoryLock
	key  string
	done bool
}

type advisoryLock struct {
	database uint32
	classID  uint32
	objID    uint32
	objSubID int16
}

func hold(t *testing.T, pool *pgxpool.Pool, key string) *barrier {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	b := &barrier{pool: pool, conn: conn, key: key}
	t.Cleanup(func() {
		if !b.done {
			conn.Release()
		}
	})
	if _, err := conn.Exec(context.Background(),
		`select pg_advisory_lock(hashtext($1))`, key); err != nil {
		t.Fatalf("take the lock: %v", err)
	}

	var pid int
	if err := conn.QueryRow(context.Background(), `select pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("read the holder's backend: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `
		select database, classid, objid, objsubid from pg_locks
		where locktype = 'advisory' and granted and pid = $1`, pid).
		Scan(&b.lock.database, &b.lock.classID, &b.lock.objID, &b.lock.objSubID); err != nil {
		t.Fatalf("read the lock the holder took: %v", err)
	}
	return b
}

// waitFor blocks until that many writers are queued on this exact lock. Until
// they are, letting go would prove nothing.
func (b *barrier) waitFor(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := b.pool.QueryRow(context.Background(), `
			select count(*) from pg_locks
			where locktype = 'advisory' and not granted
			  and database = $1 and classid = $2 and objid = $3 and objsubid = $4`,
			b.lock.database, b.lock.classID, b.lock.objID, b.lock.objSubID).
			Scan(&waiting); err != nil {
			t.Fatalf("read lock waiters: %v", err)
		}
		if waiting >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d writers reached the connection's lock", waiting, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (b *barrier) release(t *testing.T) {
	t.Helper()
	b.done = true
	if _, err := b.conn.Exec(context.Background(),
		`select pg_advisory_unlock(hashtext($1))`, b.key); err != nil {
		t.Errorf("release the lock: %v", err)
	}
	b.conn.Release()
}

func testVault(t *testing.T) *vault.Vault {
	t.Helper()
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return v
}
