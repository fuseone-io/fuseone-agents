package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

// Retention is the one setting that destroys data. Everything here is about
// it destroying only what somebody asked it to.

func newRetention(t *testing.T) *admin.Retention {
	t.Helper()
	pool := openPool(t)
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind = 'retention'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	key := make([]byte, 32)
	v, err := vault.New(key, "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return admin.NewRetention(pool, settings.NewStore(pool, v))
}

func TestRetention_unconfigured_isFiveYears(t *testing.T) {
	got, err := newRetention(t).Window(context.Background())
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	// The PRD's default, and it has to be a real number rather than "none":
	// an installation nobody configured still has an obligation to not keep
	// personal data for ever.
	if got != admin.DefaultRetention {
		t.Errorf("window = %v, want five years", got)
	}
}

func TestRetention_shorterThanADay_isRefused(t *testing.T) {
	store := newRetention(t)

	// Fat-fingering a zero into this field would erase the installation's
	// content on the next sweep. It is the one setting where a typo is
	// unrecoverable, so the floor is not a matter of taste.
	if err := store.SetWindow(context.Background(), "usr_ana", domain.Scope{}, 0); err == nil {
		t.Fatal("want a refusal")
	}
	if err := store.SetWindow(context.Background(), "usr_ana", domain.Scope{}, -time.Hour); err == nil {
		t.Fatal("want a refusal for a negative window")
	}
}

func TestRetention_configured_isWhatComesBack(t *testing.T) {
	store := newRetention(t)
	ctx := context.Background()

	want := 90 * 24 * time.Hour
	if err := store.SetWindow(ctx, "usr_ana", domain.Scope{}, want); err != nil {
		t.Fatalf("SetWindow: %v", err)
	}
	got, err := store.Window(ctx)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if got != want {
		t.Errorf("window = %v, want %v", got, want)
	}
}

func TestRetention_changingIt_isRecordedInTheTrail(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	key := make([]byte, 32)
	v, _ := vault.New(key, "test")
	store := admin.NewRetention(pool, settings.NewStore(pool, v))

	if err := store.SetWindow(ctx, "usr_ana", domain.Scope{}, 30*24*time.Hour); err != nil {
		t.Fatalf("SetWindow: %v", err)
	}

	// Shortening retention destroys data on the next sweep. Who decided that,
	// and when, is exactly what an auditor asks about afterwards.
	action, _ := lastTrailDetail(t, pool, "retention")
	if action != "retention.changed" {
		t.Errorf("action = %q", action)
	}
}
