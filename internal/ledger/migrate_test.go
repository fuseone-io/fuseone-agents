package ledger_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
)

// Every process migrates on the way up. A deployment starts them together, so
// two of them racing on a schema neither has yet is the ordinary case rather
// than the unlucky one.

func TestMigrate_twoProcessesStartingTogether_bothSucceed(t *testing.T) {
	dsn := testdb.DSN(t)
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

	var missingDigests int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from schema_migrations
		where digest is null or digest_source <> 'applied'
	`).Scan(&missingDigests); err != nil {
		t.Fatalf("read migration digests: %v", err)
	}
	if missingDigests != 0 {
		t.Fatalf("fresh migrations without applied digests = %d", missingDigests)
	}
}

func TestMigrate_legacyRowsReceiveCanonicalBaseline(t *testing.T) {
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(), `
		update schema_migrations
		set digest = null, digest_source = null
		where version <= '0081_inbox_run_index'
	`); err != nil {
		t.Fatalf("remove historical digests: %v", err)
	}
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("baseline: %v", err)
	}

	var missingBaseline int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from schema_migrations
		where version <= '0081_inbox_run_index'
		  and (digest is null or digest_source <> 'baseline')
	`).Scan(&missingBaseline); err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	if missingBaseline != 0 {
		t.Fatalf("legacy rows without baseline digests = %d", missingBaseline)
	}
}

/*
The ticket waiting notice upgrades a database that has already recorded 0076.

This is deliberately an upgrade test, not another bootstrap test. Rebuilding a
database from the embedded files cannot expose an edited migration: the fresh
schema and the new code agree while every database that recorded the old file
keeps the old shape. The setup below puts the database back at the exact
post-0076 boundary and leaves both historical versions recorded, so only 0077
can supply the new columns and the versioned constraint name.
*/
func TestMigrate_ticketWaitingNotice_upgradesAnAlreadyMigratedInbox(t *testing.T) {
	pool := testdb.Pool(t)

	if _, err := pool.Exec(t.Context(), `
		delete from schema_migrations
		where version = '0077_ticket_waiting_notice';
		alter table channel_inbox
			drop column pending_notice_answered_at,
			drop column pending_notice,
			drop column claim_attempts;
		alter table governed_external_attempts
			rename constraint governed_external_attempts_result_shape_v2
			to governed_external_attempts_result_shape;
	`); err != nil {
		t.Fatalf("restore the post-0076 schema: %v", err)
	}
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("upgrade from 0076: %v", err)
	}

	var historical int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from schema_migrations
		where version in ('0075_external_attempts', '0076_governed_ticket_ingress')
	`).Scan(&historical); err != nil {
		t.Fatalf("read historical migrations: %v", err)
	}
	if historical != 2 {
		t.Fatalf("recorded historical migrations = %d, want both untouched", historical)
	}

	var columns int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from information_schema.columns
		where table_schema = 'public' and table_name = 'channel_inbox'
		  and column_name in ('claim_attempts', 'pending_notice', 'pending_notice_answered_at')
	`).Scan(&columns); err != nil {
		t.Fatalf("read upgraded inbox: %v", err)
	}
	if columns != 3 {
		t.Fatalf("ticket waiting columns = %d, want 3", columns)
	}

	var constraints int
	if err := pool.QueryRow(t.Context(), `
		select count(*) from pg_constraint c
		join pg_class t on t.oid = c.conrelid
		where t.relname = 'governed_external_attempts'
		  and c.conname = 'governed_external_attempts_result_shape_v2'
	`).Scan(&constraints); err != nil {
		t.Fatalf("read upgraded result constraint: %v", err)
	}
	if constraints != 1 {
		t.Fatalf("versioned result constraints = %d, want 1", constraints)
	}
}

