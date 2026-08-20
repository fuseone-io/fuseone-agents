package httpapi

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

type fakeMoney struct {
	config admin.MoneyConfig
	set    admin.MoneyConfig
	err    error
}

func (f *fakeMoney) Current(context.Context) (admin.MoneyConfig, error) {
	if f.config.Currency == "" {
		return admin.DefaultMoney, nil
	}
	return f.config, nil
}

func (f *fakeMoney) Set(
	_ context.Context, _ domain.UserID, _ domain.Scope, config admin.MoneyConfig,
) error {
	if f.err != nil {
		return f.err
	}
	f.set = config
	return nil
}

func monetised(store *fakeMoney) *Server {
	return NewServer(ledger.NewMemory(), "test").WithMoney(store)
}

func TestGetMoney_namesTheUnitCostMicrosUses(t *testing.T) {
	t.Parallel()

	resp, err := monetised(&fakeMoney{config: admin.MoneyConfig{Currency: "USD"}}).
		GetMoney(context.Background(), openapi.GetMoneyRequestObject{})
	if err != nil {
		t.Fatalf("GetMoney: %v", err)
	}
	got := resp.(openapi.GetMoney200JSONResponse)
	if got.Currency != "USD" {
		t.Fatalf("currency = %q, want USD", got.Currency)
	}
}

func TestSetMoney_requiresBudgetWriteBecauseCeilingsReadIt(t *testing.T) {
	t.Parallel()

	store := &fakeMoney{}
	resp, err := monetised(store).SetMoney(as(domain.RoleApprover),
		openapi.SetMoneyRequestObject{Body: &openapi.MoneySettings{Currency: "USD"}})
	if err != nil {
		t.Fatalf("SetMoney: %v", err)
	}
	if _, ok := resp.(openapi.SetMoney403ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want forbidden", resp)
	}
	if store.set.Currency != "" {
		t.Fatalf("refused request still wrote %+v", store.set)
	}
}

func TestSetMoney_invalidCurrencyIsABadRequest(t *testing.T) {
	t.Parallel()

	resp, err := monetised(&fakeMoney{err: admin.ErrMoneyInvalid}).
		SetMoney(as(domain.RoleCurator),
			openapi.SetMoneyRequestObject{Body: &openapi.MoneySettings{Currency: "real"}})
	if err != nil {
		t.Fatalf("SetMoney: %v", err)
	}
	if _, ok := resp.(openapi.SetMoney400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want bad request", resp)
	}
}
