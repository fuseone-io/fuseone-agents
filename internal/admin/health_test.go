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

func TestRecord_aFailedDiscoveryKeepsTheLastSuccessfulProbe(t *testing.T) {
	health := newHealth(t)
	ctx := context.Background()
	answered := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	failed := answered.Add(10 * time.Minute)

	if err := health.Record(ctx, domain.IntegrationHealth{
		Name: "crm", Reachable: true, ToolCount: 3,
		ObservedAt: answered, ObservedBy: "worker-a",
	}); err != nil {
		t.Fatalf("Record success: %v", err)
	}
	if err := health.Record(ctx, domain.IntegrationHealth{
		Name: "crm", Reachable: false, Detail: "connection refused",
		ObservedAt: failed, ObservedBy: "worker-b",
	}); err != nil {
		t.Fatalf("Record failure: %v", err)
	}

	all, err := health.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	got := all["crm"]
	if got.Reachable || got.LastReachableAt == nil || !got.LastReachableAt.Equal(answered) {
		t.Fatalf("health = %#v, want failure with last success preserved", got)
	}
}

func TestRecordToolCall_aFailedCallKeepsTheLastSuccessfulCall(t *testing.T) {
	health := newHealth(t)
	ctx := context.Background()
	answered := time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC)
	failed := answered.Add(10 * time.Minute)

	if err := health.Record(ctx, domain.IntegrationHealth{
		Name: "crm", Reachable: true, ToolCount: 3,
		ObservedAt: answered, ObservedBy: "worker-a",
	}); err != nil {
		t.Fatalf("Record discovery: %v", err)
	}
	if err := health.RecordToolCall(ctx, domain.IntegrationToolCallObservation{
		Name: "crm", OK: true, Code: "none",
		ObservedAt: answered, ObservedBy: "worker-a",
	}); err != nil {
		t.Fatalf("RecordToolCall success: %v", err)
	}
	if err := health.RecordToolCall(ctx, domain.IntegrationToolCallObservation{
		Name: "crm", OK: false, Code: "mcp_personal_credential_missing",
		ObservedAt: failed, ObservedBy: "worker-b",
	}); err != nil {
		t.Fatalf("RecordToolCall failure: %v", err)
	}

	all, err := health.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	got := all["crm"]
	if got.ToolCall == nil {
		t.Fatal("tool-call health missing")
	}
	if got.ToolCall.OK || got.ToolCall.Code != "mcp_personal_credential_missing" ||
		got.ToolCall.LastOKAt == nil || !got.ToolCall.LastOKAt.Equal(answered) {
		t.Fatalf("tool-call health = %#v, want failed call with last success preserved", got.ToolCall)
	}
	if !got.Reachable {
		t.Fatalf("discovery was overwritten by tool-call health: %#v", got)
	}
}

func TestRecordToolCall_withoutDiscoveryDoesNotInventDiscovery(t *testing.T) {
	health := newHealth(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 24, 15, 0, 0, 0, time.UTC)

	if err := health.RecordToolCall(ctx, domain.IntegrationToolCallObservation{
		Name: "crm", OK: true, Code: "none",
		ObservedAt: at, ObservedBy: "worker-a",
	}); err != nil {
		t.Fatalf("RecordToolCall: %v", err)
	}

	all, err := health.All(ctx)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if _, exists := all["crm"]; exists {
		t.Fatalf("tool-call health invented discovery: %#v", all["crm"])
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
