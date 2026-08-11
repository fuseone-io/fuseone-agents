package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// Every endpoint below predates authentication. When it arrived, only the code
// being written at that moment was covered, and these answered with the whole
// installation to anybody holding a session.

func storeWithRuns(t *testing.T) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	ctx := context.Background()

	for _, run := range []struct {
		id    domain.RunID
		scope domain.Scope
	}{
		{"run-cx", domain.Scope{Company: "acme", Area: "cx"}},
		{"run-mkt", domain.Scope{Company: "acme", Area: "marketing"}},
	} {
		if _, err := store.Append(ctx, domain.Step{
			RunID: run.id, Kind: domain.StepRunStarted, Scope: run.scope,
			AgentID: "triage", VersionID: "v1",
			At: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("seed %s: %v", run.id, err)
		}
	}
	return store
}

func TestListRuns_showsOnlyTheCallersAreas(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(storeWithRuns(t), "test").
		ListRuns(inArea("cx", domain.RoleAuthor), openapi.ListRunsRequestObject{})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	page := resp.(openapi.ListRuns200JSONResponse)
	if len(page.Items) != 1 || page.Items[0].RunId != "run-cx" {
		t.Errorf("items = %v, want only the caller's area", page.Items)
	}
}

func TestListRuns_holdingThePermissionNowhere_isRefusedNotEmptied(t *testing.T) {
	t.Parallel()

	// An empty page and a refusal mean different things to somebody working
	// out why a screen is blank.
	resp, err := NewServer(storeWithRuns(t), "test").
		ListRuns(context.Background(), openapi.ListRunsRequestObject{})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if _, refused := resp.(openapi.ListRuns403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestGetRun_inAnotherArea_readsAsAbsent(t *testing.T) {
	t.Parallel()

	// Not forbidden: telling somebody a run exists in an area they cannot see
	// is itself information about that area.
	resp, err := NewServer(storeWithRuns(t), "test").
		GetRun(inArea("cx", domain.RoleAuthor), openapi.GetRunRequestObject{RunId: "run-mkt"})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if _, absent := resp.(openapi.GetRun404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestListRunSteps_inAnotherArea_readsAsAbsent(t *testing.T) {
	t.Parallel()

	// The trail carries tool names, rules and reasons: reading another area's
	// is reading what its agents were asked to do.
	resp, err := NewServer(storeWithRuns(t), "test").
		ListRunSteps(inArea("cx", domain.RoleAuthor), openapi.ListRunStepsRequestObject{RunId: "run-mkt"})
	if err != nil {
		t.Fatalf("ListRunSteps: %v", err)
	}
	if _, absent := resp.(openapi.ListRunSteps404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestVerifyRun_inAnotherArea_readsAsAbsent(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(storeWithRuns(t), "test").
		VerifyRun(inArea("cx", domain.RoleAuditor), openapi.VerifyRunRequestObject{RunId: "run-mkt"})
	if err != nil {
		t.Fatalf("VerifyRun: %v", err)
	}
	if _, absent := resp.(openapi.VerifyRun404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestGetCostRollup_narrowsToTheCallersAreas(t *testing.T) {
	t.Parallel()

	window := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	resp, err := NewServer(storeWithRuns(t), "test").
		GetCostRollup(inArea("cx", domain.RoleApprover), openapi.GetCostRollupRequestObject{
			Params: openapi.GetCostRollupParams{
				From: window.Add(-time.Hour), To: window.Add(time.Hour),
			},
		})
	if err != nil {
		t.Fatalf("GetCostRollup: %v", err)
	}
	rollup := resp.(openapi.GetCostRollup200JSONResponse)
	// What an area costs is that area's business.
	for _, bucket := range rollup.Buckets {
		if bucket.Runs != 1 {
			t.Errorf("bucket %q has %d runs, want only the caller's", bucket.Key, bucket.Runs)
		}
	}
}

func TestGetRunStats_narrowsToTheCallersAreas(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(storeWithRuns(t), "test").
		GetRunStats(inArea("cx", domain.RoleAuthor), openapi.GetRunStatsRequestObject{})
	if err != nil {
		t.Fatalf("GetRunStats: %v", err)
	}
	stats := resp.(openapi.GetRunStats200JSONResponse)
	if stats.Total != 1 {
		t.Errorf("total = %d, want only the caller's area", stats.Total)
	}
}
