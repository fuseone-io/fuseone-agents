package model_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/model"
)

// Both providers are exercised through the same assertions wherever the
// behaviour should be identical. A planner is only useful to the engine if
// swapping the vendor does not change what the loop sees.

type schemas struct{}

func (schemas) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	switch id {
	case "crm.lookup":
		return "crm.lookup", "Look a customer up by email.",
			map[string]any{"email": map[string]any{"type": "string"}}, true
	case domain.ToolMemoryFind:
		return string(id), "Find governed structured memory assertions.",
			map[string]any{"search": map[string]any{"type": "string"}}, true
	case domain.ToolMemorySuggest:
		return string(id), "Suggest a structured memory assertion.",
			map[string]any{
				"kind":      map[string]any{"type": "string"},
				"subject":   map[string]any{"type": "string"},
				"signature": map[string]any{"type": "string"},
				"claim":     map[string]any{"type": "string"},
			}, true
	default:
		return "", "", nil, false
	}
}

// capture records the request body a planner sent, so a test can assert on the
// wire shape rather than only on the parsed result.
type capture struct {
	body   map[string]any
	server *httptest.Server
}

func serve(t *testing.T, response string) *capture {
	t.Helper()
	c := &capture{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &c.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(c.server.Close)
	return c
}

func plannerFor(t *testing.T, kind model.Kind, url string, cfg model.Config) engine.Planner {
	return plannerForPrices(t, kind, url, cfg, nil)
}

func plannerForPrices(
	t *testing.T, kind model.Kind, url string, cfg model.Config,
	prices map[string]model.Prices,
) engine.Planner {
	t.Helper()

	reg := model.NewRegistry(nil)
	if err := reg.Register(model.Provider{
		Name: "under-test", Kind: kind, BaseURL: url, APIKey: "test-key",
		SupportsReasoningEffort: true,
		Prices:                  prices,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	p, err := reg.Planner("under-test", cfg, schemas{})
	if err != nil {
		t.Fatalf("Planner: %v", err)
	}
	return p
}

func input() engine.PlanInput {
	return engine.PlanInput{
		Transcript: []engine.Turn{
			{Kind: engine.TurnInput, Text: "O cliente reclama de cobrança duplicada."},
		},
		Tools:  []domain.ToolID{"crm.lookup"},
		Budget: domain.Budget{Micros: 500_000},
	}
}

// --- Anthropic ---------------------------------------------------------------

const anthropicToolUse = `{
  "id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5",
  "content":[{"type":"tool_use","id":"toolu_1","name":"crm.lookup","input":{"email":"a@b.com"}}],
  "stop_reason":"tool_use",
  "usage":{"input_tokens":1200,"output_tokens":80,"cache_read_input_tokens":9000,"cache_creation_input_tokens":300}
}`

func TestAnthropic_toolUse_becomesAProposal(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)

	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
		Model:        "claude-opus-5",
		PricePerMTok: model.Prices{InputMicros: 5_000_000, OutputMicros: 25_000_000, CacheReadMicros: 500_000},
	})

	got, err := p.Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got.Tool != "crm.lookup" {
		t.Errorf("Tool = %q, want crm.lookup", got.Tool)
	}
	if got.Done {
		t.Error("Done = true while a tool was proposed")
	}
	// Cache reads are priced separately from input; folding them together is
	// what makes an expensive agent impossible to diagnose.
	if got.Cost.CacheReadTokens != 9000 {
		t.Errorf("CacheReadTokens = %d, want 9000", got.Cost.CacheReadTokens)
	}
	want := int64(1200*5 + 80*25 + 9000*0.5 + 300*0)
	if got.Cost.Micros != want {
		t.Errorf("Micros = %d, want %d", got.Cost.Micros, want)
	}
}

func TestAnthropic_requiresExactlyOneToolUse(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)

	if _, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{}).
		Plan(context.Background(), input()); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	choice, _ := c.body["tool_choice"].(map[string]any)
	if choice["type"] != "any" || choice["disable_parallel_tool_use"] != true {
		t.Fatalf("tool_choice = %+v, want any with parallel tool use disabled", choice)
	}
}

