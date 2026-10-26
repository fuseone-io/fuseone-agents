package model_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
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
	mu     sync.Mutex
	body   map[string]any
	bodies []map[string]any
	server *httptest.Server
}

func (c *capture) record(body map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.body = body
	c.bodies = append(c.bodies, body)
}

func (c *capture) last() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

func (c *capture) all() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.bodies)
}

func serve(t *testing.T, response string) *capture {
	t.Helper()
	c := &capture{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		c.record(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(c.server.Close)
	return c
}

/*
And a read and a write that nothing orders but the lock.

The test below waits for every request before it reads, and wg.Wait establishes
a happens-before — so it proves the writer is synchronized and says nothing
about the reader. That is how ten unsynchronized reads lived in wire_test.go
beside a mutex meant to have closed this.

Two goroutines released by one channel, each signalling only after its own
access, so both accesses certainly happen and neither can be skipped by the
scheduler. What the detector then reports is the absence of an ordering between
them, which is the whole claim: only the lock provides one.
*/
func TestCapture_aReadAndAWriteAtOnce_areOrderedOnlyByItsLock(t *testing.T) {
	for _, one := range []struct {
		name string
		read func(*capture)
	}{
		{"all", func(c *capture) { _ = c.all() }},
		{"last", func(c *capture) { _ = c.last() }},
	} {
		t.Run(one.name, func(t *testing.T) {
			c := &capture{}
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				c.record(map[string]any{"read": one.name})
			}()
			go func() {
				defer wg.Done()
				<-start
				one.read(c)
			}()
			close(start)
			wg.Wait()

			if got := len(c.all()); got != 1 {
				t.Fatalf("recorded %d bodies, want the one that was written", got)
			}
		})
	}
}

// The planner may issue more than one provider request at once. The recorder
// is test infrastructure, but a race here makes the race gate probabilistic:
// rerunning until it passes would stop the gate from proving anything.
func TestCapture_recordsConcurrentRequests(t *testing.T) {
	c := serve(t, `{}`)
	const requests = 32

	start := make(chan struct{})
	errs := make(chan error, requests)
	var wg sync.WaitGroup
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			response, err := http.Post(c.server.URL, "application/json", strings.NewReader(`{}`))
			if err != nil {
				errs <- err
				return
			}
			if err := response.Body.Close(); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("request: %v", err)
	}
	if got := len(c.all()); got != requests {
		t.Fatalf("recorded requests = %d, want %d", got, requests)
	}
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

	choice, _ := c.last()["tool_choice"].(map[string]any)
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

	blocks, _ := c.last()["system"].([]any)
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
	var stable strings.Builder
	for i := 0; i <= cachedAt; i++ {
		m, _ := blocks[i].(map[string]any)
		text, _ := m["text"].(string)
		stable.WriteString(text)
		if strings.Contains(text, "Budget remaining") {
			t.Error("the per-turn budget note sits inside the cached prefix; it invalidates the cache every turn")
		}
	}
	if !strings.Contains(stable.String(), "Budget ceiling for this run") {
		t.Fatalf("cached system prompt has no provider-independent budget ceiling: %s", stable.String())
	}
}

func TestAnthropic_previousTranscriptPrefixIsCachedBeforeVolatileNotes(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)
	in := input()
	in.Step = "Investigate the alert"
	in.StopsWhen = "the cause is supported by evidence"
	in.Tools = append(in.Tools, domain.ToolMemoryFind)
	in.Transcript = append(in.Transcript,
		engine.Turn{Kind: engine.TurnToolUse, CallID: "call_1", Tool: "crm.lookup", Args: []byte(`{"email":"a@b.com"}`)},
		engine.Turn{Kind: engine.TurnToolResult, CallID: "call_1", Tool: "crm.lookup", Content: []byte(`{"id":42}`)},
	)

	_, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{}).
		Plan(context.Background(), in)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	messages, _ := c.last()["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %+v, want user/assistant/user without a consecutive guidance turn", messages)
	}
	wantRoles := []string{"user", "assistant", "user"}
	for i, message := range messages {
		m, _ := message.(map[string]any)
		if role := asString(m["role"]); role != wantRoles[i] {
			t.Fatalf("message %d role = %q, want %q", i, role, wantRoles[i])
		}
	}
	last := messageContentBlocks(t, messages[len(messages)-1])
	if len(last) != 2 || last[0]["type"] != "tool_result" || last[0]["cache_control"] == nil {
		t.Fatalf("last user turn = %+v, want the tool result cached before guidance", last)
	}
	volatile := last[1]
	if volatile["cache_control"] != nil || !strings.Contains(asString(volatile["text"]), "Budget remaining") {
		t.Fatalf("volatile guidance = %+v, want only the uncached remaining budget", volatile)
	}
	stable := anthropicSystemText(c.last())
	for _, want := range []string{
		"Investigate the alert", "Governed memory lookup is available", "Budget ceiling",
	} {
		if !strings.Contains(stable, want) {
			t.Errorf("stable system guidance does not contain %q:\n%s", want, stable)
		}
	}
}

