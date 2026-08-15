package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// OpenAICompatible speaks the chat-completions wire format.
//
// One implementation covers most of the market — OpenAI, DeepSeek, Kimi,
// Groq, xAI, Together, Mistral, and anything self-hosted behind vLLM or
// Ollama — because they all expose the same endpoint shape and differ only in
// base URL, model identifier, and which optional fields they honour.
//
// The client is hand-written rather than taken from a vendor SDK for exactly
// that reason: the differences that matter here are in the *edges* each
// provider gets wrong (usage accounting, reasoning controls, argument
// encoding), and a vendor SDK bakes in one provider's assumptions about them.
type OpenAICompatible struct {
	http     *http.Client
	provider Provider
	cfg      Config
	tools    ToolSchemas
}

func NewOpenAICompatible(provider Provider, cfg Config, tools ToolSchemas, hc *http.Client) *OpenAICompatible {
	cfg.withDefaults()
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Minute}
	}
	return &OpenAICompatible{http: hc, provider: provider, cfg: cfg, tools: tools}
}

var _ engine.Planner = (*OpenAICompatible)(nil)

func (o *OpenAICompatible) Plan(ctx context.Context, in engine.PlanInput) (engine.Proposal, error) {
	// Built once for this request and used in both directions: the names the
	// provider is offered, and the identifier a proposal is read back as.
	offered := namesFor(in)
	body := chatRequest{
		Model:     o.cfg.Model,
		Messages:  o.chatMessages(in, offered),
		Tools:     o.chatTools(in.Tools, offered),
		MaxTokens: o.cfg.MaxTokens,
	}
	// Only some providers accept a reasoning control, and an unknown field is
	// a 400 on the strict ones — send it only where it is supported.
	if o.provider.SupportsReasoningEffort {
		body.ReasoningEffort = o.cfg.Effort
	}

	var out chatResponse
	if err := o.post(ctx, "/chat/completions", body, &out); err != nil {
		return engine.Proposal{}, err
	}
	if len(out.Choices) == 0 {
		return engine.Proposal{}, fmt.Errorf("model: %s returned no choices", o.provider.Name)
	}

	choice := out.Choices[0]
	// Providers spell a policy stop differently; treat them all as a refusal
	// so the caller has one condition to handle across every provider.
	if choice.FinishReason == "content_filter" {
		return engine.Proposal{Cost: o.cost(out.Usage)}, ErrRefused
	}

	return o.proposalFrom(choice, out.Usage, offered), nil
}

func (o *OpenAICompatible) proposalFrom(c chatChoice, u chatUsage, offered names) engine.Proposal {
	p := engine.Proposal{Cost: o.cost(u)}

	if len(c.Message.ToolCalls) > 0 {
		call := c.Message.ToolCalls[0]
		p.Tool = offered.idOf(call.Function.Name)
		// Arguments arrive as a JSON *string* here, unlike Anthropic's object.
		// The Gate validates the decoded form, so pass the bytes through.
		p.Args = []byte(call.Function.Arguments)
		return p
	}

	p.Done = true
	p.Outcome = strings.TrimSpace(c.Message.Content)
	return p
}

// cost converts usage, honouring whichever cache accounting the provider
// happens to report.
//
// This is where multi-provider support genuinely degrades: OpenAI reports
// cached tokens under prompt_tokens_details, DeepSeek under its own
// hit/miss pair, and most others report nothing at all. On a provider that
// reports nothing, cached tokens are billed as ordinary input — the figure is
// an over-estimate, never a silent under-count.
func (o *OpenAICompatible) cost(u chatUsage) domain.Cost {
	pr := o.cfg.PricePerMTok
	const perMillion = 1_000_000

	cached := u.PromptTokensDetails.CachedTokens
	if cached == 0 {
		cached = u.PromptCacheHitTokens
	}
	uncached := u.PromptTokens - cached
	if uncached < 0 {
		uncached = u.PromptTokens
		cached = 0
	}

	micros := uncached*pr.InputMicros/perMillion +
		u.CompletionTokens*pr.OutputMicros/perMillion +
		cached*pr.CacheReadMicros/perMillion

	return domain.Cost{
		InputTokens:     uncached,
		OutputTokens:    u.CompletionTokens,
		CacheReadTokens: cached,
		Micros:          micros,
	}
}