func TestPlanner_reportsPromptCompositionBySource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		kind     model.Kind
		response string
	}{
		{
			name: "anthropic",
			kind: model.KindAnthropic,
			response: `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5",
			  "content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":10}}`,
		},
		{
			name: "openai-compatible",
			kind: model.KindOpenAICompatible,
			response: `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],
			  "usage":{"prompt_tokens":10}}`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, tc.response)
			in := engine.PlanInput{
				Transcript: []engine.Turn{
					{Kind: engine.TurnInput, Text: "alert text"},
					{Kind: engine.TurnToolUse, Tool: "crm.lookup", CallID: "call-1", Args: []byte(`{"email":"a@b.com"}`)},
					{Kind: engine.TurnToolResult, Tool: "crm.lookup", CallID: "call-1", Content: []byte(`{"name":"Ana"}`)},
					{Kind: engine.TurnNote, Text: "The platform refused another call."},
				},
				Tools:     []domain.ToolID{"crm.lookup"},
				Budget:    domain.Budget{Micros: 500_000},
				Remaining: domain.Consumption{Micros: 250_000},
			}

			got, err := plannerFor(t, tc.kind, c.server.URL, model.Config{
				SystemPrompt: "Follow the runbook.",
			}).Plan(context.Background(), in)
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}

			p := got.Prompt
			if p.Unit != "content_bytes" {
				t.Fatalf("Prompt.Unit = %q, want content_bytes", p.Unit)
			}
			if p.Input != int64(len("alert text")) ||
				p.Instructions != int64(len("Follow the runbook.")) ||
				p.Notes != int64(len("The platform refused another call.")) {
				t.Fatalf("Prompt = %+v, want exact source byte counts", p)
			}
			if p.ToolArguments != int64(len(`{"email":"a@b.com"}`)) ||
				p.ToolArgumentsByTool["crm.lookup"] != p.ToolArguments {
				t.Fatalf("tool argument bytes = %+v, want them attributed to crm.lookup", p)
			}
			if p.ToolResults != int64(len(`{"name":"Ana"}`)) ||
				p.ToolResultsByTool["crm.lookup"] != p.ToolResults {
				t.Fatalf("tool result bytes = %+v, want them attributed to crm.lookup", p)
			}
			if p.ToolSchemas <= 0 || p.Platform <= 0 || p.Total <= p.ToolResults {
				t.Fatalf("Prompt = %+v, want schemas and platform text included", p)
			}
			if got, want := p.Total, measuredPromptParts(p); got != want {
				t.Fatalf("Prompt.Total = %d, want measured source parts to sum to %d: %+v", got, want, p)
			}
			if got, want := sumToolBytes(p.ToolArgumentsByTool), p.ToolArguments; got != want {
				t.Fatalf("ToolArgumentsByTool sum = %d, want ToolArguments %d", got, want)
			}
			if got, want := sumToolBytes(p.ToolResultsByTool), p.ToolResults; got != want {
				t.Fatalf("ToolResultsByTool sum = %d, want ToolResults %d", got, want)
			}
		})
	}
}

