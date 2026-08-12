package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

// Configuration is what an operator wrote; health is what happened when the
// platform tried. The screen shows both, which means an observation nobody can
// act on any more is a row nobody can get rid of.

func newHealth(t *testing.T) *admin.Health {
	t.Helper()
	pool := openPool(t)
	if _, err := pool.Exec(context.Background(), `delete from integration_health`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return admin.NewHealth(pool)
}

func TestForget_aRemovedServerStopsBeingRemembered(t *testing.T) {
	health := newHealth(t)
	ctx := context.Background()

	if err := health.Record(ctx, domain.IntegrationHealth{
		Name: "crmweb", Reachable: true, ToolCount: 3,
		ObservedAt: time.Now(), ObservedBy: "worker-1",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := health.Forget(ctx, "crmweb"); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	// Without this a server somebody removed keeps appearing for ever as
	// something nobody configured — with no edit, no delete, and no way to
	// make it go away.
	all, err := health.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if _, still := all["crmweb"]; still {
		t.Error("a removed server is still remembered")
	}
}

func TestForget_oneNobodyEverObserved_isNotAnError(t *testing.T) {
	if err := newHealth(t).Forget(context.Background(), "nunca"); err != nil {
		t.Errorf("Forget: %v", err)
	}
}

func TestDeleteMCPServer_forgetsWhatWasObservedAboutIt(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `delete from integration_health`); err != nil {
		t.Fatalf("clean: %v", err)
	}

	health := admin.NewHealth(pool)
	if err := health.Record(ctx, domain.IntegrationHealth{
		Name: "crmweb", Reachable: true, ObservedAt: time.Now(), ObservedBy: "worker-1",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	integrations := newIntegrations(t).ForgettingHealth(health)
	if err := integrations.DeleteMCPServer(ctx, "usr_ana", platform, "crmweb"); err != nil {
		t.Fatalf("DeleteMCPServer: %v", err)
	}

	// Removing a server has to remove it from the screen. Leaving the
	// observation behind turns a deletion into a rename: the row stays, and
	// stops being editable.
	all, err := health.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if _, still := all["crmweb"]; still {
		t.Error("the removed server is still on the screen")
	}
}