func TestOpenAICompatible_transcriptGrowthWithinBudgetPreservesThePreviousRequestPrefix(t *testing.T) {
	t.Parallel()
	c := serve(t, openAIToolCall)
	planner := plannerFor(t, model.KindOpenAICompatible, c.server.URL, model.Config{})
	first := input()
	first.Step = "Investigate the alert"
	first.StopsWhen = "the cause is supported by evidence"
	first.Tools = append(first.Tools, domain.ToolMemoryFind)
	first.Remaining.Micros = 400_000

	if _, err := planner.Plan(context.Background(), first); err != nil {
		t.Fatalf("first Plan: %v", err)
	}
	second := first
	second.State.Called = []domain.ToolID{domain.ToolMemoryFind}
	second.Remaining.Micros = 100_000
	second.Transcript = append(slices.Clone(first.Transcript),
		engine.Turn{Kind: engine.TurnToolUse, CallID: "call_1", Tool: "crm.lookup", Args: []byte(`{"email":"a@b.com"}`)},
		engine.Turn{Kind: engine.TurnToolResult, CallID: "call_1", Tool: "crm.lookup", Content: []byte(`{"id":42}`)},
	)
	if _, err := planner.Plan(context.Background(), second); err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	bodies := c.all()
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want two", len(bodies))
	}
	firstMessages, _ := bodies[0]["messages"].([]any)
	secondMessages, _ := bodies[1]["messages"].([]any)
	if len(secondMessages) <= len(firstMessages) ||
		!reflect.DeepEqual(firstMessages, secondMessages[:len(firstMessages)]) {
		t.Fatalf("first request is not the prefix of the second:\nfirst:  %+v\nsecond: %+v",
			firstMessages, secondMessages)
	}
	system, _ := firstMessages[0].(map[string]any)
	text := asString(system["content"])
	if !strings.Contains(text, "Budget ceiling for this run") || strings.Contains(text, "Budget remaining") {
		t.Fatalf("system guidance is not stable across spend: %s", text)
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
				return anthropicPromptText(body)
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

			system := strings.Join(strings.Fields(tc.system(c.last())), " ")
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
				return anthropicPromptText(body)
			},
		},
		{
			name:     "openai-compatible",
			kind:     model.KindOpenAICompatible,
			response: openAIToolCall,
			system:   openAIPromptText,
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
			if strings.Contains(tc.system(without.last()), "Memory learning is enabled") {
				t.Fatalf("memory learning note was sent without a memory tool:\n%s", tc.system(without.last()))
			}

			with := serve(t, tc.response)
			in := input()
			in.Tools = append(in.Tools, domain.ToolMemoryFind, domain.ToolMemorySuggest)
			in.MemoryLearning = domain.MemoryLearningPolicy{Mode: domain.MemoryLearningReview}
			got, err := plannerFor(t, tc.kind, with.server.URL, model.Config{}).Plan(context.Background(), in)
			if err != nil {
				t.Fatalf("Plan with memory: %v", err)
			}
			system := tc.system(with.last())
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
			system = tc.system(disabled.last())
			if strings.Contains(system, "Memory learning is enabled") {
				t.Fatalf("memory learning note was sent while learning was off:\n%s", system)
			}
		})
	}
}

