package connectortools

import (
	"context"
	"errors"
	"testing"
)

func TestSQLContractDigest_pinsTheSelectedExecutionContract(t *testing.T) {
	t.Parallel()

	base := ready()[1].SQL
	vault := ready()[0].Vault
	want, ok := sqlContractDigest(base, vault, "orders_by_customer")
	if !ok || want == "" {
		t.Fatal("the registered template produced no contract digest")
	}

	unrelated := base
	unrelated.Templates = append([]SQLTemplate(nil), base.Templates...)
	unrelated.Templates = append(unrelated.Templates, SQLTemplate{ID: "another_question", SQL: "select 1"})
	if got, _ := sqlContractDigest(unrelated, vault, "orders_by_customer"); got != want {
		t.Fatalf("an unrelated template changed the selected contract: %q != %q", got, want)
	}

	mutations := map[string]func(*SQLConfig){
		"database target": func(cfg *SQLConfig) { cfg.Host = "replacement.internal" },
		"vault binding":   func(cfg *SQLConfig) { cfg.CredentialSource.Role = "another-role" },
		"query":           func(cfg *SQLConfig) { cfg.Templates[0].SQL += " and 1 = 1" },
		"parameter type": func(cfg *SQLConfig) {
			cfg.Templates[0].Parameters[0].Type = SQLParamInteger
		},
		"row limit": func(cfg *SQLConfig) { cfg.Templates[0].MaxRows++ },
	}
	for name, mutate := range mutations {
		changed := base
		changed.Templates = append([]SQLTemplate(nil), base.Templates...)
		changed.Templates[0].Parameters = append(
			[]SQLParameter(nil), base.Templates[0].Parameters...)
		mutate(&changed)
		got, ok := sqlContractDigest(changed, vault, "orders_by_customer")
		if !ok || got == want {
			t.Errorf("%s did not change the contract digest", name)
		}
	}
	for name, changed := range map[string]VaultConfig{
		"address":   {Address: "https://replacement-vault.internal"},
		"namespace": {Address: vault.Address, Namespace: "another-tenant"},
	} {
		if got, _ := sqlContractDigest(base, changed, "orders_by_customer"); got == want {
			t.Errorf("changing the Vault %s did not change the contract digest", name)
		}
	}
}

func TestSQLNativeTool_reserveRefusesAContractThatMovedAfterTheGate(t *testing.T) {
	t.Parallel()

	issuer := &rotatingSQLIssuer{}
	layer := newSQLLayer(t, issuer, &recordingSQLExecutor{}, &cachingBase{})
	call := boundSQLLayerCall(layer, 1)
	changed := ready()
	changed[1].SQL.Templates[0].MaxRows++
	if err := layer.SetInstances(changed); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	if err := layer.Reserve(context.Background(), call); !errors.Is(err, ErrSQLContractChanged) {
		t.Fatalf("Reserve error = %v, want moved contract", err)
	}
	if issued, _ := issuer.snapshot(); len(issued) != 0 {
		t.Fatalf("stale contract issued %d credentials", len(issued))
	}
}

func TestSQLNativeTool_reserveRefusesARepointedVaultAfterTheGate(t *testing.T) {
	t.Parallel()

	layer := newSQLLayer(t, &rotatingSQLIssuer{}, &recordingSQLExecutor{}, &cachingBase{})
	call := boundSQLLayerCall(layer, 1)
	changed := ready()
	changed[0].Vault.Address = "https://replacement-vault.internal"
	if err := layer.SetInstances(changed); err != nil {
		t.Fatalf("SetInstances: %v", err)
	}
	if err := layer.Reserve(context.Background(), call); !errors.Is(err, ErrSQLContractChanged) {
		t.Fatalf("Reserve error = %v, want moved contract", err)
	}
}
