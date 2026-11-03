package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
GET /work-items/measures (#201, PR 1): the ledger's weekly measures, narrowed
by the caller's own grants as the trail is, over a window the server's clock
decides.
*/

// measuresNow is the Friday after trailAt: its week, 2026-09-28, is current.
var (
	measuresNow  = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	measuresWeek = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
)

func measuresServer(store *ledger.Memory, now time.Time) *Server {
	return NewServer(store, "test").WithWorkItems(store).WithClock(fixedAt{t: now})
}

func measuresRequest(weeks int, agent string) openapi.GetWorkItemMeasuresRequestObject {
	req := openapi.GetWorkItemMeasuresRequestObject{}
	if weeks != 0 {
		req.Params.Weeks = &weeks
	}
	if agent != "" {
		req.Params.AgentId = &agent
	}
	return req
}

func measuresGet(t *testing.T, server *Server, ctx context.Context, req openapi.GetWorkItemMeasuresRequestObject) (openapi.WorkItemMeasures, []byte) {
	t.Helper()
	resp, err := server.GetWorkItemMeasures(ctx, req)
	if err != nil {
		t.Fatalf("GetWorkItemMeasures: %v", err)
	}
	page, ok := resp.(openapi.GetWorkItemMeasures200JSONResponse)
	if !ok {
		t.Fatalf("response = %T %+v, want 200", resp, resp)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return openapi.WorkItemMeasures(page), raw
}

func TestGetWorkItemMeasures_holdingRunReadNowhere_isRefused(t *testing.T) {
	t.Parallel()
	resp, err := measuresServer(ledger.NewMemory(), measuresNow).GetWorkItemMeasures(context.Background(), measuresRequest(0, ""))
	if _, refused := resp.(openapi.GetWorkItemMeasures403ApplicationProblemPlusJSONResponse); err != nil || !refused {
		t.Fatalf("response = %T, %v; want 403", resp, err)
	}
}

func TestGetWorkItemMeasures_weeksOutsideOneToTwentySix_is400(t *testing.T) {
	t.Parallel()
	server := measuresServer(ledger.NewMemory(), measuresNow)
	for _, weeks := range []int{-1, 0, 27} {
		req := openapi.GetWorkItemMeasuresRequestObject{Params: openapi.GetWorkItemMeasuresParams{Weeks: &weeks}}
		resp, err := server.GetWorkItemMeasures(inArea("factory", domain.RoleApprover), req)
		if _, bad := resp.(openapi.GetWorkItemMeasures400ApplicationProblemPlusJSONResponse); err != nil || !bad {
			t.Errorf("weeks %d: response = %T, %v; want 400", weeks, resp, err)
		}
	}
	for _, weeks := range []int{1, 26} {
		measuresGet(t, server, inArea("factory", domain.RoleApprover), measuresRequest(weeks, ""))
	}
}

// The agent:read rule (#230): run:read alone names no agent, and the body is
// the one an empty window answers — nothing says there were runs.
func TestGetWorkItemMeasures_runReadWithoutAgentRead_isByteEqualToAnEmptyWindow(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "secret-agent", trailFactory, trailCard, 10)
	blind := stated(measuresServer(seed.store, measuresNow), map[domain.Permission][]domain.Scope{
		domain.PermRunRead: {trailFactory}, domain.PermCostRead: {trailFactory},
	})

	_, got := measuresGet(t, blind, context.Background(), measuresRequest(0, ""))
	_, empty := measuresGet(t, measuresServer(ledger.NewMemory(), measuresNow), inArea("factory", domain.RoleApprover),
		measuresRequest(0, ""))

	if !bytes.Equal(got, empty) {
		t.Fatalf("bodies differ:\n run:read only %s\n empty window  %s", got, empty)
	}
}

func TestGetWorkItemMeasures_costReadInOneScopeOfTheRow_leavesEveryCostOut(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-factory", "developer", trailFactory, trailCard, 10)
	seed.step("run-factory", "developer", trailFactory, domain.StepRunFinished, nil)
	seed.claim("run-cx", "developer", trailCX, trailCard, 20)
	both := []domain.Scope{trailFactory, trailCX}
	partial := stated(measuresServer(seed.store, measuresNow), map[domain.Permission][]domain.Scope{
		domain.PermRunRead: both, domain.PermAgentRead: both, domain.PermCostRead: {trailFactory},
	})

	page, raw := measuresGet(t, partial, context.Background(), measuresRequest(0, ""))

	if len(page.Rows) != 1 || page.Rows[0].Runs != 2 {
		t.Fatalf("rows = %+v, want one row of both runs", page.Rows)
	}
	if strings.Contains(string(raw), "Micros") {
		t.Fatalf("a row partly outside cost:read carries a cost: %s", raw)
	}
	full, _ := measuresGet(t, measuresServer(seed.store, measuresNow), companyAuditor(), measuresRequest(0, ""))
	if r := full.Rows[0]; r.CostOnCardsMicros == nil || *r.CostOnCardsMicros != 30 ||
		r.CostInheritedMicros == nil || r.CostOffCardMicros == nil {
		t.Fatalf("with cost:read everywhere: row = %+v, want all three costs", r)
	}
}

func TestGetWorkItemMeasures_theWindowFollowsTheInjectedClock(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "developer", trailFactory, trailCard, 10)
	ctx := inArea("factory", domain.RoleApprover)

	current, _ := measuresGet(t, measuresServer(seed.store, measuresNow), ctx, measuresRequest(2, ""))
	nextWeek := measuresNow.AddDate(0, 0, 7)
	later, _ := measuresGet(t, measuresServer(seed.store, nextWeek), ctx, measuresRequest(2, ""))
	gone, _ := measuresGet(t, measuresServer(seed.store, measuresNow.AddDate(0, 0, 21)), ctx, measuresRequest(2, ""))

	if !current.From.Equal(measuresWeek.AddDate(0, 0, -7)) || !current.To.Equal(measuresWeek.AddDate(0, 0, 7)) ||
		!current.AsOf.Equal(measuresNow) {
		t.Fatalf("window = [%s, %s) as of %s, want the two weeks ending with %s", current.From, current.To,
			current.AsOf, measuresWeek)
	}
	if len(current.Rows) != 1 || current.Rows[0].Complete || !current.Rows[0].Week.Equal(measuresWeek) {
		t.Fatalf("this week: rows = %+v, want the run's week, not complete", current.Rows)
	}
	if len(later.Rows) != 1 || !later.Rows[0].Complete {
		t.Fatalf("a week later: rows = %+v, want the same week, complete", later.Rows)
	}
	if len(gone.Rows) != 0 {
		t.Fatalf("three weeks later: rows = %+v, want the run outside a two-week window", gone.Rows)
	}
	if current.WeekStart != openapi.Monday || current.Timezone != openapi.UTC {
		t.Errorf("weekStart %q timezone %q, want monday and UTC", current.WeekStart, current.Timezone)
	}
}