func TestMemoryFindGuidanceStaysCacheableAfterTheRunAlreadyLooked(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		kind     model.Kind
		response string
		stable   func(map[string]any) any
		prompt   func(map[string]any) string
	}{
		{name: "anthropic", kind: model.KindAnthropic, response: anthropicToolUse,
			stable: func(body map[string]any) any { return body["system"] }, prompt: anthropicPromptText},
		{name: "openai-compatible", kind: model.KindOpenAICompatible, response: openAIToolCall,
			stable: func(body map[string]any) any {
				messages, _ := body["messages"].([]any)
				return messages[0]
			}, prompt: openAIPromptText},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, tc.response)
			in := input()
			in.Tools = append(in.Tools, domain.ToolMemoryFind)
			planner := plannerFor(t, tc.kind, c.server.URL, model.Config{})
			if _, err := planner.Plan(context.Background(), in); err != nil {
				t.Fatalf("Plan before lookup: %v", err)
			}
			in.State.Called = []domain.ToolID{domain.ToolMemoryFind}
			if _, err := planner.Plan(context.Background(), in); err != nil {
				t.Fatalf("Plan after lookup: %v", err)
			}
			bodies := c.all()
			if !reflect.DeepEqual(tc.stable(bodies[0]), tc.stable(bodies[1])) {
				t.Fatalf("stable provider prefix changed after memory.find:\nbefore: %+v\nafter:  %+v",
					tc.stable(bodies[0]), tc.stable(bodies[1]))
			}
			prompt := tc.prompt(c.last())
			for _, want := range []string{"may already have searched", "materially narrower", "equivalent search"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt does not contain %q:\n%s", want, prompt)
				}
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

func messageContentBlocks(t *testing.T, message any) []map[string]any {
	t.Helper()
	m, _ := message.(map[string]any)
	raw, _ := m["content"].([]any)
	blocks := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		block, _ := item.(map[string]any)
		blocks = append(blocks, block)
	}
	return blocks
}

func anthropicSystemText(body map[string]any) string {
	var text strings.Builder
	blocks, _ := body["system"].([]any)
	for _, block := range blocks {
		m, _ := block.(map[string]any)
		text.WriteString(asString(m["text"]))
	}
	return text.String()
}

func anthropicPromptText(body map[string]any) string {
	var text strings.Builder
	blocks, _ := body["system"].([]any)
	for _, block := range blocks {
		m, _ := block.(map[string]any)
		text.WriteString(" ")
		text.WriteString(asString(m["text"]))
	}
	messages, _ := body["messages"].([]any)
	for _, message := range messages {
		m, _ := message.(map[string]any)
		content, _ := m["content"].([]any)
		for _, block := range content {
			item, _ := block.(map[string]any)
			text.WriteString(" ")
			text.WriteString(asString(item["text"]))
		}
	}
	return text.String()
}

func openAIPromptText(body map[string]any) string {
	var text strings.Builder
	messages, _ := body["messages"].([]any)
	for _, message := range messages {
		m, _ := message.(map[string]any)
		text.WriteString(" ")
		text.WriteString(asString(m["content"]))
	}
	return text.String()
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
	if _, sent := c.last()["reasoning_effort"]; sent {
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

func TestOpenAICompatible_transientWrappedIn400_isRetryable(t *testing.T) {
	t.Parallel()

	// A proxy in front of the provider (LiteLLM) sometimes wraps a transient
	// upstream failure in a 400. The status alone says permanent; the body
	// proves otherwise, and the body wins.
	cases := []struct {
		name string
		body string
		code string
	}{
		{
			name: "connection error named in the message",
			body: `{"error":{"message":"litellm.APIConnectionError: APIConnectionError: OpenAIException - Connection error.","type":null,"param":null,"code":"500"}}`,
			code: model.CodeProviderUnavailable,
		},
		{
			name: "timeout named in the message",
			body: `{"error":{"message":"litellm.Timeout: Connection timed out after 600.0 seconds","type":null,"param":null,"code":"408"}}`,
			code: model.CodeProviderUnavailable,
		},
		{
			name: "service unavailable named in the message",
			body: `{"error":{"message":"litellm.ServiceUnavailableError: No deployments available for selected model","type":null,"param":null}}`,
			code: model.CodeProviderUnavailable,
		},
		{
			name: "internal server error named in the message",
			body: `{"error":{"message":"litellm.InternalServerError: OpenAIException - server error","type":null,"param":null}}`,
			code: model.CodeProviderUnavailable,
		},
		{
			name: "rate limit named in the message",
			body: `{"error":{"message":"litellm.RateLimitError: RateLimitError: OpenAIException - Rate limit reached","type":null,"param":null,"code":"429"}}`,
			code: model.CodeRateLimited,
		},
		{
			name: "upstream status carried only in the body's code field",
			body: `{"error":{"message":"upstream failed","type":null,"param":null,"code":"503"}}`,
			code: model.CodeProviderUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)

			_, err := plannerFor(t, model.KindOpenAICompatible, srv.URL, model.Config{}).
				Plan(context.Background(), input())

			var provider *model.ProviderError
			if !errors.As(err, &provider) {
				t.Fatalf("Plan = %v, want a provider error", err)
			}
			if provider.Code != tc.code || !provider.Retryable {
				t.Errorf("classified %q retryable=%t, want %q retryable", provider.Code, provider.Retryable, tc.code)
			}
			// The wire status stays on the record: the ledger shows the 400
			// the proxy actually sent, only the retry verdict changes.
			if provider.Status != http.StatusBadRequest {
				t.Errorf("Status = %d, want the original 400", provider.Status)
			}
		})
	}
}

