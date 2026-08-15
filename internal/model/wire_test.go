package model_test

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

/*
A tool identifier is `server.tool`, and no provider accepts the dot.

Both wire formats bound a function name to `[A-Za-z0-9_-]`, so every agent
holding any tool at all was refused with a 400 before the model ever read the
prompt — the run retried five times and parked. It was invisible in every test
because a stub answers whatever it is sent.

So the name on the wire is a rendering, like the chips in the editor, and what
the Gate rules on is the identifier the catalogue issued. The two directions
are one map, built per request from the pack itself, because guessing the
inverse of an encoding is how a call reaches a tool nobody authorised.
*/

// accepted is the pattern both providers publish for a function name.
var accepted = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func TestAnthropic_offersANameTheProviderAccepts(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)
	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5"})

	if _, err := p.Plan(context.Background(), input()); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got := firstToolName(t, c.body, "tools"); !accepted.MatchString(got) {
		t.Errorf("offered %q, which the provider refuses", got)
	}
}

func TestOpenAICompatible_offersANameTheProviderAccepts(t *testing.T) {
	t.Parallel()
	c := serve(t, openAIToolCall)
	p := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{Model: "gpt-test"})

	if _, err := p.Plan(context.Background(), input()); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	tools, _ := c.body["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal("no tools were offered")
	}
	fn, _ := tools[0].(map[string]any)["function"].(map[string]any)
	if got, _ := fn["name"].(string); !accepted.MatchString(got) {
		t.Errorf("offered %q, which the provider refuses", got)
	}
}

// What comes back is the identifier the catalogue issued, whatever the wire
// called it. The Gate rules on the pack, and a name it does not recognise is a
// refusal for a call the model was in fact allowed to make.
func TestAnthropic_proposalCarriesTheCatalogueIdentifier(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUseWireName)
	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5"})

	got, err := p.Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.Tool != domain.ToolID("crm.lookup") {
		t.Errorf("Tool = %q, want the catalogue's crm.lookup", got.Tool)
	}
}

func TestOpenAICompatible_proposalCarriesTheCatalogueIdentifier(t *testing.T) {
	t.Parallel()
	c := serve(t, openAIToolCallWireName)
	p := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{Model: "gpt-test"})

	got, err := p.Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.Tool != domain.ToolID("crm.lookup") {
		t.Errorf("Tool = %q, want the catalogue's crm.lookup", got.Tool)
	}
}

// A name nobody offered stays exactly as the model said it. The trail records
// what was proposed rather than the nearest thing it resembles, and the Gate
// refuses it for not being in the pack — which is the correct answer.
func TestAnthropic_aNameNobodyOffered_isRecordedAsItCame(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUseUnknown)
	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5"})

	got, err := p.Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.Tool != domain.ToolID("erp__refund") {
		t.Errorf("Tool = %q, want what the model actually said", got.Tool)
	}
}

const anthropicToolUseWireName = `{
  "id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5",
  "content":[{"type":"tool_use","id":"toolu_1","name":"crm__lookup","input":{"email":"a@b.com"}}],
  "stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}
}`

const anthropicToolUseUnknown = `{
  "id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5",
  "content":[{"type":"tool_use","id":"toolu_1","name":"erp__refund","input":{}}],
  "stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}
}`

const openAIToolCallWireName = `{
  "id":"chat_1","choices":[{"message":{"role":"assistant","tool_calls":[
    {"id":"call_1","type":"function","function":{"name":"crm__lookup","arguments":"{\"email\":\"a@b.com\"}"}}]},
    "finish_reason":"tool_calls"}],
  "usage":{"prompt_tokens":10,"completion_tokens":5}
}`

func firstToolName(t *testing.T, body map[string]any, field string) string {
	t.Helper()
	raw, err := json.Marshal(body[field])
	if err != nil {
		t.Fatalf("re-encode %s: %v", field, err)
	}
	var tools []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tools); err != nil || len(tools) == 0 {
		t.Fatalf("no tools were offered: %v", err)
	}
	return tools[0].Name
}

/*
The transcript replays past calls, and it names them too.

A run that got through its first turn would be refused on its second: the
assistant turn carries the tool it called, and sent as the catalogue's
identifier that name is the same 400 one turn later. It is the harder half to
notice, because the failure moves with the transcript rather than with the
pack.
*/
func TestAnthropic_replayedCall_isNamedTheWayItWasOffered(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUseWireName)
	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5"})

	if _, err := p.Plan(context.Background(), withACallAlreadyMade()); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got := replayedName(t, c.body); !accepted.MatchString(got) {
		t.Errorf("replayed as %q, which the provider refuses", got)
	}
}

func TestOpenAICompatible_replayedCall_isNamedTheWayItWasOffered(t *testing.T) {
	t.Parallel()
	c := serve(t, openAIToolCallWireName)
	p := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{Model: "gpt-test"})

	if _, err := p.Plan(context.Background(), withACallAlreadyMade()); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got := replayedName(t, c.body); !accepted.MatchString(got) {
		t.Errorf("replayed as %q, which the provider refuses", got)
	}
}

func withACallAlreadyMade() engine.PlanInput {
	in := input()
	in.Transcript = append(in.Transcript,
		engine.Turn{Kind: engine.TurnToolUse, CallID: "call_1",
			Tool: "crm.lookup", Args: []byte(`{"email":"a@b.com"}`)},
		engine.Turn{Kind: engine.TurnToolResult, CallID: "call_1",
			Content: []byte(`{"account":"acct_4471"}`)},
	)
	return in
}

// replayedName digs the tool name out of whichever message shape carried it.
//
// Read loosely on purpose: one format puts the call in a content block and the
// other in a field beside a string body, and this asserts about the name in
// both rather than about either shape.
func replayedName(t *testing.T, body map[string]any) string {
	t.Helper()
	messages, _ := body["messages"].([]any)

	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		blocks, _ := message["content"].([]any)
		for _, raw := range blocks {
			block, _ := raw.(map[string]any)
			if block["type"] == "tool_use" {
				name, _ := block["name"].(string)
				return name
			}
		}
		calls, _ := message["tool_calls"].([]any)
		for _, raw := range calls {
			call, _ := raw.(map[string]any)
			fn, _ := call["function"].(map[string]any)
			name, _ := fn["name"].(string)
			return name
		}
	}
	t.Fatal("no replayed call was sent")
	return ""
}

/*
Where the run is, told the same way by both providers.

A planner is only useful to the engine if swapping the vendor does not change
what the loop sees, and one of them was not telling the model which step it was
at or what its author said would end it. The same agent, published once, behaved
differently by vendor — and the half that stayed silent was the half where
`stops_when` could never be asserted, so the trail could never record it.
*/
func TestOpenAICompatible_atAStep_isToldWhichOneAndWhatEndsIt(t *testing.T) {
	t.Parallel()
	c := serve(t, openAIToolCallWireName)
	p := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{Model: "gpt-test"})

	in := input()
	in.Step = "Find who wrote in"
	in.StopsWhen = "the message does not say which customer it is about"
	if _, err := p.Plan(context.Background(), in); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	system := systemTurn(t, c.body)
	if !strings.Contains(system, in.Step) {
		t.Errorf("the model was not told which step it is at:\n%s", system)
	}
	if !strings.Contains(system, in.StopsWhen) {
		t.Errorf("the model was not told what ends the step:\n%s", system)
	}
}

func systemTurn(t *testing.T, body map[string]any) string {
	t.Helper()
	messages, _ := body["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "system" {
			content, _ := message["content"].(string)
			return content
		}
	}
	t.Fatal("no system turn was sent")
	return ""
}
