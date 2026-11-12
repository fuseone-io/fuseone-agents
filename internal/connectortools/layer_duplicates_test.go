package connectortools

import (
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
One name is one instance (NT-019, phase 0).

The tool id carries no scope, so two rows sharing a connector and a name would
both answer to vault.prod.*, and the map would keep whichever came last. The
database refuses such a pair now; the layer refuses it too, rather than pick
one, and keeps serving the set it had.
*/
func TestSetInstances_oneNameInTwoScopes_isRefusedAndThePreviousSetStays(t *testing.T) {
	t.Parallel()
	cx := Instance{Name: "prod", Connector: "vault", Enabled: true,
		Scope: domain.Scope{Company: "acme", Area: "cx"}}
	layer := newVaultLayer(t, cx)

	finance := cx
	finance.Scope = domain.Scope{Company: "acme", Area: "finance"}
	err := layer.SetInstances([]Instance{cx, finance})
	if !errors.Is(err, ErrDuplicateInstance) {
		t.Fatalf("err = %v, want ErrDuplicateInstance", err)
	}

	served, _, found := layer.native("vault.prod.read_metadata")
	if !found || served.Scope != cx.Scope {
		t.Errorf("served %+v (found %v), want the previous set's %s", served.Scope, found, cx.Scope)
	}
}