func TestOpenAICompatible_genuineBadRequest_stillParksAsPermanent(t *testing.T) {
	t.Parallel()

	// Inputs a lax reclassifier would wrongly wave through: a plain 400
	// with no transience marker must stay non-retryable, or an invalid
	// request burns budget forever while hiding the fault. The code field
	// may carry a provider's own numeric code, not an HTTP status — a
	// number outside the HTTP range is noise, never evidence.
	cases := []struct {
		name string
		body string
	}{
		{
			name: "no marker anywhere",
			body: `{"error":{"message":"litellm.BadRequestError: model gpt-x is not in model_list","type":"invalid_request_error","param":"model","code":"400"}}`,
		},
		{
			name: "provider-specific numeric code beyond the HTTP range",
			body: `{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"1001"}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(srv.Close)

			_, err := plannerFor(t, model.KindOpenAICompatible, srv.URL, model.Config{}).
				Plan(context.Background(), input())

			var provider *model.ProviderError
			if !errors.As(err, &provider) {
				t.Fatalf("Plan = %v, want a provider error", err)
			}
			if provider.Code != model.CodeBadRequest || provider.Retryable {
				t.Errorf("classified %q retryable=%t, want %q non-retryable", provider.Code, provider.Retryable, model.CodeBadRequest)
			}
		})
	}
}

func TestAnthropic_transientWrappedIn400_isRetryable(t *testing.T) {
	t.Parallel()

	// The same proxy can sit behind the anthropic kind; the evidence read
	// must not be an openai-compatible privilege.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"litellm.APIConnectionError: AnthropicException - Connection error."}}`)
	}))
	t.Cleanup(srv.Close)

	_, err := plannerFor(t, model.KindAnthropic, srv.URL, model.Config{}).
		Plan(context.Background(), input())

	var provider *model.ProviderError
	if !errors.As(err, &provider) {
		t.Fatalf("Plan = %v, want a provider error", err)
	}
	if provider.Code != model.CodeProviderUnavailable || !provider.Retryable {
		t.Errorf("classified %q retryable=%t, want %q retryable", provider.Code, provider.Retryable, model.CodeProviderUnavailable)
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

func TestAnthropic_hourPrefix_capsTheStablePrefixAndLeavesTheTailShort(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)

	_, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{CachePrefixTTL: "1h"}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	system, _ := c.last()["system"].([]any)
	if len(system) == 0 {
		t.Fatal("no system blocks on the wire")
	}
	prefix, _ := system[len(system)-1].(map[string]any)
	control, _ := prefix["cache_control"].(map[string]any)
	if control["ttl"] != "1h" {
		t.Errorf("prefix cache_control = %v, want ttl 1h", control)
	}

	// The tail changes every run; caching it for an hour pays double for
	// tokens that are never read again.
	messages, _ := c.last()["messages"].([]any)
	for _, m := range messages {
		raw, _ := json.Marshal(m)
		if strings.Contains(string(raw), `"ttl"`) {
			t.Errorf("a message block carries a ttl: %s", raw)
		}
	}
}

func TestAnthropic_defaultPrefix_sendsNoTTL(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicToolUse)

	_, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	raw, _ := json.Marshal(c.last())
	if strings.Contains(string(raw), `"ttl"`) {
		t.Errorf("default request carries a ttl: an agent nobody opted in would pay the hour rate")
	}
}

