package ledger_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
)

// ContentStore is what the engine needs of a claim check.
type ContentStore interface {
	Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error)
	PutFor(ctx context.Context, kind, owner string, seq int64, data []byte) (string, error)
	Get(ctx context.Context, ref string) ([]byte, error)
	Erase(ctx context.Context, owner string, reason string) (int, error)
	ErasePast(ctx context.Context, before time.Time, reason string) (int, error)
}

func contentStores(t *testing.T) map[string]func(*testing.T) ContentStore {
	t.Helper()

	return boundedStores(t, 0)
}

/*
boundedStores is the same pair with a payload limit set.

Both stores take one, and the fake bounds exactly what Postgres bounds: a fake
that accepted what production truncates would let every suite using it certify
behaviour the real thing does not have.
*/
func boundedStores(t *testing.T, limit int) map[string]func(*testing.T) ContentStore {
	t.Helper()

	stores := map[string]func(*testing.T) ContentStore{
		"memory": func(*testing.T) ContentStore {
			return engine.NewMemoryContent().WithLimit(limit)
		},
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
		return ledger.NewContent(pool).WithLimit(limit)
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

		t.Run(name+"/a case set and a run holding the same bytes are separate objects", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			runRef, err := store.Put(ctx, "run_a", 1, []byte(`{"assunto":"cobrança"}`))
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			caseRef, err := store.PutFor(ctx, "case", "suporte", 1, []byte(`{"assunto":"cobrança"}`))
			if err != nil {
				t.Fatalf("PutFor: %v", err)
			}

			// Retention and per-subject erasure work per owner (AU-11,
			// NF-09), so a purged run must not take a case set with it.
			if runRef == caseRef {
				t.Fatalf("one reference for two owners: %s", runRef)
			}
			if _, err := store.Get(ctx, runRef); err != nil {
				t.Errorf("run content gone: %v", err)
			}
			if _, err := store.Get(ctx, caseRef); err != nil {
				t.Errorf("case content gone: %v", err)
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

// Retention and per-subject erasure both reach the referenced content and
// never the step that references it (AU-11, NF-09). That is what keeps the
// hash chain intact while the personal data goes: the step keeps a reference
// and a digest, and neither changes.

func TestEraseContract(t *testing.T) {
	for name, open := range contentStores(t) {
		t.Run(name+"/erasing an owner leaves a tombstone, not a hole", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			ref, err := store.Put(ctx, "run-1", 1, []byte(`{"email":"ana@exemplo.com.br"}`))
			if err != nil {
				t.Fatalf("Put: %v", err)
			}
			erased, err := store.Erase(ctx, "run-1", "subject")
			if err != nil {
				t.Fatalf("Erase: %v", err)
			}
			if erased != 1 {
				t.Errorf("erased %d objects, want 1", erased)
			}

			// Erased and never-stored are different facts, and an auditor
			// reading a trail that points at nothing has to be able to tell
			// them apart: one is a deletion somebody performed, the other is a
			// reference that was always wrong.
			_, err = store.Get(ctx, ref)
			if !errors.Is(err, ledger.ErrErased) {
				t.Errorf("Get = %v, want it reported as erased", err)
			}
		})

		t.Run(name+"/erasing one owner leaves another alone", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			mine, _ := store.Put(ctx, "run-mine", 1, []byte("mine"))
			theirs, _ := store.Put(ctx, "run-theirs", 1, []byte("theirs"))
			if _, err := store.Erase(ctx, "run-mine", "subject"); err != nil {
				t.Fatalf("Erase: %v", err)
			}

			if _, err := store.Get(ctx, mine); !errors.Is(err, ledger.ErrErased) {
				t.Errorf("Get(mine) = %v", err)
			}
			// Erasure is per subject. Taking a neighbour's data with it would
			// be the same failure as not erasing at all, in the other
			// direction.
			if _, err := store.Get(ctx, theirs); err != nil {
				t.Errorf("Get(theirs) = %v, want it untouched", err)
			}
		})

		t.Run(name+"/retention reaches only what is old enough", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			ref, _ := store.Put(ctx, "run-1", 1, []byte("recent"))
			// Nothing is old yet, and a purge that took today's data because
			// nothing was configured would be the worst possible default.
			count, err := store.ErasePast(ctx, time.Now().Add(-time.Hour), "retention")
			if err != nil {
				t.Fatalf("ErasePast: %v", err)
			}
			if count != 0 {
				t.Errorf("erased %d, want none", count)
			}
			if _, err := store.Get(ctx, ref); err != nil {
				t.Errorf("Get = %v, want it kept", err)
			}
		})

		t.Run(name+"/erasing twice is not an error", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			if _, err := store.Put(ctx, "run-1", 1, []byte("x")); err != nil {
				t.Fatalf("Put: %v", err)
			}
			if _, err := store.Erase(ctx, "run-1", "subject"); err != nil {
				t.Fatalf("first: %v", err)
			}
			// A retry after a failed erasure must not refuse: the caller is
			// asking for a state, not for an event.
			again, err := store.Erase(ctx, "run-1", "subject")
			if err != nil {
				t.Fatalf("second: %v", err)
			}
			if again != 0 {
				t.Errorf("erased %d the second time, want none left", again)
			}
		})
	}
}

func TestPut_beyondTheLimit_keepsAPrefixAndSaysSo(t *testing.T) {
	for name, open := range boundedStores(t, 1024) {
		t.Run(name+"/a payload past the limit is truncated, not refused", func(t *testing.T) {
			store := open(t)
			ctx := context.Background()

			// A tool that returns a database dump would otherwise put it in a
			// row. Refusing outright is worse than truncating: the run needs
			// an answer, and a call that already reached the far side cannot
			// be un-made by the store declining to remember it.
			huge := bytes.Repeat([]byte("a"), 4096)

			ref, err := store.Put(ctx, "run-1", 1, huge)
			if err != nil {
				t.Fatalf("Put: %v", err)
			}

			got, err := store.Get(ctx, ref)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if len(got) != 1024 {
				t.Errorf("stored %d bytes, want the limit", len(got))
			}

			// The digest is of the whole thing, so somebody holding the
			// original can still prove it is what the run used.
			whole := sha256.Sum256(huge)
			if !strings.Contains(ref, hex.EncodeToString(whole[:])[:16]) {
				t.Errorf("ref = %q, want it to carry the digest of the whole payload", ref)
			}
		})
	}
}
