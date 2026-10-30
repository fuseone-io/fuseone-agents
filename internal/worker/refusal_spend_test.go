package worker

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

// A provider bills a refusal: it read the whole prompt before declining. Both
// adapters used to hand the priced proposal back beside the refusal error, and
// the runner drops a proposal whenever there is an error — so the refusal's
// tokens never reached the ledger, the budget or FinOps (#132). This crosses
// the real planner, the runner and the ledger, for every adapter that can
// refuse, and asks for the exact figures the provider billed.
func TestTurn_aBilledRefusal_landsOnTheLedgerExactly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		kind     model.Kind
		response string
	}{
		{
			name: "anthropic", kind: model.KindAnthropic,
			response: `{"id":"m","type":"message","role":"assistant","model":"m-test",
			  "content":[],"stop_reason":"refusal",
			  "usage":{"input_tokens":1200,"output_tokens":30}}`,
		},
		{
			name: "openai compatible", kind: model.KindOpenAICompatible,
			response: `{"model":"m-test","choices":[{"finish_reason":"content_filter",
			  "message":{"role":"assistant","content":""}}],
			  "usage":{"prompt_tokens":1200,"completion_tokens":30}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			planner := refusingPlanner(t, tc.kind, tc.response)
			s := newSetup(t, Config{MaxAttempts: 3, BaseBackoff: time.Second},
				&flakyPlanner{failures: 1, delegate: planner}, nil)
			openRun(t, s.store)

			if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("turn: %v", err)
			}

			runs, err := s.store.Runs(context.Background())
			if err != nil || len(runs) != 1 {
				t.Fatalf("Runs = %v, %v", runs, err)
			}
			steps, err := s.store.Read(context.Background(), runs[0], domain.FirstSeq)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			var spent []domain.Cost
			for _, step := range steps {
				if step.Kind == domain.StepPlanned {
					spent = append(spent, step.Cost)
				}
			}
			// 1200 in at 2 USD/M and 30 out at 10 USD/M: 2400 + 300 micros.
			want := domain.Cost{InputTokens: 1200, OutputTokens: 30, Micros: 2_700}
			if len(spent) != 1 || spent[0].InputTokens != want.InputTokens ||
				spent[0].OutputTokens != want.OutputTokens || spent[0].Micros != want.Micros {
				t.Fatalf("planned spend = %+v, want exactly one step carrying %+v", spent, want)
			}
		})
	}
}

// refusingPlanner is the real adapter of kind, priced, behind a server that
// answers every call with response.
func refusingPlanner(t *testing.T, kind model.Kind, response string) engine.Planner {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)

	reg := model.NewRegistry(nil)
	if err := reg.Register(model.Provider{
		Name: "under-test", Kind: kind, BaseURL: srv.URL, APIKey: "test-key",
		Prices: map[string]model.Prices{
			"m-test": {InputMicros: 2_000_000, OutputMicros: 10_000_000},
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p, err := reg.Planner("under-test", model.Config{Model: "m-test"}, lookupSchema{})
	if err != nil {
		t.Fatalf("Planner: %v", err)
	}
	return p
}

// lookupSchema describes the one tool the setup's pack offers, so the request
// the adapter assembles is the one an installation would send.
type lookupSchema struct{}

func (lookupSchema) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if id != "crm.lookup" {
		return "", "", nil, false
	}
	return "crm.lookup", "Look a customer up by email.",
		map[string]any{"email": map[string]any{"type": "string"}}, true
}