func TestOpenAICompatible_promptCacheKey_sentOnlyWhereTheProviderAcceptsIt(t *testing.T) {
	t.Parallel()

	// The strict providers answer 400 to unknown fields, so the key rides
	// only where the provider is known to take it.
	cases := []struct {
		name     string
		supports bool
		key      string
		want     bool
	}{
		{name: "supported provider with a key", supports: true, key: "ticket-triage", want: true},
		{name: "strict provider never sees the field", supports: false, key: "ticket-triage", want: false},
		{name: "no key means no field even where supported", supports: true, key: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, openAIToolCall)
			reg := model.NewRegistry(nil)
			if err := reg.Register(model.Provider{
				Name: "under-test", Kind: model.KindOpenAICompatible,
				BaseURL: c.server.URL, APIKey: "test-key",
				SupportsPromptCacheKey: tc.supports,
			}); err != nil {
				t.Fatalf("Register: %v", err)
			}
			p, err := reg.Planner("under-test", model.Config{CacheKey: tc.key}, schemas{})
			if err != nil {
				t.Fatalf("Planner: %v", err)
			}
			if _, err := p.Plan(context.Background(), input()); err != nil {
				t.Fatalf("Plan: %v", err)
			}

			got, present := c.last()["prompt_cache_key"]
			if present != tc.want {
				t.Fatalf("prompt_cache_key present = %t, want %t", present, tc.want)
			}
			if tc.want && got != "ticket-triage" {
				t.Errorf("prompt_cache_key = %v, want the agent's own key", got)
			}
		})
	}
}

func TestPreset_openai_acceptsThePromptCacheKey(t *testing.T) {
	t.Parallel()
	if !model.Presets["openai"].SupportsPromptCacheKey {
		t.Error("the openai preset should accept prompt_cache_key; without it every installation opts in by hand")
	}
}

const anthropicHourCacheWrite = `{
  "id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5",
  "content":[{"type":"tool_use","id":"toolu_1","name":"crm.lookup","input":{}}],
  "stop_reason":"tool_use",
  "usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":0,
           "cache_creation_input_tokens":90000,
           "cache_creation":{"ephemeral_5m_input_tokens":20000,"ephemeral_1h_input_tokens":70000}}
}`

func TestAnthropic_hourCacheWrites_arePricedAtTheirOwnRate(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicHourCacheWrite)

	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
		Model: "claude-opus-5",
		PricePerMTok: model.Prices{
			InputMicros: 5_000_000, OutputMicros: 25_000_000,
			CacheWriteMicros: 6_250_000, CacheWrite1hMicros: 10_000_000,
		},
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got.Cost.CacheWriteTokens != 90000 || got.Cost.CacheWrite1hTokens != 70000 {
		t.Errorf("writes = %d total / %d hour, want 90000/70000",
			got.Cost.CacheWriteTokens, got.Cost.CacheWrite1hTokens)
	}
	// 100*5 + 10*25 + 20000*6.25 + 70000*10 in micros-per-million terms.
	want := int64(100*5 + 10*25 + 20_000*6.25 + 70_000*10)
	if got.Cost.Micros != want {
		t.Errorf("Micros = %d, want %d — the hour window has its own rate", got.Cost.Micros, want)
	}
}

func TestAnthropic_hourCacheWrites_withoutOwnRate_billAtTheShortRate(t *testing.T) {
	t.Parallel()
	c := serve(t, anthropicHourCacheWrite)

	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
		Model: "claude-opus-5",
		PricePerMTok: model.Prices{
			InputMicros: 5_000_000, OutputMicros: 25_000_000,
			CacheWriteMicros: 6_250_000,
		},
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// Unconfigured is unconfigured: no invented multiplier, today's rate,
	// corrected on the prices screen rather than guessed in code.
	want := int64(100*5 + 10*25 + 90_000*6.25)
	if got.Cost.Micros != want {
		t.Errorf("Micros = %d, want %d", got.Cost.Micros, want)
	}
}

