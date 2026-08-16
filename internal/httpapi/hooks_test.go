package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// The webhook is the one door on this platform that opens for something other
// than a person. These are the properties that keep it from opening for
// anybody who guessed a path.

type fakeHooks struct {
	armed  map[string]string
	agents map[string]domain.AgentID
}

func (f *fakeHooks) Verify(_ context.Context, path, secret string) (bool, error) {
	want, declared := f.armed[path]
	if !declared {
		return false, trigger.ErrNoHook
	}
	if want == "" {
		return false, trigger.ErrNotArmed
	}
	return want == secret, nil
}

func (f *fakeHooks) Find(_ context.Context, path string) (trigger.Hook, error) {
	agent, ok := f.agents[path]
	if !ok {
		return trigger.Hook{}, trigger.ErrNoHook
	}
	return trigger.Hook{Path: path, Agent: agent, Armed: f.armed[path] != ""}, nil
}

func (f *fakeHooks) Rotate(context.Context, string, domain.UserID, time.Time) (string, error) {
	return "", nil
}
func (f *fakeHooks) Sync(context.Context, domain.AgentID, domain.Scope, []string) error { return nil }
func (f *fakeHooks) ForAgent(context.Context, domain.AgentID) ([]trigger.Hook, error) {
	return nil, nil
}

type hookRegistry struct{}

func (hookRegistry) Versions(context.Context, domain.AgentID) ([]domain.AgentSummary, error) {
	return []domain.AgentSummary{{
		ID: "triage", VersionID: "v2", Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}, nil
}

type hookClock struct{}

func (hookClock) Now() time.Time { return time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC) }

func hookServer(t *testing.T) (*http.ServeMux, *ledger.Memory) {
	t.Helper()
	store := ledger.NewMemory()
	hooks := &fakeHooks{
		armed:  map[string]string{"crm/ticket": "s3cret", "erp/order": ""},
		agents: map[string]domain.AgentID{"crm/ticket": "triage", "erp/order": "triage"},
	}
	mux := http.NewServeMux()
	NewHooks(hooks, trigger.NewOpener(store, hookRegistry{}, hookClock{}),
		slog.New(slog.NewTextHandler(io.Discard, nil))).Mount(mux)
	return mux, store
}

func post(t *testing.T, mux *http.ServeMux, path, secret, delivery, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/hooks/"+path, strings.NewReader(body))
	if secret != "" {
		req.Header.Set("X-FuseOne-Secret", secret)
	}
	if delivery != "" {
		req.Header.Set("Idempotency-Key", delivery)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHook_withTheSecret_opensARun(t *testing.T) {
	t.Parallel()
	mux, store := hookServer(t)

	rec := post(t, mux, "crm/ticket", "s3cret", "delivery-1", `{"ticket":"OPS-8841"}`)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%s)", rec.Code, rec.Body)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want 1", len(runs))
	}
}

func TestHook_redelivery_namesTheSameRunRatherThanOpeningASecond(t *testing.T) {
	t.Parallel()
	mux, store := hookServer(t)

	first := post(t, mux, "crm/ticket", "s3cret", "delivery-1", `{"ticket":"OPS-8841"}`)
	second := post(t, mux, "crm/ticket", "s3cret", "delivery-1", `{"ticket":"OPS-8841"}`)

	if second.Code != http.StatusOK {
		t.Fatalf("redelivery status = %d, want 200", second.Code)
	}
	if runOf(t, first) != runOf(t, second) {
		t.Error("a redelivery named a different run")
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want 1 — every sender redelivers", len(runs))
	}
}

