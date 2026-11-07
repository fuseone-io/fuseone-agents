package admin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// One name in two areas is two instances, and asking for one at company scope is asking
// for a third that does not exist.
func TestConnectorInstanceNamed_readsTheExactScope(t *testing.T) {
	instances, _, _ := newConnectorInstances(t)
	ctx := context.Background()
	token := "connector-named-token"
	for _, area := range []domain.AreaID{"payments", "platform"} {
		instance := vaultInstance("app-x", true)
		instance.Vault.Mount = string(area)
		if err := instances.PutConnectorInstance(ctx, "usr_ana", platform, settings.ScopeArea,
			domain.Scope{Company: "acme", Area: area}, instance, &token, false); err != nil {
			t.Fatalf("PutConnectorInstance %s: %v", area, err)
		}
	}

	got, revision, err := instances.ConnectorInstanceNamed(ctx, settings.ScopeArea,
		domain.Scope{Company: "acme", Area: "platform"}, "app-x")
	if err != nil {
		t.Fatalf("ConnectorInstanceNamed: %v", err)
	}
	if got.Vault.Mount != "platform" || got.Scope.Area != "platform" || !got.HasToken || revision == "" {
		t.Fatalf("read %+v at %q, want the platform instance", got, revision)
	}

	_, _, err = instances.ConnectorInstanceNamed(ctx, settings.ScopeCompany, domain.Scope{Company: "acme"}, "app-x")
	if !errors.Is(err, admin.ErrConnectorInstanceUnknown) {
		t.Fatalf("company scope = %v, want ErrConnectorInstanceUnknown", err)
	}
}
