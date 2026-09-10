package ledger_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
)

// Every process migrates on the way up. A deployment starts them together, so
// two of them racing on a schema neither has yet is the ordinary case rather
// than the unlucky one.

func TestMigrate_twoProcessesStartingTogether_bothSucceed(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; skipping the migration race")
	}

	// A database with nothing in it, so both callers have the whole set to
	// apply and every migration is a chance to collide.
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(t.Context(),
		`drop schema public cascade; create schema public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Separate pools: two processes, not two goroutines sharing one.
			own, err := pgxpool.New(context.Background(), dsn)
			if err != nil {
				errs[i] = err
				return
			}
			defer own.Close()
			errs[i] = ledger.Migrate(context.Background(), own)
		}()
	}
	wg.Wait()

	// Without a lock both read the same empty set of applied versions, both
	// try to create the same table, and the loser dies on a duplicate object —
	// which in a deployment is one pod crash-looping until the other finishes.
	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i+1, err)
		}
	}
}

/*
Conversations stored under the id alone take their connection into the key.

The second half of a two-release move: the release before this one reads both
shapes, so renaming what is stored is safe now and was not then. An installation
upgrading into this schema has rows the new key does not name, and the delete
keys by connection and id — so without the rename a removal would match nothing
and report success.

Four things the statement has to get right, each of which was got wrong on the
way here, and each of which is a case below.
*/
func TestMigrate_conversationsUnderTheIdAlone_takeTheirConnection(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; skipping the migration")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const version = "0072_conversations_by_connection"
	rows := []struct {
		name  string
		value string
		want  string
	}{
		// The ordinary one: renamed, and told what shape it is now in.
		{"C-LEGACY", `{"channel":"acme-slack","mode":"mentions"}`,
			"10:acme-slack/C-LEGACY"},
		// Bytes, not runes: the key counts octets and connection names accept
		// Unicode, so a rename measured in characters produces a name the
		// reader cannot take apart.
		{"C-UNICODE", `{"channel":"café-slack","mode":"mentions"}`,
			"11:café-slack/C-UNICODE"},
		// A connection whose name holds what `like` reads as a wildcard. No
		// accuser: this statement compares nothing against a built prefix, so
		// the hazard is designed out rather than guarded — and the case is
		// here to say that if somebody adds such a comparison, this row is the
		// one that will show it.
		{"C-WILD", `{"channel":"acme_slack","mode":"mentions"}`,
			"10:acme_slack/C-WILD"},
		// A row whose new name is already taken, by a conversation somebody
		// saved after the release that writes the new shape. Left where it is:
		// a rename that collides aborts the statement, and a migration that
		// dies is a process that will not start.
		{"C-TAKEN", `{"channel":"acme-slack","mode":"mentions"}`, "C-TAKEN"},
		// Already moved. Left exactly as it is, so the statement is safe to run
		// against a database somebody has since written to.
		{"11:other-slack/C-DONE",
			`{"channel":"other-slack","keyVersion":2,"mode":"mentions"}`,
			"11:other-slack/C-DONE"},
		// Nothing to hang it on: left alone rather than given an invented
		// connection.
		{"C-ORPHANED", `{"mode":"mentions"}`, "C-ORPHANED"},
		// A shape nobody can read. Left where it is — the reader calls it
		// nobody's conversation — and, crucially, not allowed to take the
		// upgrade down with it: cast to an integer this row aborts the
		// statement, the hook, and the release.
		{"C-BROKEN", `{"channel":"acme-slack","keyVersion":"broken"}`, "C-BROKEN"},
		{"C-BOOLEAN", `{"channel":"acme-slack","keyVersion":true}`, "C-BOOLEAN"},
	}
	// The row that makes C-TAKEN's new name unavailable, written first.
	rows = append(rows, struct {
		name  string
		value string
		want  string
	}{
		"10:acme-slack/C-TAKEN",
		`{"channel":"acme-slack","keyVersion":2,"mode":"mentions"}`,
		"10:acme-slack/C-TAKEN",
	})
	for _, one := range rows {
		if _, err := pool.Exec(t.Context(), `
			insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
			values ('area', 'acme', 'migrating', 'channel_conversation', $1, $2, true, 'restore')
			on conflict (scope_kind, company_id, area_id, kind, name) do update set value = excluded.value`,
			one.name, one.value); err != nil {
			t.Fatalf("write the %s row: %v", one.name, err)
		}
	}
	if _, err := pool.Exec(t.Context(),
		`delete from schema_migrations where version = $1`, version); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`delete from settings where company_id = 'acme' and area_id = 'migrating'`)
	})

	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate again: %v", err)
	}

	for _, one := range rows {
		// Read as text, never cast: the statement under test must survive a
		// row whose keyVersion is not a number, and a readback that casts
		// fails on the very row that proves it.
		var version *string
		err := pool.QueryRow(t.Context(), `
			select value->>'keyVersion' from settings
			where kind = 'channel_conversation' and company_id = 'acme'
			  and area_id = 'migrating' and name = $1`, one.want).Scan(&version)
		if err != nil {
			t.Fatalf("%s did not become %s: %v", one.name, one.want, err)
		}
		// Renamed rows declare the shape they are in. A rename without it is
		// read as an id that happens to contain a colon and a slash, which is
		// nobody's conversation.
		if moved := one.want != one.name; moved && (version == nil || *version != "2") {
			t.Errorf("%s was renamed to %s and declares version %v",
				one.name, one.want, version)
		}
	}
}

/*
The rename waits for whoever is writing that connection.

It runs as a pre-upgrade hook, so the release before it is still serving — and
that release reads both shapes and writes the old one. Unlocked, the order that
happens is: a pod decides to write, this statement renames the row, the pod
writes the old name back, and the conversation exists twice. The runtime then
refuses it as ambiguous: a conversation broken by an upgrade that reported
success, and broken for good.

Proved by holding the lock a writer would hold and watching the migration wait
for it. The lock is asked of Postgres by the name the administration uses, so
this also says the two agree about what that name is.
*/
func TestMigrate_conversationsUnderTheIdAlone_waitForTheConnectionsLock(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is unset; skipping the migration")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := pool.Exec(t.Context(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
		values ('area', 'acme', 'locked', 'channel_conversation', 'C-LOCKED',
		        '{"channel":"held-slack","mode":"mentions"}'::jsonb, true, 'restore')
		on conflict (scope_kind, company_id, area_id, kind, name) do update set value = excluded.value`,
	); err != nil {
		t.Fatalf("write the row: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`delete from settings where company_id = 'acme' and area_id = 'locked'`)
	})
	if _, err := pool.Exec(t.Context(),
		`delete from schema_migrations where version = '0072_conversations_by_connection'`,
	); err != nil {
		t.Fatalf("forget the migration: %v", err)
	}

	// The lock a writer holds while it configures that connection.
	holder, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer holder.Release()
	if _, err := holder.Exec(t.Context(),
		`select pg_advisory_lock(hashtext($1))`, "channel:held-slack"); err != nil {
		t.Fatalf("take the lock: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- ledger.Migrate(context.Background(), pool) }()

	select {
	case err := <-done:
		t.Fatalf("the rename did not wait for the connection's lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	if _, err := holder.Exec(t.Context(),
		`select pg_advisory_unlock(hashtext($1))`, "channel:held-slack"); err != nil {
		t.Fatalf("release the lock: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("migrate after the lock was released: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the migration never finished after the lock was released")
	}

	var name string
	if err := pool.QueryRow(t.Context(), `
		select name from settings
		where kind = 'channel_conversation' and company_id = 'acme' and area_id = 'locked'`,
	).Scan(&name); err != nil {
		t.Fatalf("read the row back: %v", err)
	}
	if name != "10:held-slack/C-LOCKED" {
		t.Errorf("name = %q, want it renamed once the lock was free", name)
	}
}