func TestPlanner_recordsPriceProvenanceFromTheRegistry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		prices         map[string]model.Prices
		wantStatus     string
		wantNonZeroUse bool
	}{
		{
			name:       "missing",
			wantStatus: domain.ModelPriceMissing,
		},
		{
			name:       "configured zero",
			prices:     map[string]model.Prices{"gpt": {}},
			wantStatus: domain.ModelPriceConfigured,
		},
		{
			name:           "non-zero rounded below a micro",
			prices:         map[string]model.Prices{"gpt": {InputMicros: 1}},
			wantStatus:     domain.ModelPriceConfigured,
			wantNonZeroUse: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],
			  "usage":{"prompt_tokens":1,"completion_tokens":0}}`)

			got, err := plannerForPrices(
				t, model.KindOpenAICompatible, c.server.URL,
				model.Config{Model: "gpt"}, tc.prices,
			).Plan(context.Background(), input())
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}

			if got.Cost.Micros != 0 {
				t.Fatalf("Cost.Micros = %d, want zero so only provenance explains it", got.Cost.Micros)
			}
			if got.Price.Status != tc.wantStatus || got.Price.NonZeroApplied != tc.wantNonZeroUse {
				t.Fatalf("Price = %+v, want status %q and non-zero use %v",
					got.Price, tc.wantStatus, tc.wantNonZeroUse)
			}
		})
	}
}

// Elision is not part of what was sent, so a composition carrying it still
// sums to Total. Without the exclusion above this reads as a mismatch, which
// is the wrong failure: the prompt is correct and the adding is not.
func TestPromptParts_elisionIsNotPartOfTheTotal(t *testing.T) {
	t.Parallel()

	got := measuredPromptParts(domain.PromptInputBreakdown{
		Instructions:      100,
		ToolResults:       200,
		ToolResultsElided: 90_000,
		Total:             300,
	})
	if got != 300 {
		t.Errorf("parts summed to %d, want 300 — elision counted as sent", got)
	}
}

func measuredPromptParts(p domain.PromptInputBreakdown) int64 {
	v := reflect.ValueOf(p)
	t := v.Type()
	var total int64
	for i := 0; i < v.NumField(); i++ {
		// Total is the sum being checked, and ToolResultsElided is what the
		// model never received — adding it would assert that the composition
		// describes a prompt that was not sent. Named here rather than left to
		// the next int64 field: a counter added later joins this sum silently,
		// which is how a guard stops guarding without failing.
		switch t.Field(i).Name {
		case "Total", "ToolResultsElided":
			continue
		}
		if v.Field(i).Kind() != reflect.Int64 {
			continue
		}
		total += v.Field(i).Int()
	}
	return total
}

func sumToolBytes(m map[domain.ToolID]int64) int64 {
	var total int64
	for _, n := range m {
		total += n
	}
	return total
}

func TestAnthropic_finishTool_meansTheRunIsDone(t *testing.T) {
	t.Parallel()
	c := serve(t, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5",
	  "content":[{"type":"tool_use","id":"toolu_finish","name":"_fuseone__finish","input":{"summary":"Classifiquei como cobrança.","stopped_by":"não encontrar o cliente"}}],
	  "stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`)

	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if !got.Done || got.Tool != "" || got.Outcome != "Classifiquei como cobrança." ||
		got.StoppedBy != "não encontrar o cliente" {
		t.Errorf("proposal = %+v, want an explicit finished run", got)
	}
}

