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
	// One name, one scope: since NT-019 a second area cannot hold "app-x".
	instance := vaultInstance("app-x", true)
	instance.Vault.Mount = "platform"
	if err := instances.PutConnectorInstance(ctx, "usr_ana", platform, settings.ScopeArea,
		domain.Scope{Company: "acme", Area: "platform"}, instance, &token, false); err != nil {
		t.Fatalf("PutConnectorInstance platform: %v", err)
	}
	if err := instances.PutConnectorInstance(ctx, "usr_ana", platform, settings.ScopeArea,
		domain.Scope{Company: "acme", Area: "payments"}, instance, &token, false); !errors.Is(err, settings.ErrNameTaken) {
		t.Fatalf("a second area taking the name = %v, want ErrNameTaken", err)
	}

	got, revision, err := instances.ConnectorInstanceNamed(ctx, settings.ScopeArea,
		domain.Scope{Company: "acme", Area: "platform"}, "app-x")
	if err != nil {
		t.Fatalf("ConnectorInstanceNamed: %v", err)
	}
	if got.Vault.Mount != "platform" || got.Scope.Area != "platform" || !got.HasToken || revision == "" {
		t.Fatalf("read %+v at %q, want the platform instance", got, revision)
	}

	for _, other := range []struct {
		kind  settings.ScopeKind
		scope domain.Scope
	}{
		{settings.ScopeCompany, domain.Scope{Company: "acme"}},
		{settings.ScopeArea, domain.Scope{Company: "acme", Area: "payments"}},
	} {
		_, _, err = instances.ConnectorInstanceNamed(ctx, other.kind, other.scope, "app-x")
		if !errors.Is(err, admin.ErrConnectorInstanceUnknown) {
			t.Fatalf("%s = %v, want ErrConnectorInstanceUnknown", other.scope, err)
		}
	}
}
