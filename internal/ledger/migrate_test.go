package ledger_test

import (
	"context"
	"os"
	"sync"
	"testing"

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
Conversations stored before the connection joined the key are renamed.

Until 0071 a conversation was stored under the vendor's id alone, so an
installation upgrading into this schema has rows the new key does not name — and
the delete, which now keys by connection and id, would match nothing and report
success. The migration is what makes the two agree.

Exercised by putting a row in the old shape back and letting the migration run
again, which is the only honest way to see a file that has already been applied.
*/
func TestMigrate_conversationsStoredUnderTheIdAlone_takeTheirConnection(t *testing.T) {
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

	const version = "0071_conversations_by_connection"
	for _, one := range []struct{ name, value string }{
		{"C-LEGACY", `{"channel":"acme-slack","mode":"mentions"}`},
		// Nothing to hang it on: left as it is rather than given an invented
		// connection.
		{"C-ORPHANED", `{"mode":"mentions"}`},
	} {
		if _, err := pool.Exec(t.Context(), `
			insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by)
			values ('area', 'acme', 'legacy', 'channel_conversation', $1, $2, true, 'restore')
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
			`delete from settings where company_id = 'acme' and area_id = 'legacy'`)
	})

	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate again: %v", err)
	}

	var names []string
	rows, err := pool.Query(t.Context(), `
		select name from settings
		where kind = 'channel_conversation' and company_id = 'acme' and area_id = 'legacy'
		order by name`)
	if err != nil {
		t.Fatalf("read the rows back: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		names = append(names, name)
	}
	if len(names) != 2 || names[0] != "C-ORPHANED" || names[1] != "acme-slack/C-LEGACY" {
		t.Fatalf("names = %v, want the one with a connection renamed and the other left", names)
	}
}
