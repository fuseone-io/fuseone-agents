package admin_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

/*
A save that drops a replaced image must not judge "nobody refers to it" while
another branding write that refers to it is still uncommitted.

The competing write is staged by hand: a transaction that follows the same
protocol a save does — the branding lock first — and then writes a second
branding setting naming the image. Under READ COMMITTED the save cannot see
that row, so without the lock it deletes the image and the other setting
commits pointing at nothing.

The save runs in a goroutine and the test waits for one of two things: it
finished (no lock, the race already lost) or it is queued on the lock. Either
way the competitor then commits, and the image must still exist.
*/
func TestBranding_cleanupWaitsForAConcurrentSaveThatReferencesTheImage(t *testing.T) {
	store, assets, pool := newAssets(t)
	ctx := context.Background()
	old, next := put(t, assets, image(pngMagic, "race-old")), put(t, assets, image(pngMagic, "race-new"))
	set(t, store, admin.BrandAssetPath(old.SHA256), "")

	competitor := referenceInOpenTx(t, pool, old.SHA256)

	done := make(chan error, 1)
	go func() {
		done <- store.Set(ctx, "usr_bia", domain.Scope{}, admin.BrandingConfig{
			DisplayName: "Acme", LogoURL: admin.BrandAssetPath(next.SHA256),
		})
	}()
	early := waitForSaveOrLockWait(t, pool, done)

	if err := competitor.Commit(ctx); err != nil {
		t.Fatalf("commit competitor: %v", err)
	}
	if early == nil {
		err := <-done
		early = &err
	}
	if *early != nil {
		t.Fatalf("Set: %v", *early)
	}
	if !exists(t, assets, old.SHA256) {
		t.Fatal("the image a concurrently saved brand references was deleted")
	}
}

type openTx interface{ Commit(context.Context) error }

// referenceInOpenTx takes the branding lock and writes a second branding
// setting naming sha, leaving the transaction open.
func referenceInOpenTx(t *testing.T, pool *pgxpool.Pool, sha string) openTx {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 0))`,
		admin.BrandingLock); err != nil {
		t.Fatalf("lock: %v", err)
	}
	value, _ := json.Marshal(admin.BrandingConfig{DisplayName: "Other", LogoURL: admin.BrandAssetPath(sha)})
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	if err := settings.NewStore(pool, v).PutTx(ctx, tx, settings.Setting{
		ScopeKind: settings.ScopeInstallation, Kind: settings.KindBranding,
		Name: "race-other", Value: value, Enabled: true, UpdatedBy: "usr_caio",
	}); err != nil {
		t.Fatalf("put competing brand: %v", err)
	}
	return tx
}

// waitForSaveOrLockWait returns the save's result if it finished, or nil
// once it is queued on an advisory lock.
func waitForSaveOrLockWait(t *testing.T, pool *pgxpool.Pool, done <-chan error) *error {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			return &err
		default:
		}
		var waiting int
		if err := pool.QueryRow(context.Background(), `
			select count(*) from pg_stat_activity
			where wait_event_type = 'Lock' and wait_event = 'advisory'
			  and query like '%pg_advisory_xact_lock%' and pid <> pg_backend_pid()`).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting > 0 {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the save neither finished nor waited on the branding lock")
	return nil
}