// "Edited" is not recorded; lane time and caught-later live on the board.
// Said once, in every response, an empty one too — never a zero.
func TestGetWorkItemMeasures_unavailable_namesEditedOnceAndTheBoardMeasures(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "developer", trailFactory, trailCard, 10)
	for name, store := range map[string]*ledger.Memory{"seeded": seed.store, "empty": ledger.NewMemory()} {
		page, raw := measuresGet(t, measuresServer(store, measuresNow), inArea("factory", domain.RoleApprover),
			measuresRequest(0, ""))
		count := map[openapi.UnavailableMeasureMeasure]int{}
		for _, u := range page.Unavailable {
			count[u.Measure]++
		}
		if count[openapi.Edited] != 1 || count[openapi.LaneTime] != 1 || count[openapi.CaughtLater] != 1 ||
			len(page.Unavailable) != 3 {
			t.Errorf("%s: unavailable = %+v, want edited, laneTime and caughtLater once each", name, page.Unavailable)
		}
		if n := strings.Count(string(raw), `"edited"`); n != 1 {
			t.Errorf("%s: edited appears %d times in the body, want once: %s", name, n, raw)
		}
	}
}

// Counts, agent ids and instants only: nothing a step or a decision carried,
// and no reference or run id, reaches the body.
func TestGetWorkItemMeasures_carriesNoArgumentPayloadNoteOrReference(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-1", "developer", trailFactory, trailCard, 10)
	seed.step("run-1", "developer", trailFactory, domain.StepToolCalled,
		map[string]any{"tool": "github.issue_write", "args": map[string]string{"body": "SECRET-ARG-cpf-123"}})
	req := seed.step("run-1", "developer", trailFactory, domain.StepApprovalRequested, domain.ApprovalRequestedPayload{
		Tool: "github.issue_write", Rule: "SECRET-RULE", Reason: "SECRET-REASON",
		ArgsRef: "content:SECRET-ARGS-REF", ArgsDigest: "SECRET-DIGEST",
	})
	seed.step("run-1", "developer", trailFactory, domain.StepApprovalDecided,
		domain.ApprovalDecidedPayload{Approved: false, By: "ana", Note: "SECRET-NOTE", AtSeq: req.Seq})
	seed.step("run-1", "developer", trailFactory, domain.StepApprovalRequested, domain.ApprovalRequestedPayload{
		Tool: "github.issue_write", ArgsRef: "content:SECRET-ARGS-REF-2", ArgsDigest: "SECRET-DIGEST-2",
	})
	seed.step("run-1", "developer", trailFactory, domain.StepAbandoned, domain.AbandonedPayload{By: "ana", Reason: "SECRET-ABANDON"})

	page, raw := measuresGet(t, measuresServer(seed.store, measuresNow), inArea("factory", domain.RoleApprover),
		measuresRequest(0, ""))

	if p := page.Rows[0].Proposals; p.Requested != 2 || p.Refused != 1 || p.RevisedAfterRefusal != 1 || p.Abandoned != 1 {
		t.Fatalf("proposals = %+v; want the counts", p)
	}
	for _, leak := range []string{"SECRET", "issue_write", "ana", "run-1", "fuseone-agents#204", "content:"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the body carries %q: %s", leak, raw)
		}
	}
}

func TestGetWorkItemMeasures_filteredByAgent_answersThatAgentAlone(t *testing.T) {
	t.Parallel()
	seed := newTrailSeed(t)
	seed.claim("run-dev", "developer", trailFactory, trailCard, 10)
	seed.claim("run-qa", "qa", trailFactory, trailCard, 10)

	page, _ := measuresGet(t, measuresServer(seed.store, measuresNow), inArea("factory", domain.RoleApprover),
		measuresRequest(0, "qa"))

	if len(page.Rows) != 1 || page.Rows[0].AgentId != "qa" {
		t.Fatalf("rows = %+v, want qa alone", page.Rows)
	}
}
