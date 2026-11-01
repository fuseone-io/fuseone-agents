package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The runtime page shows the event dispatch backlog (#228) as ages computed by
the server's clock, and the health signal degrades once the oldest waiting
run has waited past the threshold.
*/

var finishedAt = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func finishedRunAt(t *testing.T, at time.Time) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	for _, kind := range []domain.StepKind{domain.StepRunStarted, domain.StepRunFinished} {
		if _, err := store.Append(context.Background(), domain.Step{
			RunID: "run-a", Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: "triage", VersionID: "v1", OnBehalfOf: "ana", At: at,
		}); err != nil {
			t.Fatalf("append %s: %v", kind, err)
		}
	}
	return store
}

func runtimeHealthAt(t *testing.T, ctx context.Context, store *ledger.Memory, now time.Time) openapi.RuntimeHealth {
	t.Helper()
	resp, err := NewServer(store, "test").WithClock(fixedAt{t: now}).
		GetRuntimeHealth(ctx, openapi.GetRuntimeHealthRequestObject{})
	if err != nil {
		t.Fatalf("GetRuntimeHealth: %v", err)
	}
	health, ok := resp.(openapi.GetRuntimeHealth200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the health", resp)
	}
	return openapi.RuntimeHealth(health)
}

func TestGetRuntimeHealth_aRunWaitingPastTheThreshold_degradesEventDispatch(t *testing.T) {
	t.Parallel()
	now := finishedAt.Add(domain.EventDispatchDegradedAfter + time.Minute)

	got := runtimeHealthAt(t, asInstallation(domain.RoleAdmin), finishedRunAt(t, finishedAt), now).EventDispatch
	if got == nil {
		t.Fatal("no event dispatch section for an installation-wide reader")
	}
	if got.Status != openapi.EventDispatchDegraded || got.Ready != 1 || got.LagSeconds != 360 {
		t.Fatalf("event dispatch = %+v; want degraded, one ready, 360s of lag", *got)
	}
	if got.DegradedAfterSeconds != 300 || got.XactOpen || !got.XactAgeKnown || got.XactAgeSeconds != nil {
		t.Fatalf("event dispatch = %+v; want the 300s threshold and no open transaction", *got)
	}
}

func TestGetRuntimeHealth_aRunJustFinished_isHealthy(t *testing.T) {
	t.Parallel()
	now := finishedAt.Add(20 * time.Second)

	got := runtimeHealthAt(t, asInstallation(domain.RoleAdmin), finishedRunAt(t, finishedAt), now).EventDispatch
	if got == nil || got.Status != openapi.EventDispatchOk || got.LagSeconds != 20 {
		t.Fatalf("event dispatch = %+v; want ok with 20s of lag", got)
	}
}

func TestGetRuntimeHealth_anAreaReader_seesNoEventDispatch(t *testing.T) {
	t.Parallel()

	got := runtimeHealthAt(t, inArea("cx", domain.RoleAdmin), finishedRunAt(t, finishedAt), finishedAt)
	if got.EventDispatch != nil {
		t.Fatalf("event dispatch = %+v; an area reader sees no installation-wide backlog", *got.EventDispatch)
	}
}

// A holder whose start the role cannot see has no age: absent, not zero,
// because zero would read as nothing open.
func TestRuntimeEventDispatchFrom_aHiddenHolder_hasNoAge(t *testing.T) {
	t.Parallel()
	got := runtimeEventDispatchFrom(domain.EventDispatchBacklog{
		HeldByXmin: 2, OldestFinishedAt: finishedAt, Xact: domain.OpenXact{Open: true},
	}, finishedAt.Add(time.Minute))

	if !got.XactOpen || got.XactAgeKnown || got.XactAgeSeconds != nil || got.HeldByXmin != 2 {
		t.Fatalf("event dispatch = %+v; want the holder open with no age", got)
	}
}

// A count stopped at the cap says so, and says where the cap is, so the page
// reads "1000+" rather than a backlog of exactly a thousand.
func TestRuntimeEventDispatchFrom_aCappedCount_saysItIsALowerBound(t *testing.T) {
	t.Parallel()
	got := runtimeEventDispatchFrom(domain.EventDispatchBacklog{
		Ready:      domain.EventDispatchCountCap,
		HeldByXmin: domain.EventDispatchCountCap, OldestFinishedAt: finishedAt, HeldCapped: true,
	}, finishedAt.Add(time.Minute))

	// A thousand ready exactly, and more than a thousand held: only the
	// held count is a lower bound.
	if got.ReadyCapped || !got.HeldCapped || got.CountCap != domain.EventDispatchCountCap {
		t.Fatalf("event dispatch = %+v; want the count marked capped at %d", got, domain.EventDispatchCountCap)
	}
}
