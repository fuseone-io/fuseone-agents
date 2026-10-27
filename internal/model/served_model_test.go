package model_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

func TestPlanner_aGatewaySubstitution_isRecordedAndPricedAsServed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		kind     model.Kind
		response string
	}{
		{
			name: "anthropic", kind: model.KindAnthropic,
			response: `{"id":"m","type":"message","role":"assistant","model":"served-expensive","content":[{"type":"tool_use","id":"t","name":"crm.lookup","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":1000000}}`,
		},
		{
			name: "openai-compatible", kind: model.KindOpenAICompatible,
			response: `{"model":"served-expensive","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"t","type":"function","function":{"name":"crm.lookup","arguments":"{}"}}]}}],"usage":{"prompt_tokens":1000000}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, tc.response)
			got, err := plannerForPrices(t, tc.kind, c.server.URL, model.Config{Model: "requested-cheap"}, map[string]model.Prices{
				"requested-cheap":  {InputMicros: 2_000_000},
				"served-expensive": {InputMicros: 7_000_000},
			}).Plan(context.Background(), input())
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if got.Model != "requested-cheap" || got.ServedModel != "served-expensive" {
				t.Fatalf("models = requested %q / served %q", got.Model, got.ServedModel)
			}
			if got.Cost.Micros != 7_000_000 || got.Price.Status != "configured" {
				t.Fatalf("cost = %d (%s), want served rate 7000000", got.Cost.Micros, got.Price.Status)
			}
		})
	}
}

func TestPlanner_anUnpricedServedModel_fallsBackToTheRequestedRate(t *testing.T) {
	t.Parallel()
	const response = `{"id":"m","type":"message","role":"assistant","model":"served-unpriced","content":[{"type":"tool_use","id":"t","name":"crm.lookup","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":1000000}}`
	c := serve(t, response)
	got, err := plannerForPrices(t, model.KindAnthropic, c.server.URL, model.Config{Model: "requested-priced"}, map[string]model.Prices{
		"requested-priced": {InputMicros: 3_000_000},
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.ServedModel != "served-unpriced" || got.Cost.Micros != 3_000_000 {
		t.Fatalf("served = %q cost = %d, want served-unpriced at requested rate", got.ServedModel, got.Cost.Micros)
	}
}

func TestPlanner_theRequestedModelBeingServed_recordsNoDivergence(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)
	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5"}).Plan(context.Background(), engine.PlanInput{Tools: input().Tools})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.ServedModel != "" {
		t.Fatalf("ServedModel = %q, want omitted when identical", got.ServedModel)
	}
}