func (o *OpenAICompatible) chatMessages(in engine.PlanInput, offered names) []chatMessage {
	system := o.cfg.SystemPrompt + "\n\n" + loopContract
	// Where the run is, and the exception its author wrote for that step.
	// After the contract rather than inside it: this changes as the run
	// advances, and what a provider caches is the prefix that does not.
	if in.Step != "" {
		system += "\n\n" + stepNote(in)
	}
	if in.Budget.Micros > 0 {
		system += fmt.Sprintf(
			"\n\nBudget remaining for this run: %s. Pace yourself and finish cleanly rather than being cut off.",
			formatMicros(in.Remaining.Micros))
	}

	// The system turn stays first and byte-stable across turns: every provider
	// with prompt caching keys on the prefix, and none of them expose explicit
	// breakpoints, so a stable prefix is the only lever available.
	msgs := []chatMessage{{Role: "system", Content: system}}

	for _, t := range in.Transcript {
		switch t.Kind {
		case engine.TurnInput, engine.TurnNote:
			msgs = append(msgs, chatMessage{Role: "user", Content: t.Text})

		case engine.TurnToolUse:
			msgs = append(msgs, chatMessage{
				Role: "assistant",
				ToolCalls: []chatToolCall{{
					ID:       t.CallID,
					Type:     "function",
					Function: chatFunctionCall{Name: offered.wire[t.Tool], Arguments: argsString(t.Args)},
				}},
			})

		case engine.TurnToolResult:
			content := string(t.Content)
			if content == "" {
				content = "(no content)"
			}
			msgs = append(msgs, chatMessage{Role: "tool", ToolCallID: t.CallID, Content: content})
		}
	}
	return msgs
}

func (o *OpenAICompatible) chatTools(ids []domain.ToolID, offered names) []chatTool {
	if o.tools == nil {
		return nil
	}
	out := make([]chatTool, 0, len(ids))
	for _, id := range ids {
		_, desc, schema, ok := o.tools.Schema(id)
		if !ok {
			continue
		}
		out = append(out, chatTool{
			Type: "function",
			Function: chatFunctionDef{
				Name:        offered.wire[id],
				Description: desc,
				Parameters: map[string]any{
					"type":       "object",
					"properties": schema,
				},
			},
		})
	}
	return out
}

func (o *OpenAICompatible) post(ctx context.Context, path string, body, into any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("model: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(o.provider.BaseURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("model: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if o.provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.provider.APIKey)
	}
	for k, v := range o.provider.Headers {
		req.Header.Set(k, v)
	}

	resp, err := o.http.Do(req)
	if err != nil {
		return fmt.Errorf("model: %s: %w", o.provider.Name, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("model: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The body carries the provider's own error text, which is the only
		// thing that makes a 400 from an unfamiliar provider diagnosable.
		return fmt.Errorf("model: %s returned %d: %s",
			o.provider.Name, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("model: decode %s response: %w", o.provider.Name, err)
	}
	return nil
}

func argsString(args []byte) string {
	if len(args) == 0 {
		return "{}"
	}
	return string(args)
}

// --- wire format -------------------------------------------------------------

type chatRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Tools           []chatTool    `json:"tools,omitempty"`
	MaxTokens       int64         `json:"max_tokens,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}

type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatTool struct {
	Type     string          `json:"type"`
	Function chatFunctionDef `json:"function"`
}

type chatFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

type chatChoice struct {
	FinishReason string      `json:"finish_reason"`
	Message      chatMessage `json:"message"`
}

// chatUsage carries every cache-accounting shape seen in the wild; whichever
// the provider populates is the one that gets used.
type chatUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
}
