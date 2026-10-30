package model_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

// An agent that never read its cache could not be diagnosed: the ledger kept
// each prompt block's size, and two blocks of equal size with different bytes
// read the same. A fingerprint of every cacheable block, as sent, names the
// block that moved between two calls.
func TestPlan_prefixDigests_nameTheBlockThatChanged(t *testing.T) {
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
			digestsAt := func(step string) domain.PromptDigests {
				c := serve(t, tc.response)
				in := input()
				in.Step = step
				p, err := plannerFor(t, tc.kind, c.server.URL, model.Config{
					SystemPrompt: "Você varre os logs de integração.",
				}).Plan(context.Background(), in)
				if err != nil {
					t.Fatalf("Plan at %q: %v", step, err)
				}
				if p.Prompt.Digests == nil {
					t.Fatal("the planned prompt carries no digests")
				}
				return *p.Prompt.Digests
			}

			first, again, moved := digestsAt("Run V1"), digestsAt("Run V1"), digestsAt("Run V2")
			if first.Tools == "" || len(first.System) == 0 {
				t.Fatalf("digests = %+v, want the tools and every system block", first)
			}
			if !equalDigests(first, again) {
				t.Errorf("the same request digested differently: %+v vs %+v", first, again)
			}
			if first.Tools != moved.Tools {
				t.Errorf("a step change moved the tools digest: %s vs %s", first.Tools, moved.Tools)
			}
			if equalDigests(first, moved) {
				t.Errorf("a step change left every digest equal: %+v", first)
			}
		})
	}
}

func equalDigests(a, b domain.PromptDigests) bool {
	if a.Tools != b.Tools || len(a.System) != len(b.System) {
		return false
	}
	for i := range a.System {
		if a.System[i] != b.System[i] {
			return false
		}
	}
	return true
}