func TestAnthropic_missingWriteBreakdown_onAnHourAgent_countsEveryWriteAsHour(t *testing.T) {
	t.Parallel()
	// A proxy that strips the breakdown must err the bill upward, never
	// silently down: the agent asked for the hour window, so the writes are
	// presumed to have paid for it.
	const noBreakdown = `{
	  "id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5",
	  "content":[{"type":"tool_use","id":"toolu_1","name":"crm.lookup","input":{}}],
	  "stop_reason":"tool_use",
	  "usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":90000}
	}`
	c := serve(t, noBreakdown)

	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
		Model: "claude-opus-5", CachePrefixTTL: "1h",
		PricePerMTok: model.Prices{
			InputMicros: 5_000_000, OutputMicros: 25_000_000,
			CacheWriteMicros: 6_250_000, CacheWrite1hMicros: 10_000_000,
		},
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := int64(100*5 + 10*25 + 90_000*10)
	if got.Cost.CacheWrite1hTokens != 90000 || got.Cost.Micros != want {
		t.Errorf("writes1h = %d, Micros = %d; want 90000 and %d",
			got.Cost.CacheWrite1hTokens, got.Cost.Micros, want)
	}
}

func TestAnthropic_partialWriteBreakdown_onAnHourAgent_failsUpward(t *testing.T) {
	t.Parallel()
	// A proxy that strips only the 1h field leaves the 5m one standing; the
	// unexplained remainder must bill at the hour rate the agent asked for,
	// never silently at the short one.
	const partial = `{
	  "id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5",
	  "content":[{"type":"tool_use","id":"toolu_1","name":"crm.lookup","input":{}}],
	  "stop_reason":"tool_use",
	  "usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":90000,
	           "cache_creation":{"ephemeral_5m_input_tokens":20000}}
	}`
	c := serve(t, partial)

	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
		Model: "claude-opus-5", CachePrefixTTL: "1h",
		PricePerMTok: model.Prices{
			InputMicros: 5_000_000, OutputMicros: 25_000_000,
			CacheWriteMicros: 6_250_000, CacheWrite1hMicros: 10_000_000,
		},
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := int64(100*5 + 10*25 + 20_000*6.25 + 70_000*10)
	if got.Cost.CacheWrite1hTokens != 70000 || got.Cost.Micros != want {
		t.Errorf("writes1h = %d, Micros = %d; want 70000 and %d",
			got.Cost.CacheWrite1hTokens, got.Cost.Micros, want)
	}
}

func TestMarketPrices_anthropicWindows_followThePublishedMultipliers(t *testing.T) {
	t.Parallel()
	// Anthropic publishes the windows as multipliers of input — 1.25x for 5m,
	// 2x for 1h — so a market row that drifts from them is a typo, not a deal.
	for _, p := range model.MarketPrices() {
		name := p.Provider + "/" + p.Model
		if p.Provider != "anthropic" {
			continue
		}
		if p.Prices.CacheWriteMicros*4 != p.Prices.InputMicros*5 {
			t.Errorf("%s: 5m write %d is not 1.25x input %d", name, p.Prices.CacheWriteMicros, p.Prices.InputMicros)
		}
		if p.Prices.CacheWrite1hMicros != p.Prices.InputMicros*2 {
			t.Errorf("%s: 1h write %d is not 2x input %d", name, p.Prices.CacheWrite1hMicros, p.Prices.InputMicros)
		}
	}
}

func TestAnthropic_hourOnlyRate_readsAsAppliedNotZero(t *testing.T) {
	t.Parallel()
	// With only the hour rate configured, money flows — and the provenance
	// must say a rate applied, or the screen explains a positive figure as
	// "zero configured".
	c := serve(t, anthropicHourCacheWrite)

	got, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{
		Model:        "claude-opus-5",
		PricePerMTok: model.Prices{CacheWrite1hMicros: 10_000_000},
	}).Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !got.Price.NonZeroApplied {
		t.Error("NonZeroApplied = false while the hour rate billed the call")
	}
}

const toolChoiceRejection = `{"type":"error","error":{"type":"invalid_request_error","message":"tool_choice: type 'any' is not supported by this model; use 'auto'"}}`

