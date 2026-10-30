package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

// The prefix digests answer one question — which block changed between two
// calls — and only if they reach the ledger, where that question is asked.
// This crosses the real adapter, the runner and the ledger for both wires.
func TestTurn_plannedStep_recordsThePrefixDigestsTheAdapterSent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		kind     model.Kind
		response string
	}{
		{
			name: "anthropic", kind: model.KindAnthropic,
			response: `{"id":"m","type":"message","role":"assistant","model":"m-test",
			  "content":[{"type":"tool_use","id":"tu_1","name":"crm__lookup","input":{"email":"a@b.com"}}],
			  "stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":10}}`,
		},
		{
			name: "openai compatible", kind: model.KindOpenAICompatible,
			response: `{"model":"m-test","choices":[{"finish_reason":"tool_calls",
			  "message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function",
			  "function":{"name":"crm__lookup","arguments":"{\"email\":\"a@b.com\"}"}}]}}],
			  "usage":{"prompt_tokens":100,"completion_tokens":10}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newSetup(t, Config{MaxAttempts: 3, BaseBackoff: time.Second},
				&flakyPlanner{failures: 1, delegate: refusingPlanner(t, tc.kind, tc.response)}, nil)
			openRun(t, s.store)

			if _, err := s.worker.turn(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("turn: %v", err)
			}

			p := plannedPayload(t, s)
			if p.Prompt == nil || p.Prompt.Digests == nil {
				t.Fatalf("planned prompt = %+v, want the digests the adapter computed", p.Prompt)
			}
			if d := p.Prompt.Digests; d.Tools == "" || len(d.System) == 0 {
				t.Errorf("digests = %+v, want the tools and the system blocks", d)
			}
		})
	}
}

func plannedPayload(t *testing.T, s *setup) domain.PlannedPayload {
	t.Helper()
	runs, err := s.store.Runs(context.Background())
	if err != nil || len(runs) != 1 {
		t.Fatalf("Runs = %v, %v", runs, err)
	}
	steps, err := s.store.Read(context.Background(), runs[0], domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, step := range steps {
		if step.Kind != domain.StepPlanned {
			continue
		}
		var p domain.PlannedPayload
		if err := json.Unmarshal(step.Payload, &p); err != nil {
			t.Fatalf("decode planned: %v", err)
		}
		return p
	}
	t.Fatal("no planned step on the ledger")
	return domain.PlannedPayload{}
}
