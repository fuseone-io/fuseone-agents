package model_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

// An agent whose steps are one query each moves to a new step on most turns,
// and the step note sits in the system prompt. With the only system breakpoint
// after that note, every step change re-wrote the agent's whole instructions
// and tool list to cache at the write rate. The instructions must be cached on
// their own, ahead of anything that names the step.
func TestAnthropic_stepChange_leavesTheInstructionsPrefixIdentical(t *testing.T) {
	t.Parallel()
	const instructions = "Você varre os logs de integração a cada hora."

	prefixAt := func(step string) string {
		c := serve(t, anthropicToolUse)
		in := input()
		in.Step = step
		_, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
			SystemPrompt: instructions, CachePrefixTTL: "1h",
		}).Plan(context.Background(), in)
		if err != nil {
			t.Fatalf("Plan at %q: %v", step, err)
		}
		return firstSystemBreakpointPrefix(t, c.last())
	}

	first, second := prefixAt("Run V1"), prefixAt("Run V2")
	if !strings.Contains(first, instructions) {
		t.Fatalf("the first system breakpoint does not cover the instructions:\n%s", first)
	}
	if first != second {
		t.Errorf("a step change altered the prefix up to the first breakpoint:\n%s\n---\n%s", first, second)
	}
}

// firstSystemBreakpointPrefix is the tools and system bytes a provider keys
// the earliest system cache entry on: tools first, then system blocks up to and
// including the first one carrying cache_control.
func firstSystemBreakpointPrefix(t *testing.T, body map[string]any) string {
	t.Helper()
	tools, err := json.Marshal(body["tools"])
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	var b strings.Builder
	b.Write(tools)
	blocks, _ := body["system"].([]any)
	for _, block := range blocks {
		m, _ := block.(map[string]any)
		raw, _ := json.Marshal(m)
		b.Write(raw)
		if m["cache_control"] != nil {
			return b.String()
		}
	}
	t.Fatal("no cache breakpoint on the system prompt")
	return ""
}

// The provider refuses a request with more than four cache breakpoints, so a
// split prefix must not push a full request past the limit.
func TestAnthropic_fullRequest_staysWithinFourCacheBreakpoints(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)
	in := input()
	in.Step = "Run V1"
	in.Transcript = append(in.Transcript,
		engine.Turn{Kind: engine.TurnToolUse, CallID: "call_1", Tool: "crm.lookup", Args: []byte(`{"email":"a@b.com"}`)},
		engine.Turn{Kind: engine.TurnToolResult, CallID: "call_1", Tool: "crm.lookup", Content: []byte(`{"id":42}`)},
	)
	_, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{CachePrefixTTL: "1h"}).
		Plan(context.Background(), in)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	raw, err := json.Marshal(c.last())
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if n := strings.Count(string(raw), `"cache_control"`); n == 0 || n > 4 {
		t.Errorf("request carries %d cache breakpoints, want between 1 and 4", n)
	}
}
