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