func TestAnthropic_textOnly_doesNotFinishTheRun(t *testing.T) {
	t.Parallel()
	c := serve(t, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5",
	  "content":[{"type":"text","text":"Classifiquei como cobrança."}],
	  "stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`)

	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// Text is useful evidence, but not an act. Finishing is a platform tool,
	// so the engine has a concrete step to record instead of inferring from
	// the absence of a tool call.
	if got.Done || got.Tool != "" || got.Outcome == "" {
		t.Errorf("proposal = %+v, want text held without a finish decision", got)
	}
}

func TestAnthropic_refusal_isReportedNotParsedAsEmpty(t *testing.T) {
	t.Parallel()
	c := serve(t, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5",
	  "content":[],"stop_reason":"refusal","usage":{"input_tokens":10,"output_tokens":0}}`)

	_, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{}).
		Plan(context.Background(), input())

	// A refusal arrives as a successful HTTP response with empty content.
	// Reading content[0] without checking the stop reason turns a policy
	// decision into a nil dereference.
	if !errors.Is(err, model.ErrRefused) {
		t.Errorf("Plan = %v, want %v", err, model.ErrRefused)
	}
}

func TestAnthropic_systemPrompt_isCachedAndFreeOfVolatileText(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)

	_, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
		SystemPrompt: "Você faz triagem de chamados.",
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	blocks, _ := c.body["system"].([]any)
	if len(blocks) < 2 {
		t.Fatalf("system has %d blocks, want the prompt plus the loop contract", len(blocks))
	}

	// The breakpoint must sit on the last *stable* block. Anything volatile —
	// the remaining budget — belongs after it, or the prefix changes every
	// turn and nothing is ever read from cache.
	var cachedAt = -1
	for i, b := range blocks {
		if m, _ := b.(map[string]any); m["cache_control"] != nil {
			cachedAt = i
		}
	}
	if cachedAt == -1 {
		t.Fatal("no cache breakpoint on the system prompt")
	}
	for i := 0; i <= cachedAt; i++ {
		m, _ := blocks[i].(map[string]any)
		text, _ := m["text"].(string)
		if strings.Contains(text, "Budget remaining") {
			t.Error("the per-turn budget note sits inside the cached prefix; it invalidates the cache every turn")
		}
	}
}

func TestLoopContract_tellsTheModelToFinishWithTheFinishTool(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		kind     model.Kind
		response string
		system   func(map[string]any) string
	}{
		{
			name:     "anthropic",
			kind:     model.KindAnthropic,
			response: anthropicToolUse,
			system: func(body map[string]any) string {
				blocks, _ := body["system"].([]any)
				var text strings.Builder
				for _, b := range blocks {
					m, _ := b.(map[string]any)
					text.WriteString(" ")
					text.WriteString(asString(m["text"]))
				}
				return text.String()
			},
		},
		{
			name:     "openai-compatible",
			kind:     model.KindOpenAICompatible,
			response: openAIToolCall,
			system: func(body map[string]any) string {
				msgs, _ := body["messages"].([]any)
				if len(msgs) == 0 {
					return ""
				}
				first, _ := msgs[0].(map[string]any)
				return asString(first["content"])
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, tc.response)

			_, err := plannerFor(t, tc.kind, c.server.URL, model.Config{}).
				Plan(context.Background(), input())
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}

			system := strings.Join(strings.Fields(tc.system(c.body)), " ")
			for _, want := range []string{
				"call that tool now",
				"Do not say that you will continue",
				"call the finish tool",
				"the only way to finish",
			} {
				if !strings.Contains(system, want) {
					t.Errorf("system prompt does not contain %q:\n%s", want, system)
				}
			}
		})
	}
}

func TestMemoryTools_areExplainedOnlyWhenOffered(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		kind     model.Kind
		response string
		system   func(map[string]any) string
	}{
		{
			name:     "anthropic",
			kind:     model.KindAnthropic,
			response: anthropicToolUse,
			system: func(body map[string]any) string {
				blocks, _ := body["system"].([]any)
				var text strings.Builder
				for _, b := range blocks {
					m, _ := b.(map[string]any)
					text.WriteString(" ")
					text.WriteString(asString(m["text"]))
				}
				return text.String()
			},
		},
		{
			name:     "openai-compatible",
			kind:     model.KindOpenAICompatible,
			response: openAIToolCall,
			system: func(body map[string]any) string {
				msgs, _ := body["messages"].([]any)
				first, _ := msgs[0].(map[string]any)
				return asString(first["content"])
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			without := serve(t, tc.response)
			base, err := plannerFor(t, tc.kind, without.server.URL, model.Config{}).
				Plan(context.Background(), input())
			if err != nil {
				t.Fatalf("Plan without memory: %v", err)
			}
			if strings.Contains(tc.system(without.body), "Memory learning is enabled") {
				t.Fatalf("memory learning note was sent without a memory tool:\n%s", tc.system(without.body))
			}

			with := serve(t, tc.response)
			in := input()
			in.Tools = append(in.Tools, domain.ToolMemoryFind, domain.ToolMemorySuggest)
			in.MemoryLearning = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}
			got, err := plannerFor(t, tc.kind, with.server.URL, model.Config{}).Plan(context.Background(), in)
			if err != nil {
				t.Fatalf("Plan with memory: %v", err)
			}
			system := tc.system(with.body)
			for _, want := range []string{
				"Governed memory lookup is available",
				"_fuseone__memory__find",
				"Search with short separate terms such as a service name plus an error code",
				"Memory learning is enabled",
				"_fuseone__memory__suggest",
				"Search memory first when the subject or signature may already exist",
				"Do not suggest one-off facts, secrets, approvals, permissions or broad opinions",
				"After a memory suggestion result, do not retry the same suggestion in this run",
			} {
				if !strings.Contains(system, want) {
					t.Errorf("system prompt does not contain %q:\n%s", want, system)
				}
			}
			if got.Prompt.Platform <= base.Prompt.Platform {
				t.Errorf("platform prompt bytes = %d, want more than %d after adding the memory note",
					got.Prompt.Platform, base.Prompt.Platform)
			}

			disabled := serve(t, tc.response)
			in.MemoryLearning = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningOff}
			if _, err := plannerFor(t, tc.kind, disabled.server.URL, model.Config{}).
				Plan(context.Background(), in); err != nil {
				t.Fatalf("Plan with manually granted memory suggest but learning off: %v", err)
			}
			system = tc.system(disabled.body)
			if strings.Contains(system, "Memory learning is enabled") {
				t.Fatalf("memory learning note was sent while learning was off:\n%s", system)
			}
		})
	}
}

func TestOpenAICompatible_finishTool_becomesTheSameFinishedProposal(t *testing.T) {
	t.Parallel()
	c := serve(t, `{
	  "choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[
	    {"id":"call_finish","type":"function","function":{"name":"_fuseone__finish","arguments":"{\"summary\":\"Pronto.\"}"}}]}}],
	  "usage":{"prompt_tokens":10,"completion_tokens":5}
	}`)

	got, err := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{Model: "gpt-x"}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !got.Done || got.Tool != "" || got.Outcome != "Pronto." {
		t.Errorf("proposal = %+v, want an explicit finished run", got)
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// --- OpenAI-compatible -------------------------------------------------------

const openAIToolCall = `{
  "choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[
    {"id":"call_1","type":"function","function":{"name":"crm.lookup","arguments":"{\"email\":\"a@b.com\"}"}}]}}],
  "usage":{"prompt_tokens":1200,"completion_tokens":80,"prompt_tokens_details":{"cached_tokens":900}}
}`

func TestOpenAICompatible_toolCall_becomesTheSameProposalShape(t *testing.T) {
	t.Parallel()
	c := serve(t, openAIToolCall)

	got, err := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{
		Model:        "gpt-x",
		PricePerMTok: model.Prices{InputMicros: 3_000_000, OutputMicros: 15_000_000, CacheReadMicros: 300_000},
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got.Tool != "crm.lookup" {
		t.Errorf("Tool = %q, want crm.lookup", got.Tool)
	}
	// Arguments arrive as a JSON string here and as an object on Anthropic.
	// The engine must not be able to tell the difference.
	var args map[string]any
	if err := json.Unmarshal(got.Args, &args); err != nil {
		t.Fatalf("Args is not valid JSON: %v", err)
	}
	if args["email"] != "a@b.com" {
		t.Errorf("Args = %s, want the email the model chose", got.Args)
	}
	// Cached tokens must not also be counted as input, or the run is billed
	// twice for the same prefix.
	if got.Cost.InputTokens != 300 || got.Cost.CacheReadTokens != 900 {
		t.Errorf("input/cached = %d/%d, want 300/900",
			got.Cost.InputTokens, got.Cost.CacheReadTokens)
	}
}

func TestOpenAICompatible_deepseekCacheFields_areUnderstood(t *testing.T) {
	t.Parallel()
	// DeepSeek reports cache hits under its own field name rather than
	// OpenAI's nested details object.
	c := serve(t, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"pronto"}}],
	  "usage":{"prompt_tokens":1000,"completion_tokens":10,"prompt_cache_hit_tokens":800}}`)

	got, err := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{Model: "deepseek-chat"}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got.Cost.CacheReadTokens != 800 || got.Cost.InputTokens != 200 {
		t.Errorf("cached/input = %d/%d, want 800/200",
			got.Cost.CacheReadTokens, got.Cost.InputTokens)
	}
}

