package admin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// A binding change has to be readable in the audit trail, or the record says
// somebody edited a SQL instance without saying what they bound it to.
func TestConnectorInstanceDetail_recordsTheBindingAndNoCredential(t *testing.T) {
	t.Parallel()

	detail := connectorInstanceDetail(connectortools.Instance{
		Connector: "sql", Name: "app-x", Enabled: true, Token: "DETAIL-CANARY",
		SQL: connectortools.SQLConfig{
			Host: "db.internal", Port: 5432, Database: "appx",
			CredentialSource: connectortools.CredentialSource{
				Kind:          connectortools.CredentialVaultDatabaseRole,
				VaultInstance: "prod", Mount: "database", Role: "app-x-readonly",
			},
		},
	}, false, false)

	sql, ok := detail["sql"].(map[string]any)
	if !ok {
		t.Fatal("a sql instance recorded no binding")
	}
	source, ok := sql["credentialSource"].(map[string]any)
	if !ok || source["vaultInstance"] != "prod" || source["role"] != "app-x-readonly" {
		t.Fatalf("credentialSource = %+v, want the vault instance and role", source)
	}
	if rendered := fmt.Sprint(detail); strings.Contains(rendered, "DETAIL-CANARY") {
		t.Fatalf("the audit detail carries the token: %s", rendered)
	}
}

/*
The row a write replaces is the one under its key, not every row sharing a name.

Two instances may carry one name in different scopes. Matching on the name
alone dropped both from the validation set, so editing an area's vault removed
the company's vault as well and let a configuration validate as unambiguous
and then be written ambiguous.
*/
func TestSameSetting_matchesOneRowByItsStoredKey(t *testing.T) {
	t.Parallel()

	areaScope := domain.Scope{Company: "acme", Area: "platform"}
	companyScope := domain.Scope{Company: "acme"}
	inArea := connectortools.ConfiguredInstance{
		Instance:  connectortools.Instance{Connector: "vault", Name: "prod", Scope: areaScope},
		ScopeKind: settings.ScopeArea,
	}
	inCompany := connectortools.ConfiguredInstance{
		Instance:  connectortools.Instance{Connector: "vault", Name: "prod", Scope: companyScope},
		ScopeKind: settings.ScopeCompany,
	}

	if !sameSetting(inArea, areaScope, "prod") {
		t.Error("the row being edited was not recognised as itself")
	}
	if sameSetting(inCompany, areaScope, "prod") {
		t.Error("a namesake in another scope was removed from the set")
	}
	// The connector is deliberately not part of the key: a body may change the
	// connector stored under one, and the row being replaced is still that
	// key's row.
	other := inArea
	other.Connector = "smtp"
	if !sameSetting(other, areaScope, "prod") {
		t.Error("changing the connector under a key made the write miss its own row")
	}
}
