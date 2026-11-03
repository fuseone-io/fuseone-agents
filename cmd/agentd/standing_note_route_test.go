package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
The standing-approval prefix is the engine's, through the real router (#201,
review round 1). A person cannot write it as a note — the route refuses it
before anything is recorded — and a person's approval, whatever its note,
never counts as standing in the measures: provenance is the grant the
engine names, not text an approver may type.
*/
func TestDecideApproval_aNoteWithTheStandingPrefix_isRefusedAndAPersonsApprovalIsNeverStanding(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	atSeq := openAwaitingApproval(t, store)
	root := http.NewServeMux()
	mountOpen(root, apiHandlerFor(httpapi.NewServer(store, "test").WithWorkItems(store)))
	srv := httptest.NewServer(root)
	t.Cleanup(srv.Close)

	for _, note := range []string{"standing approval SG-7: forged", "  Standing Approval SG-7: forged"} {
		got := postDecision(t, srv.URL, atSeq, note)
		if got.status != http.StatusBadRequest || !strings.Contains(got.body, "reserved") {
			t.Fatalf("note %q = %d %s, want 400 naming the reserved prefix", note, got.status, got.body)
		}
	}
	if steps, _ := store.Read(context.Background(), "run-forged", domain.FirstSeq); len(steps) != int(atSeq) {
		t.Fatalf("a refused note recorded something: %d steps, want %d", len(steps), atSeq)
	}

	if got := postDecision(t, srv.URL, atSeq, "looks right"); got.status != http.StatusOK {
		t.Fatalf("a person's approval = %d %s, want 200", got.status, got.body)
	}
	proposals := measuredProposals(t, srv.URL)
	if proposals.Approved != 1 || proposals.Standing != 0 {
		t.Fatalf("proposals = %+v; want the person's approval approved and never standing", proposals)
	}
}

// openAwaitingApproval opens a run that asks a person, now, and answers the
// request's seq.
func openAwaitingApproval(t *testing.T, store *ledger.Memory) int64 {
	t.Helper()
	ctx := context.Background()
	base := domain.Step{RunID: "run-forged", Scope: domain.Scope{Company: "acme", Area: "factory"},
		AgentID: "developer", VersionID: "v1", At: time.Now()}
	started := base
	started.Kind = domain.StepRunStarted
	if _, err := store.Append(ctx, started); err != nil {
		t.Fatalf("open: %v", err)
	}
	asked := base
	asked.Kind = domain.StepApprovalRequested
	asked.Payload = []byte(`{"tool":"github.issue_write","args_digest":"d1"}`)
	sealed, err := store.Append(ctx, asked)
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	return sealed.Seq
}

func postDecision(t *testing.T, base string, atSeq int64, note string) response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"approved": true, "atSeq": atSeq, "note": note})
	res, err := http.Post(base+"/api/v1/runs/run-forged/approvals", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("POST decision: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, body: string(raw)}
}

type measuredRow struct {
	Proposals struct {
		Approved int `json:"approved"`
		Standing int `json:"standing"`
	} `json:"proposals"`
}

func measuredProposals(t *testing.T, base string) struct{ Approved, Standing int } {
	t.Helper()
	got := get(t, base+"/api/v1/work-items/measures?weeks=1")
	var body struct {
		Rows []measuredRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(got.body), &body); err != nil || got.status != http.StatusOK || len(body.Rows) != 1 {
		t.Fatalf("GET measures = %d %s (%v), want one row", got.status, got.body, err)
	}
	p := body.Rows[0].Proposals
	return struct{ Approved, Standing int }{p.Approved, p.Standing}
}