func TestAnthropic_modelRejectingAnyToolChoice_learnsAutoAndRetries(t *testing.T) {
	t.Parallel()
	c := &capture{}
	calls := 0
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		c.record(body)
		calls++
		w.Header().Set("Content-Type", "application/json")
		choice, _ := body["tool_choice"].(map[string]any)
		if choice["type"] == "any" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, toolChoiceRejection)
			return
		}
		_, _ = io.WriteString(w, anthropicToolUse)
	}))
	t.Cleanup(c.server.Close)

	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5-5"})
	got, err := p.Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan after the downgrade: %v", err)
	}
	if got.Tool != "crm.lookup" {
		t.Errorf("Tool = %q, want the proposal from the retried call", got.Tool)
	}
	bodies := c.all()
	if len(bodies) != 2 {
		t.Fatalf("calls = %d, want the refusal and one retry", len(bodies))
	}
	retried, _ := bodies[1]["tool_choice"].(map[string]any)
	if retried["type"] != "auto" || retried["disable_parallel_tool_use"] != true {
		t.Errorf("retry tool_choice = %v, want auto with parallel use still off", retried)
	}

	// Learned, not rediscovered: the next turn pays no 400.
	if _, err := p.Plan(context.Background(), input()); err != nil {
		t.Fatalf("second Plan: %v", err)
	}
	bodies = c.all()
	if len(bodies) != 3 {
		t.Fatalf("calls = %d, want exactly one more", len(bodies))
	}
	learned, _ := bodies[2]["tool_choice"].(map[string]any)
	if learned["type"] != "auto" {
		t.Errorf("learned tool_choice = %v, want auto from the first request", learned)
	}
}

func TestAnthropic_ordinaryBadRequest_isNotMistakenForAToolChoiceLimit(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens is too large"}}`)
	}))
	t.Cleanup(srv.Close)

	_, err := plannerFor(t, model.KindAnthropic, srv.URL, model.Config{}).
		Plan(context.Background(), input())

	var provider *model.ProviderError
	if !errors.As(err, &provider) || provider.Code != model.CodeBadRequest {
		t.Fatalf("err = %v, want the plain bad request", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d; a genuine bad request must not be retried", calls)
	}
}

func TestAnthropic_toolChoiceLearning_isPerModel(t *testing.T) {
	t.Parallel()
	c := &capture{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		c.record(body)
		w.Header().Set("Content-Type", "application/json")
		choice, _ := body["tool_choice"].(map[string]any)
		if body["model"] == "claude-opus-5-5" && choice["type"] == "any" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, toolChoiceRejection)
			return
		}
		_, _ = io.WriteString(w, anthropicToolUse)
	}))
	t.Cleanup(c.server.Close)

	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5-5"})
	if _, err := p.Plan(context.Background(), input()); err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// A step naming the older model keeps the structural guarantee: the
	// limit belongs to the model that stated it, never to the planner.
	stepInput := input()
	stepInput.Model = "claude-opus-5"
	if _, err := p.Plan(context.Background(), stepInput); err != nil {
		t.Fatalf("Plan with the step override: %v", err)
	}
	bodies := c.all()
	last, _ := bodies[len(bodies)-1]["tool_choice"].(map[string]any)
	if bodies[len(bodies)-1]["model"] != "claude-opus-5" || last["type"] != "any" {
		t.Errorf("override call = model %v choice %v, want any on the model that takes it",
			bodies[len(bodies)-1]["model"], last)
	}
}

func TestAnthropic_underLearnedAuto_freeTextIsNudgedNotFinal(t *testing.T) {
	t.Parallel()
	c := &capture{}
	calls := 0
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		c.record(body)
		calls++
		w.Header().Set("Content-Type", "application/json")
		choice, _ := body["tool_choice"].(map[string]any)
		if choice["type"] == "any" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, toolChoiceRejection)
			return
		}
		if calls <= 2 {
			// The downgrade retry, then the model's polite status text.
			if calls == 2 {
				_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5",
				  "content":[{"type":"text","text":"Memória carregada; pendente executar V1."}],
				  "stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				return
			}
		}
		_, _ = io.WriteString(w, anthropicToolUse)
	}))
	t.Cleanup(c.server.Close)

	p := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5-5"})
	got, err := p.Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// The status text must not have closed the run: the nudged retry's tool
	// proposal is the answer.
	if got.Tool != "crm.lookup" || got.Outcome != "" {
		t.Fatalf("proposal = tool %q outcome %q, want the tool from the nudged call", got.Tool, got.Outcome)
	}
	bodies := c.all()
	if len(bodies) != 3 {
		t.Fatalf("calls = %d, want refusal, text, nudged retry", len(bodies))
	}
	raw, _ := json.Marshal(bodies[2])
	if !strings.Contains(string(raw), "Memória carregada") {
		t.Error("the nudged call does not carry the model's own text back to it")
	}
	// Both calls happened; the invoice is their sum, not the second twice.
	if got.Cost.InputTokens != 1201 || got.Cost.OutputTokens != 81 ||
		got.Cost.CacheReadTokens != 9000 || got.Cost.CacheWriteTokens != 300 {
		t.Errorf("cost = %+v, want the first text call plus the nudged one", got.Cost)
	}
}