func TestOpenAICompatible_contentFilter_isReportedAsARefusal(t *testing.T) {
	t.Parallel()
	c := serve(t, `{"choices":[{"finish_reason":"content_filter","message":{"role":"assistant","content":""}}],
	  "usage":{"prompt_tokens":10,"completion_tokens":0}}`)

	_, err := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{}).
		Plan(context.Background(), input())

	// Providers spell a policy stop differently; the engine sees one condition.
	if !errors.Is(err, model.ErrRefused) {
		t.Errorf("Plan = %v, want %v", err, model.ErrRefused)
	}
}

func TestOpenAICompatible_providerRejectingUnknownFields_getsNoReasoningEffort(t *testing.T) {
	t.Parallel()
	c := serve(t, `{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{}}`)

	reg := model.NewRegistry(nil)
	// A provider that has not opted in: sending reasoning_effort blindly is a
	// 400 on the strict ones.
	_ = reg.Register(model.Provider{
		Name: "strict", Kind: model.KindOpenAICompatible,
		BaseURL: c.server.URL, SupportsReasoningEffort: false,
	})
	p, _ := reg.Planner("strict", model.Config{Effort: "high"}, schemas{})

	if _, err := p.Plan(context.Background(), input()); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if _, sent := c.body["reasoning_effort"]; sent {
		t.Error("reasoning_effort was sent to a provider that does not accept it")
	}
}