func TestHook_wrongSecret_isRejectedAndOpensNothing(t *testing.T) {
	t.Parallel()
	mux, store := hookServer(t)

	rec := post(t, mux, "crm/ticket", "wrong", "delivery-1", `{}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 0 {
		t.Error("a rejected call opened a run")
	}
}

func TestHook_pathWithNoSecretYet_isClosed(t *testing.T) {
	t.Parallel()
	mux, _ := hookServer(t)

	// Declared but never rotated. Answering it would leave an agent reachable
	// by anybody who read the specification.
	rec := post(t, mux, "erp/order", "", "delivery-1", `{}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHook_unknownPath_answersLikeAWrongSecret(t *testing.T) {
	t.Parallel()
	mux, _ := hookServer(t)

	// A caller probing paths must not learn which ones exist: that is a map of
	// the installation, drawn one request at a time.
	unknown := post(t, mux, "made/up", "s3cret", "delivery-1", `{}`)
	wrong := post(t, mux, "crm/ticket", "wrong", "delivery-1", `{}`)

	if unknown.Code != wrong.Code {
		t.Errorf("unknown path answered %d and a wrong secret %d; they must not differ",
			unknown.Code, wrong.Code)
	}
}

func TestHook_withoutADeliveryIdentifier_isRefusedWithAnExplanation(t *testing.T) {
	t.Parallel()
	mux, store := hookServer(t)

	rec := post(t, mux, "crm/ticket", "s3cret", "", `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	// The sender's integrator is the only person who can fix this, and they
	// are reading this response.
	if !strings.Contains(rec.Body.String(), "Idempotency-Key") {
		t.Errorf("the refusal does not name the header: %s", rec.Body)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 0 {
		t.Error("a run opened without a delivery identifier")
	}
}

func TestHook_getRequest_isNotAWayToTriggerAnAgent(t *testing.T) {
	t.Parallel()
	mux, _ := hookServer(t)

	req := httptest.NewRequest(http.MethodGet, "/hooks/crm/ticket", nil)
	req.Header.Set("X-FuseOne-Secret", "s3cret")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusAccepted {
		t.Error("a GET opened a run; anything that follows links could trigger an agent")
	}
}

func runOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body.RunID
}

/*
An agent that is not running is a state, not a failure of the platform.

Paused, stopped by a switch, or still a draft: the door answers 500 "Could not
open the run" with nothing in the detail. The person reading that is the
sender's integrator, and what they conclude is that this platform is broken —
while on our side it is logged as an error and pages somebody about a decision
an operator made on purpose.

The secret was correct, so there is nothing left to protect: a caller who
authenticated is entitled to know why their delivery did nothing. That is not
the probing this handler guards against, which is answered identically whether
a path exists or not, before any of this.
*/
func TestHook_agentIsPaused_saysSoRatherThanFailing(t *testing.T) {
	t.Parallel()
	mux, store := hookServerPaused(t)

	rec := post(t, mux, "crm/ticket", "s3cret", "d-1", `{"id":1}`)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 — the agent is paused, nothing failed", rec.Code)
	}
	if runs, _ := store.Runs(context.Background()); len(runs) != 0 {
		t.Error("a run was opened for a paused agent")
	}
}

// Named, because "could not open the run" sends the integrator to us and the
// answer is a switch on our side that they cannot see.
func TestHook_agentIsPaused_theReasonReachesTheSender(t *testing.T) {
	t.Parallel()
	mux, _ := hookServerPaused(t)

	rec := post(t, mux, "crm/ticket", "s3cret", "d-1", `{"id":1}`)

	if !strings.Contains(rec.Body.String(), "paused") {
		t.Errorf("body = %s, want it to name the state", rec.Body.String())
	}
}

func hookServerPaused(t *testing.T) (*http.ServeMux, *ledger.Memory) {
	t.Helper()
	store := ledger.NewMemory()
	hooks := &fakeHooks{
		armed:  map[string]string{"crm/ticket": "s3cret"},
		agents: map[string]domain.AgentID{"crm/ticket": "triage"},
	}
	mux := http.NewServeMux()
	opener := trigger.NewOpener(store, hookRegistry{}, hookClock{}).WithPauses(allPaused{})
	NewHooks(hooks, opener, slog.New(slog.NewTextHandler(io.Discard, nil))).Mount(mux)
	return mux, store
}

type allPaused struct{}

func (allPaused) IsPaused(context.Context, domain.AgentID) (bool, error) { return true, nil }

/*
A body somebody outside sent arrives tainted.

The secret proves who sent it and says nothing about what is inside. Until the
label was sealed on the opening step, a run started by a webhook met the taint
check only after an untrusted tool had answered — so an agent that read the
body and wrote straight from it was never escalated, which is the one case the
check exists for.
*/
func TestHook_theBody_opensTheRunTainted(t *testing.T) {
	t.Parallel()
	mux, store := hookServer(t)

	if rec := post(t, mux, "crm/ticket", "s3cret", "d-taint", `{"x":1}`); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want the run opened", rec.Code)
	}

	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Fatalf("runs = %v, want one", runs)
	}
	steps, _ := store.Read(context.Background(), runs[0], domain.FirstSeq)
	if !steps[0].Labels.Has(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the body marked untrusted", steps[0].Labels)
	}
}
