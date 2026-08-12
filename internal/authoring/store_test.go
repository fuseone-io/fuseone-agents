package authoring_test

import (
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
)

// The authoring assistant points at a provider that Integrações already
// connected. It is a choice, not a second registry: a separate credential
// store would be a second place to leak from, to rotate and to audit.

func storeFor(t *testing.T) (*authoring.Store, *settings.Store) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the authoring suite")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `delete from settings where kind in ('authoring','model_provider')`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	keys, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	set := settings.NewStore(pool, keys)
	return authoring.NewStore(set), set
}

func TestChoose_providerNobodyConnected_refused(t *testing.T) {
	store, _ := storeFor(t)

	err := store.Choose(t.Context(), authoring.Choice{Provider: "fantasma", Model: "x"}, "usr_a")

	if !errors.Is(err, authoring.ErrNoProvider) {
		t.Fatalf("got %v, want ErrNoProvider", err)
	}
}

func TestCurrent_nothingChosen_reportsAuthoringIsOff(t *testing.T) {
	store, _ := storeFor(t)

	choice, err := store.Current(t.Context())
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	// An installation with no strong model still publishes agents through the
	// form. The interview is the good path, not the only one.
	if choice.Enabled {
		t.Errorf("got %+v, want authoring off", choice)
	}
}

func TestChoose_connectedProvider_becomesTheCurrentChoice(t *testing.T) {
	store, set := storeFor(t)
	connect(t, set, "anthropic")

	if err := store.Choose(t.Context(), authoring.Choice{
		Provider: "anthropic", Model: "claude-opus-5", Effort: "high",
	}, "usr_a"); err != nil {
		t.Fatalf("choose: %v", err)
	}

	choice, err := store.Current(t.Context())
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if !choice.Enabled || choice.Provider != "anthropic" || choice.Model != "claude-opus-5" {
		t.Errorf("got %+v", choice)
	}
}

func connect(t *testing.T, set *settings.Store, name string) {
	t.Helper()
	if err := set.Put(t.Context(), settings.Setting{
		ScopeKind: settings.ScopeInstallation,
		Kind:      settings.KindModelProvider,
		Name:      name,
		Value:     []byte(`{"kind":"anthropic"}`),
		Enabled:   true,
	}); err != nil {
		t.Fatalf("connect %s: %v", name, err)
	}
}