func TestOpenAICompatible_upstreamError_carriesTheProviderMessage(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"model not found"}}`)
	}))
	t.Cleanup(srv.Close)

	_, err := plannerFor(t, model.KindOpenAICompatible, srv.URL, model.Config{}).
		Plan(context.Background(), input())

	// An unfamiliar provider's 400 is only diagnosable if its own text
	// survives to the operator.
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Errorf("error = %v, want the provider's own message", err)
	}
}

func TestOpenAICompatible_overloadBecomesAStableProviderFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-request-id", "req_529")
		w.WriteHeader(529)
		_, _ = io.WriteString(w, `{"error":{"type":"overloaded_error","message":"Overloaded"}}`)
	}))
	t.Cleanup(srv.Close)

	_, err := plannerFor(t, model.KindOpenAICompatible, srv.URL, model.Config{}).
		Plan(context.Background(), input())

	var provider *model.ProviderError
	if !errors.As(err, &provider) {
		t.Fatalf("Plan = %v, want a provider error", err)
	}
	if provider.Code != model.CodeProviderOverloaded || provider.Provider != "under-test" ||
		provider.Status != 529 || provider.RequestID != "req_529" || !provider.Retryable {
		t.Errorf("provider error = %+v, want a typed overload", *provider)
	}
	got, ok := model.FailureSummaryOf(err)
	if !ok {
		t.Fatal("FailureSummaryOf did not read the provider error")
	}
	if got.Code != model.CodeProviderOverloaded || got.RequestID != "req_529" {
		t.Errorf("summary = %+v, want the low-cardinality failure plus request id", got)
	}
}

// --- registry ---------------------------------------------------------------

func TestRegistry_unknownProvider_saysWhatIsAvailable(t *testing.T) {
	t.Parallel()

	reg := model.NewRegistry(nil)
	_ = reg.Register(model.Provider{Name: "anthropic", Kind: model.KindAnthropic})

	_, err := reg.Planner("openai", model.Config{}, schemas{})
	if err == nil || !strings.Contains(err.Error(), "anthropic") {
		t.Errorf("error = %v, want it to name the configured providers", err)
	}
}

func TestRegistry_openAICompatibleWithoutBaseURL_isRejectedAtRegistration(t *testing.T) {
	t.Parallel()

	// Self-hosted presets carry no URL on purpose. Catching it here beats a
	// confusing connection error on the first run of the day.
	err := model.NewRegistry(nil).Register(model.Provider{Name: "vllm", Kind: model.KindOpenAICompatible})
	if err == nil {
		t.Error("Register accepted an OpenAI-compatible provider with no base URL")
	}
}

func TestPresets_coverTheProvidersOperatorsExpect(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"anthropic", "openai", "deepseek", "kimi", "groq", "xai", "vllm", "ollama"} {
		p, ok := model.Preset(name)
		if !ok {
			t.Errorf("no preset for %q", name)
			continue
		}
		if p.Kind == model.KindOpenAICompatible && p.BaseURL == "" && name != "vllm" {
			t.Errorf("preset %q has no base URL", name)
		}
	}
}
