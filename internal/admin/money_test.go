package admin_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

func TestMoney_unconfiguredUsesTheHistoricalCurrency(t *testing.T) {
	store, _ := newMoney(t)
	got, err := store.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != "BRL" {
		t.Fatalf("currency = %q, want BRL", got.Currency)
	}
}

func TestMoney_recordsTheInstallationCurrency(t *testing.T) {
	store, _ := newMoney(t)
	if err := store.Set(t.Context(), "usr_ana", domain.Scope{}, admin.MoneyConfig{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	got, err := store.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", got.Currency)
	}
}

func TestMoney_refusesValuesThatAreNotCurrencyCodes(t *testing.T) {
	store, _ := newMoney(t)
	for _, currency := range []string{"", "R$", "US", "USDT", "usdollar"} {
		if err := store.Set(t.Context(), "usr_ana", domain.Scope{}, admin.MoneyConfig{Currency: currency}); !errors.Is(err, admin.ErrMoneyInvalid) {
			t.Fatalf("Set(%q) = %v, want invalid money settings", currency, err)
		}
	}
}

func TestMoney_changingItIsRecordedWithoutPretendingToConvertHistory(t *testing.T) {
	store, pool := newMoney(t)
	if err := store.Set(t.Context(), "usr_ana", domain.Scope{}, admin.MoneyConfig{Currency: "USD"}); err != nil {
		t.Fatal(err)
	}

	action, detail := lastTrailDetail(t, pool, "installation")
	if action != "money.changed" {
		t.Fatalf("action = %q", action)
	}
	// Decoded rather than matched as text: the column is jsonb, and Postgres
	// reserialises it with its own spacing. Asserting the rendering makes the
	// test fail on a database formatting choice while the recorded fact is
	// exactly right — which is a test about the wrong thing.
	var recorded struct {
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal([]byte(detail), &recorded); err != nil {
		t.Fatalf("detail is not JSON: %v", err)
	}
	if recorded.Currency != "USD" {
		t.Fatalf("recorded currency = %q", recorded.Currency)
	}
}

func newMoney(t *testing.T) (*admin.Money, *pgxpool.Pool) {
	t.Helper()
	pool := openPool(t)
	if _, err := pool.Exec(t.Context(), `delete from settings where kind = 'money'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return admin.NewMoney(pool, settings.NewStore(pool, nil)), pool
}
