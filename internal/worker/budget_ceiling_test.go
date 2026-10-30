package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

// The production case: an agent whose own ceiling equals its scope's daily
// ceiling plans under the scope's headroom, which falls with every call any
// agent in the scope makes. Written into the cached prefix, that figure made
// every call a cache miss. This crosses the claim, the narrowing, the runner
// and the real adapter, with the scope's spend moving between two turns.
func TestTurn_scopeSpendMovingBetweenTurns_sendsTheSameSystemPrompt(t *testing.T) {
	t.Parallel()
	systemAfter := func(spent int64) string {
		planner, sent := capturingPlanner(t)
		s := newSetup(t, Config{MaxAttempts: 3, BaseBackoff: time.Second},
			&flakyPlanner{failures: 1, delegate: planner}, nil)
		s.worker.WithCeilings(staticCeilings{
			ceiling: domain.Budget{Micros: 1_000_000},
			period:  domain.PeriodDaily,
			spent:   domain.Consumption{Micros: spent},
		})
		openRun(t, s.store)
		if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
			t.Fatalf("turn: %v", err)
		}
		return systemText(t, sent())
	}

	before, after := systemAfter(120_000), systemAfter(340_000)
	if before != after {
		t.Errorf("the scope's spend moved the system prompt:\n%s\n---\n%s", before, after)
	}
	if !strings.Contains(before, "Budget ceiling for this run: 1.000000") {
		t.Errorf("system does not show the agent's own ceiling:\n%s", before)
	}
}

// capturingPlanner is the real Anthropic adapter behind a server that keeps
// the last request body and answers with a call to the setup's one tool.
func capturingPlanner(t *testing.T) (engine.Planner, func() []byte) {
	t.Helper()
	var last atomic.Pointer[[]byte]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		last.Store(&raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"m-test",
		  "content":[{"type":"tool_use","id":"tu_1","name":"crm__lookup","input":{"email":"a@b.com"}}],
		  "stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}}`)
	}))
	t.Cleanup(srv.Close)

	reg := model.NewRegistry(nil)
	if err := reg.Register(model.Provider{
		Name: "under-test", Kind: model.KindAnthropic, BaseURL: srv.URL, APIKey: "test-key",
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p, err := reg.Planner("under-test", model.Config{Model: "m-test"}, lookupSchema{})
	if err != nil {
		t.Fatalf("Planner: %v", err)
	}
	return p, func() []byte {
		body := last.Load()
		if body == nil {
			t.Fatal("the provider was never called")
		}
		return *body
	}
}

func systemText(t *testing.T, body []byte) string {
	t.Helper()
	var req struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	var b strings.Builder
	for _, block := range req.System {
		b.WriteString(block.Text)
	}
	return b.String()
}