// A preview database may have applied the briefly edited 0076, which already
// carried the notice columns but could never record 0077. The new migration
// converges that shape too; otherwise fixing frozen history repairs an older
// preview and breaks a newer one with duplicate-column errors.
func TestMigrate_ticketWaitingNotice_convergesTheInterimPreviewSchema(t *testing.T) {
	pool := testdb.Pool(t)

	if _, err := pool.Exec(t.Context(), `
		delete from schema_migrations
		where version = '0077_ticket_waiting_notice';
		alter table governed_external_attempts
			rename constraint governed_external_attempts_result_shape_v2
			to governed_external_attempts_result_shape;
	`); err != nil {
		t.Fatalf("restore the interim preview schema: %v", err)
	}
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("converge the interim preview schema: %v", err)
	}

	var applied bool
	if err := pool.QueryRow(t.Context(), `
		select exists (
			select 1 from schema_migrations
			where version = '0077_ticket_waiting_notice'
		)
	`).Scan(&applied); err != nil {
		t.Fatalf("read migration state: %v", err)
	}
	if !applied {
		t.Fatal("0077 was not recorded after converging the preview schema")
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
	pool := testdb.Pool(t)

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
	pool := testdb.Pool(t)

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

	// Which advisory lock that was, as Postgres itself identifies it. Waiting is
	// then something the database is asked about rather than something inferred
	// from a stopwatch: a slow migration and a migration queued behind this
	// holder look identical from outside, and only one of them is the rule.
	var pid int
	if err := holder.QueryRow(t.Context(), `select pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("read the holder's backend: %v", err)
	}
	var db, class, obj, sub uint32
	if err := pool.QueryRow(t.Context(), `
		select database, classid, objid, objsubid from pg_locks
		where locktype = 'advisory' and granted and pid = $1`, pid).
		Scan(&db, &class, &obj, &sub); err != nil {
		t.Fatalf("read the lock the holder took: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- ledger.Migrate(context.Background(), pool) }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("the rename did not wait for the connection's lock: %v", err)
		default:
		}
		var waiting int
		if err := pool.QueryRow(t.Context(), `
			select count(*) from pg_locks
			where locktype = 'advisory' and not granted
			  and database = $1 and classid = $2 and objid = $3 and objsubid = $4`,
			db, class, obj, sub).Scan(&waiting); err != nil {
			t.Fatalf("read lock waiters: %v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the migration never queued on the connection's lock")
		}
		time.Sleep(5 * time.Millisecond)
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

// Corrections erased before erasure took expectations still hold them. The next
// step turns expectations into content, and these must not arrive there readable.
func TestMigrate_correctionsAlreadyErased_loseTheirExpectations(t *testing.T) {
	pool := testdb.IsolatedThrough(t, "0101_settings_read_by_owner")
	ctx := t.Context()

	if _, err := pool.Exec(ctx, `
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes, erased_at, erased_reason)
		values ('regression://migrate-erased/1/gone', 'regression', 'migrate-erased', 1, 'gone', null, 0, now(), 'subject'),
		       ('regression://migrate-erased/1/kept', 'regression', 'migrate-erased', 1, 'kept', '{}', 2, null, '');
		insert into regression_cases (agent_id, case_id, input_ref, expectations) values
		  ('migrate-erased', 'erased', 'regression://migrate-erased/1/gone', '[{"kind":"says","value":"ana@example.com"}]'),
		  ('migrate-erased', 'readable', 'regression://migrate-erased/1/kept', '[{"kind":"says","value":"keep"}]');
	`); err != nil {
		t.Fatalf("restore the state before the migration: %v", err)
	}
	if err := ledger.MigrateThrough(ctx, pool, "0102_erased_corrections_expectations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	read := func(id string) string {
		var text string
		if err := pool.QueryRow(ctx, `select expectations::text from regression_cases
			where agent_id = 'migrate-erased' and case_id = $1`, id).Scan(&text); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		return text
	}
	if got := read("erased"); got != "[]" {
		t.Errorf("erased case = %s, want its expectations gone", got)
	}
	if got := read("readable"); !strings.Contains(got, "keep") {
		t.Errorf("readable case = %s, want it untouched", got)
	}
}

/*
The binary before 0102 runs alongside it during a rolling update, and that binary
clears only a note. Each statement here is its SQL, narrowed only so that retention
stays inside this test's rows: it reports the erasure as done, so what it does not
clear has to be cleared by the database.
*/
func TestMigrate_theErasureSQLOfThePreviousVersion_leavesNoExpectations(t *testing.T) {
	cases := []struct {
		name  string
		erase func(ctx context.Context, tx pgx.Tx, ref string) error
	}{{
		name: "a subject erasure",
		erase: func(ctx context.Context, tx pgx.Tx, ref string) error {
			if _, err := tx.Exec(ctx, `
				update run_content
				set bytes = null, erased_at = now(), erased_reason = $2
				where ref = any($1::text[]) and erased_at is null`, []string{ref}, "subject"); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				update regression_cases set note = ''
				where input_ref = any($1::text[]) or from_run = any($2::text[])`,
				[]string{ref}, []string{"run-old-binary"})
			return err
		},
	}, {
		name: "retention",
		erase: func(ctx context.Context, tx pgx.Tx, _ string) error {
			if _, err := tx.Exec(ctx, `
				update run_content
				set bytes = null, erased_at = now(), erased_reason = $2
				where created_at < $1 and erased_at is null and run_id = 'old-binary'`,
				time.Now().Add(time.Hour).UTC(), "retention"); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				update regression_cases c set note = ''
				from run_content rc
				where rc.ref = c.input_ref and rc.erased_at is not null and c.note <> ''`)
			return err
		},
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := testdb.IsolatedThrough(t, "0102_erased_corrections_expectations")
			ctx := t.Context()
			const ref = "regression://old-binary/1/copy"
			if _, err := pool.Exec(ctx, `
				insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes)
				values ('regression://old-binary/1/copy', 'regression', 'old-binary', 1, 'copy', '{}', 2),
				       ('regression://old-binary/1/other', 'regression', 'old-binary-other', 1, 'other', '{}', 2);
				insert into regression_cases (agent_id, case_id, input_ref, from_run, expectations) values
				  ('old-binary', 'named', 'regression://old-binary/1/copy', 'run-old-binary',
				   '[{"kind":"says","value":"ana@example.com"}]'),
				  ('old-binary', 'other', 'regression://old-binary/1/other', 'run-other',
				   '[{"kind":"says","value":"keep"}]');
			`); err != nil {
				t.Fatalf("seed: %v", err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			if err := tc.erase(ctx, tx, ref); err != nil {
				t.Fatalf("erase: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatalf("commit: %v", err)
			}

			if got := expectationsIn(t, pool, "old-binary", "named"); got != "[]" {
				t.Errorf("after the old binary's erasure = %s, want nothing left", got)
			}
			if got := expectationsIn(t, pool, "old-binary", "other"); !strings.Contains(got, "keep") {
				t.Errorf("a correction nothing erased = %s, want it untouched", got)
			}
		})
	}
}

func expectationsIn(t *testing.T, pool *pgxpool.Pool, agent, id string) string {
	t.Helper()
	var text string
	if err := pool.QueryRow(context.Background(), `select expectations::text from regression_cases
		where agent_id = $1 and case_id = $2`, agent, id).Scan(&text); err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return text
}

// An erasure holding its tombstone uncommitted while the migration runs. A backfill
// reading before that commit, with the trigger arriving after it, would miss the row
// for good: the old binary has already finished and nothing comes back for it.
func TestMigrate_anErasureInFlight_isSeenByTheBackfill(t *testing.T) {
	pool := testdb.IsolatedThrough(t, "0101_settings_read_by_owner")
	ctx := t.Context()
	const ref = "regression://in-flight/1/copy"
	if _, err := pool.Exec(ctx, `
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes)
		values ('regression://in-flight/1/copy', 'regression', 'in-flight', 1, 'copy', '{}', 2);
		insert into regression_cases (agent_id, case_id, input_ref, expectations)
		values ('in-flight', 'named', 'regression://in-flight/1/copy', '[{"kind":"says","value":"ana@example.com"}]');
	`); err != nil {
		t.Fatalf("restore the state before the migration: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `update run_content set bytes = null, erased_at = now(), erased_reason = 'subject'
		where ref = $1`, ref); err != nil {
		t.Fatalf("tombstone: %v", err)
	}

	migrated := make(chan error, 1)
	go func() {
		migrated <- ledger.MigrateThrough(context.Background(), pool, "0102_erased_corrections_expectations")
	}()

	// The migration has reached the table and is queued behind this transaction.
	deadline := time.Now().Add(30 * time.Second)
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `select exists (
			select 1 from pg_locks where relation = 'run_content'::regclass and not granted)`).Scan(&waiting); err != nil {
			t.Fatalf("read locks: %v", err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the migration never queued behind the erasure in flight")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit the erasure: %v", err)
	}
	if err := <-migrated; err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if got := expectationsIn(t, pool, "in-flight", "named"); got != "[]" {
		t.Fatalf("expectations = %s, want them gone", got)
	}
}

// Recording a correction again restores the copy before writing what it expects,
// and clearing is for a copy going away, never for one coming back.
func TestMigrate_aCopyRestored_doesNotLoseTheExpectationsWrittenAfter(t *testing.T) {
	pool := testdb.IsolatedThrough(t, "0102_erased_corrections_expectations")
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes, full_bytes, erased_at, erased_reason)
		values ('regression://restored/1/copy', 'regression', 'restored', 1, 'copy', null, 2, now(), 'subject');
		insert into regression_cases (agent_id, case_id, input_ref)
		values ('restored', 'named', 'regression://restored/1/copy');
		update run_content set bytes = '{}', erased_at = null, erased_reason = '' where run_id = 'restored';
		update regression_cases set expectations = '[{"kind":"says","value":"recorded again"}]'
		where agent_id = 'restored';
		update run_content set full_bytes = 2 where run_id = 'restored';
	`); err != nil {
		t.Fatalf("record again: %v", err)
	}
	if got := expectationsIn(t, pool, "restored", "named"); !strings.Contains(got, "recorded again") {
		t.Fatalf("expectations = %s, want what was recorded", got)
	}
}