func TestAnthropic_nudgeCallFailing_surfacesTheProviderError(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		calls++
		w.Header().Set("Content-Type", "application/json")
		choice, _ := body["tool_choice"].(map[string]any)
		switch {
		case choice["type"] == "any":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, toolChoiceRejection)
		case calls == 2:
			_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5",
			  "content":[{"type":"text","text":"status intermediário"}],
			  "stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"try later"}}`)
		}
	}))
	t.Cleanup(srv.Close)

	_, err := plannerFor(t, model.KindAnthropic, srv.URL, model.Config{Model: "claude-opus-5-5"}).
		Plan(context.Background(), input())

	// A failed nudge must fail the turn so the worker's retry machinery
	// runs — quietly accepting the status text is the bug this PR removes.
	var provider *model.ProviderError
	if !errors.As(err, &provider) || !provider.Retryable {
		t.Fatalf("err = %v, want the nudge call's own retryable failure", err)
	}
}

func TestAnthropic_nudgeReplay_carriesThinkingBlocksWhole(t *testing.T) {
	t.Parallel()
	c := &capture{}
	calls := 0
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		c.record(body)
		calls++
		w.Header().Set("Content-Type", "application/json")
		choice, _ := body["tool_choice"].(map[string]any)
		if choice["type"] == "any" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, toolChoiceRejection)
			return
		}
		if calls == 2 {
			_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5",
			  "content":[{"type":"thinking","thinking":"planejando a varredura","signature":"sig"},
			             {"type":"text","text":"vou executar V1"}],
			  "stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			return
		}
		_, _ = io.WriteString(w, anthropicToolUse)
	}))
	t.Cleanup(c.server.Close)

	if _, err := plannerFor(t, model.KindAnthropic, c.server.URL, model.Config{Model: "claude-opus-5-5"}).
		Plan(context.Background(), input()); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	bodies := c.all()
	raw, _ := json.Marshal(bodies[len(bodies)-1])
	// Adaptive thinking means real answers carry thinking blocks, and the
	// provider wants them replayed whole in multi-turn conversations.
	if !strings.Contains(string(raw), "planejando a varredura") {
		t.Error("the nudged call drops the thinking block from the replayed answer")
	}
}

func TestAnthropic_underLearnedAuto_textTwiceIsTheOutcome(t *testing.T) {
	t.Parallel()
	calls := 0
	text := `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5",
	  "content":[{"type":"text","text":"acabou mesmo"}],
	  "stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		calls++
		w.Header().Set("Content-Type", "application/json")
		choice, _ := body["tool_choice"].(map[string]any)
		if choice["type"] == "any" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, toolChoiceRejection)
			return
		}
		_, _ = io.WriteString(w, text)
	}))
	t.Cleanup(srv.Close)

	got, err := plannerFor(t, model.KindAnthropic, srv.URL, model.Config{Model: "claude-opus-5-5"}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// Insisting forever would burn budget against a model that has said its
	// piece twice; the second text stands as the outcome.
	if got.Outcome != "acabou mesmo" || calls != 3 {
		t.Fatalf("outcome = %q after %d calls, want the second text accepted", got.Outcome, calls)
	}
}

func TestAnthropic_withoutDowngrade_textOnlyStaysTheOutcome(t *testing.T) {
	t.Parallel()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5",
		  "content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(srv.Close)

	got, err := plannerFor(t, model.KindAnthropic, srv.URL, model.Config{Model: "claude-opus-5"}).
		Plan(context.Background(), input())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	// Under the structural mode text-only is already the provider breaking
	// the contract its tool_choice accepted; today's reading stands and no
	// nudge fires.
	if got.Outcome != "ok" || calls != 1 {
		t.Fatalf("outcome = %q after %d calls, want today's behaviour untouched", got.Outcome, calls)
	}
}
