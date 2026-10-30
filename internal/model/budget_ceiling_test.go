package model_test

import (
	"context"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

// The budget a run plans under is the agent's ceiling narrowed by its scope's
// headroom, recomputed on every claim — and the headroom falls with every call
// any agent in the scope makes. An agent whose own ceiling did not bind wrote
// that falling number into its cached prefix, so no two calls shared a prefix
// and the whole prompt was re-written to cache on every turn. The prefix shows
// the agent's own ceiling, which only a new version changes.
func TestPlan_scopeHeadroomMovingBetweenCalls_leavesTheSystemPromptIdentical(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		kind     model.Kind
		response string
	}{
		{"anthropic", model.KindAnthropic, anthropicToolUse},
		{"openai compatible", model.KindOpenAICompatible, openAIToolCall},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			systemWith := func(headroom int64) string {
				c := serve(t, tc.response)
				in := input()
				in.Ceiling = domain.Budget{Micros: 1_000_000_000}
				in.Budget = domain.Budget{Micros: headroom}
				if _, err := plannerFor(t, tc.kind, c.server.URL, model.Config{}).
					Plan(context.Background(), in); err != nil {
					t.Fatalf("Plan: %v", err)
				}
				return systemOf(t, tc.kind, c.last())
			}

			before, after := systemWith(812_345_678), systemWith(803_219_004)
			if before != after {
				t.Errorf("the scope's spend moved the system prompt:\n%s\n---\n%s", before, after)
			}
			if want := "Agent-configured budget ceiling: 1000.000000"; !strings.Contains(before, want) {
				t.Errorf("system does not show the agent's own ceiling %q:\n%s", want, before)
			}
		})
	}
}

func systemOf(t *testing.T, kind model.Kind, body map[string]any) string {
	t.Helper()
	if kind == model.KindAnthropic {
		return anthropicSystemText(body)
	}
	messages, _ := body["messages"].([]any)
	if len(messages) == 0 {
		t.Fatal("no messages on the wire")
	}
	first, _ := messages[0].(map[string]any)
	return asString(first["content"])
}

// The stable line cannot be the run's limit: the scope may enforce less, and
// the OpenAI-compatible wire carries no remaining-budget note to correct it.
// A model told "1000" for a run its scope caps at 10 plans work the Gate will
// stop, so the line names whose ceiling it is and that a scope may bind lower.
func TestPlan_scopeBindsBelowTheAgentCeiling_noWireClaimsTheAgentCeilingAsTheRunLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		kind     model.Kind
		response string
	}{
		{"anthropic", model.KindAnthropic, anthropicToolUse},
		{"openai compatible", model.KindOpenAICompatible, openAIToolCall},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, tc.response)
			in := input()
			in.Ceiling = domain.Budget{Micros: 1_000_000_000}
			in.Budget = domain.Budget{Micros: 10_000_000}
			in.Remaining = domain.Consumption{Micros: 10_000_000}
			if _, err := plannerFor(t, tc.kind, c.server.URL, model.Config{}).
				Plan(context.Background(), in); err != nil {
				t.Fatalf("Plan: %v", err)
			}
			system := systemOf(t, tc.kind, c.last())
			if strings.Contains(system, "ceiling for this run: 1000") {
				t.Errorf("the system states the agent's ceiling as this run's limit:\n%s", system)
			}
			for _, want := range []string{
				"Agent-configured budget ceiling: 1000.000000", "Scope limits may enforce a lower amount",
			} {
				if !strings.Contains(system, want) {
					t.Errorf("system lacks %q:\n%s", want, system)
				}
			}
		})
	}
}
