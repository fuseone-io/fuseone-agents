package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// Opening a run is the one write on this API that reaches the real world: a
// worker picks it up and starts calling tools. Everything here is about making
// that happen exactly as often as somebody asked for it.

func triggerable(t *testing.T) *fakeDetail {
	t.Helper()
	return &fakeDetail{
		versions: []domain.AgentSummary{
			{
				ID: "triage", VersionID: "v2", Name: "Atendimento",
				Scope:  domain.Scope{Company: "acme", Area: "cx"},
				Latest: true, PublishedAt: time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC),
			},
		},
	}
}

func startRequest(key string) openapi.StartRunRequestObject {
	return openapi.StartRunRequestObject{
		AgentId: "triage",
		Params:  openapi.StartRunParams{IdempotencyKey: key},
		Body:    &openapi.StartRunJSONRequestBody{},
	}
}

func TestStartRun_opensARunPinnedToTheNewestVersion(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()

	resp, err := NewServer(store, "test").WithAgents(triggerable(t)).
		StartRun(inArea("cx", domain.RoleAuthor), startRequest("intent-0001"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	opened, ok := resp.(openapi.StartRun201JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the opened run", resp)
	}
	// Pinned, so publishing a new version never changes what this run does.
	if opened.VersionId != "v2" {
		t.Errorf("version = %q, want the newest published", opened.VersionId)
	}
	if opened.Phase != openapi.PhaseUnstarted && opened.Phase != openapi.PhaseRunning {
		t.Errorf("phase = %q, want a run waiting for a worker", opened.Phase)
	}
}

func TestStartRun_sameKeyTwice_returnsTheFirstRunRatherThanOpeningASecond(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	server := NewServer(store, "test").WithAgents(triggerable(t))
	ctx := inArea("cx", domain.RoleAuthor)

	first, err := server.StartRun(ctx, startRequest("intent-0001"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	opened := first.(openapi.StartRun201JSONResponse)

	// A double-clicked button, or a retry after a timeout. Either way the
	// caller meant one run, and a second would be a real effect twice.
	second, err := server.StartRun(ctx, startRequest("intent-0001"))
	if err != nil {
		t.Fatalf("StartRun again: %v", err)
	}
	again, ok := second.(openapi.StartRun200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the run the key already opened", second)
	}
	if again.RunId != opened.RunId {
		t.Errorf("second call opened %q, want %q", again.RunId, opened.RunId)
	}

	runs, err := store.Runs(context.Background())
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want 1", len(runs))
	}
}

func TestStartRun_withoutThePermission_isRefused(t *testing.T) {
	t.Parallel()

	// Reading what an agent did and making it do something again are different
	// authorities. An auditor holds the first and must not hold the second.
	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).
		StartRun(inArea("cx", domain.RoleAuditor), startRequest("intent-0002"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, refused := resp.(openapi.StartRun403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestStartRun_inAnotherArea_readsAsAbsent(t *testing.T) {
	t.Parallel()

	resp, err := NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).
		StartRun(inArea("marketing", domain.RoleAuthor), startRequest("intent-0003"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, absent := resp.(openapi.StartRun404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestStartRun_withInput_storesItOutsideTheLedger(t *testing.T) {
	t.Parallel()
	store, content := ledger.NewMemory(), engine.NewMemoryContent()

	req := startRequest("intent-0004")
	input := "cliente pediu segunda via do boleto"
	req.Body = &openapi.StartRunJSONRequestBody{Input: &input}

	resp, err := NewServer(store, "test").WithAgents(triggerable(t)).WithContent(content).
		StartRun(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	opened := resp.(openapi.StartRun201JSONResponse)

	steps, err := store.Read(context.Background(), domain.RunID(opened.RunId), domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	ref, _ := referenceOf(steps[0])
	if ref == "" {
		t.Fatal("the run's input is not referenced from its opening step")
	}

	// In the content store, never inlined: what a run is about routinely
	// carries personal data, and the ledger is kept for years.
	stored, err := content.Get(context.Background(), ref)
	if err != nil || string(stored) != input {
		t.Errorf("stored input = %q (%v), want it kept outside the ledger", stored, err)
	}
}
