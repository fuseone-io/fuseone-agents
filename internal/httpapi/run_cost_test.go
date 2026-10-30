package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// The run page said "N in · 0 out" for a run that was mostly cache (#120):
// the single run was costed from the folded spend, which keeps one token
// total, and the mapping put that total under input. The money was right and
// the breakdown it came from was gone. The run carries what its steps were
// billed for, class by class, the same sum the run list reads.
func TestGetRun_costCarriesTheTokenBreakdownItsStepsWereBilledFor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

	for i, step := range []domain.Step{
		{Kind: domain.StepRunStarted},
		{Kind: domain.StepPlanned, Payload: mustPayload(t, domain.PlannedPayload{Node: "plan"}),
			Cost: domain.Cost{InputTokens: 40, OutputTokens: 300, CacheReadTokens: 9_000,
				CacheWriteTokens: 1_200, Micros: 20_000}},
		{Kind: domain.StepPlanned, Payload: mustPayload(t, domain.PlannedPayload{Node: "plan"}),
			Cost: domain.Cost{InputTokens: 2, OutputTokens: 150, CacheReadTokens: 10_200,
				CacheWriteTokens: 80, Micros: 9_000}},
	} {
		step.RunID, step.Scope, step.AgentID, step.VersionID = "run-cx", scope, "triage", "v1"
		step.At = at.Add(time.Duration(i) * time.Second)
		if _, err := store.Append(ctx, step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}

	resp, err := NewServer(store, "test").
		GetRun(inArea("cx", domain.RoleAuthor), openapi.GetRunRequestObject{RunId: "run-cx"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	got := resp.(openapi.GetRun200JSONResponse).Cost
	want := openapi.Cost{
		Micros: 29_000, InputTokens: ptr(int64(42)), OutputTokens: ptr(int64(450)),
		CacheReadTokens: ptr(int64(19_200)), CacheWriteTokens: ptr(int64(1_280)),
	}
	if got.Micros != want.Micros || valueOr(got.InputTokens) != 42 || valueOr(got.OutputTokens) != 450 ||
		valueOr(got.CacheReadTokens) != 19_200 || valueOr(got.CacheWriteTokens) != 1_280 {
		t.Errorf("cost = micros %d in %d out %d cache read %d write %d, want micros %d in 42 out 450 read 19200 write 1280",
			got.Micros, valueOr(got.InputTokens), valueOr(got.OutputTokens),
			valueOr(got.CacheReadTokens), valueOr(got.CacheWriteTokens), want.Micros)
	}
}
