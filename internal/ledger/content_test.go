package ledger_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
)

// ContentStore is what the engine needs of a claim check.
type ContentStore interface {
	Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error)
	Get(ctx context.Context, ref string) ([]byte, error)
}

func contentStores(t *testing.T) map[string]func(*testing.T) ContentStore {
	t.Helper()

	stores := map[string]func(*testing.T) ContentStore{
		"memory": func(*testing.T) ContentStore { return engine.NewMemoryContent() },
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		requireDatabase(t, dsn)
		t.Log("TEST_DATABASE_URL is unset; skipping the Postgres content store")
		return stores
	}

	stores["postgres"] = func(t *testing.T) ContentStore {
		pool, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
		t.Cleanup(pool.Close)
		if err := ledger.Migrate(context.Background(), pool); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		if _, err := pool.Exec(context.Background(), `truncate run_content`); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		return ledger.NewContent(pool)
	}
	return stores
}

func TestContentContract(t *testing.T) {
	for name, open := range contentStores(t) {
		t.Run(name+"/what goes in comes back", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			ref, err := store.Put(ctx, "run-1", 4, []byte(`{"email":"cliente@exemplo.com.br"}`))
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			got, err := store.Get(ctx, ref)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if string(got) != `{"email":"cliente@exemplo.com.br"}` {
				t.Errorf("Get = %q, want the bytes that went in", got)
			}
		})

		t.Run(name+"/an unknown reference is an error, not empty bytes", func(t *testing.T) {
			store := open(t)

			// Empty bytes would reach the model as an empty tool result and be
			// reasoned about as though the tool had returned nothing.
			if _, err := store.Get(context.Background(), "run://run-1/9/deadbeef"); err == nil {
				t.Error("Get invented content for a reference nothing was stored under")
			}
		})

		t.Run(name+"/storing the same bytes twice is idempotent", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			// A retry after a crash must not fail, and must not duplicate.
			first, err := store.Put(ctx, "run-1", 4, []byte("same"))
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			second, err := store.Put(ctx, "run-1", 4, []byte("same"))
			if err != nil {
				t.Fatalf("Put again: %v", err)
			}
			if first != second {
				t.Errorf("refs differ: %q then %q", first, second)
			}
		})

		t.Run(name+"/different runs holding the same bytes get their own reference", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			// Retention and per-subject erasure work per run: purging one must
			// not take the other's content with it.
			a, _ := store.Put(ctx, "run-a", 1, []byte("shared"))
			b, _ := store.Put(ctx, "run-b", 1, []byte("shared"))
			if a == b {
				t.Errorf("both runs share the reference %q", a)
			}
		})
	}
}
