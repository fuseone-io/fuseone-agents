package admin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

func newBranding(t *testing.T) (*admin.Branding, *pgxpool.Pool) {
	t.Helper()
	pool := freshPool(t)
	key := make([]byte, 32)
	v, err := vault.New(key, "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return admin.NewBranding(pool, settings.NewStore(pool, v)), pool
}

func TestBranding_unconfiguredUsesTheProductBrand(t *testing.T) {
	store, _ := newBranding(t)

	got, err := store.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got.DisplayName != admin.DefaultBranding.DisplayName {
		t.Errorf("display = %q, want the built-in product name", got.DisplayName)
	}
}

func TestBranding_storesWhatTheInstallationPresents(t *testing.T) {
	store, _ := newBranding(t)
	ctx := context.Background()

	err := store.Set(ctx, "usr_ana", domain.Scope{}, admin.BrandingConfig{
		DisplayName:  "Acme Agents",
		LogoURL:      "https://assets.example/acme.svg",
		IconURL:      "data:image/png;base64,aGVsbG8=",
		PrimaryColor: "#2357C6",
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got.DisplayName != "Acme Agents" || got.LogoURL == "" || got.IconURL == "" || got.PrimaryColor != "#2357C6" {
		t.Fatalf("Current = %+v", got)
	}
}

func TestBranding_refusesNonImageURLsAndInvalidColours(t *testing.T) {
	store, _ := newBranding(t)
	ctx := context.Background()

	for _, config := range []admin.BrandingConfig{
		{DisplayName: "Acme", LogoURL: "javascript:alert(1)"},
		{DisplayName: "Acme", PrimaryColor: "2357C6"},
		{DisplayName: ""},
	} {
		if err := store.Set(ctx, "usr_ana", domain.Scope{}, config); !errors.Is(err, admin.ErrBrandingInvalid) {
			t.Fatalf("Set(%+v) = %v, want invalid branding", config, err)
		}
	}
}

func TestBranding_changingItIsRecordedWithoutDumpingURLs(t *testing.T) {
	store, handle := newBranding(t)
	ctx := context.Background()

	if err := store.Set(ctx, "usr_ana", domain.Scope{}, admin.BrandingConfig{
		DisplayName:  "Acme Agents",
		LogoURL:      "https://assets.example/acme.svg",
		PrimaryColor: "#2357C6",
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	action, detail := lastTrailDetail(t, handle, "installation")
	if action != "branding.changed" {
		t.Fatalf("action = %q", action)
	}
	if detail == "" || strings.Contains(detail, "assets.example") {
		t.Fatalf("detail = %s", detail)
	}
}
