package admin_test

import (
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

// A stored price list is the installation's own override. Market defaults ship
// separately, because a negotiated contract rate is the only number the
// installation itself supplied.

func TestPutPrice_ratesComeBackPerModel(t *testing.T) {
	i := newIntegrations(t)

	err := i.PutPrice(t.Context(), "usr_a", domain.Scope{}, admin.ModelPrice{
		Provider: "anthropic", Model: "claude-opus-5",
		InputMicros: 5_000_000, OutputMicros: 25_000_000,
		CacheReadMicros: 500_000, CacheWriteMicros: 6_250_000,
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := i.Prices(t.Context())
	if err != nil {
		t.Fatalf("prices: %v", err)
	}
	if len(got) != 1 || got[0].Model != "claude-opus-5" || got[0].CacheReadMicros != 500_000 {
		t.Fatalf("got %+v", got)
	}
}

func TestPutPrice_cacheReadIsItsOwnRate(t *testing.T) {
	i := newIntegrations(t)

	// Cache reads cost a fraction of input, and collapsing them into one
	// number is what makes an agent's cost impossible to diagnose (PRD FO-08).
	err := i.PutPrice(t.Context(), "usr_a", domain.Scope{}, admin.ModelPrice{
		Provider: "anthropic", Model: "m", InputMicros: 5_000_000,
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	got, _ := i.Prices(t.Context())
	if len(got) != 1 || got[0].CacheReadMicros != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestPutPrice_withoutAModel_isRefused(t *testing.T) {
	i := newIntegrations(t)

	// A rate for "anthropic" and no model would price every model the same,
	// which is wrong by an order of magnitude between the largest and the
	// smallest in the same family.
	if err := i.PutPrice(t.Context(), "usr_a", domain.Scope{},
		admin.ModelPrice{Provider: "anthropic"}); err == nil {
		t.Error("want a refusal")
	}
}
