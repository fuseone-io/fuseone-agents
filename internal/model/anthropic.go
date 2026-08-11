// Package model turns a run's state into the next proposed action by asking a
// language model.
//
// It is the only non-deterministic component in the platform, which is why its
// output is a proposal the Gate rules on rather than an instruction the loop
// obeys.
package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// ErrRefused means the provider's safety classifiers declined the request.
//
// It arrives as a successful HTTP response with stop_reason "refusal", not as
// an error, so code that reads the first content block without checking would
// see an empty response instead. Retrying the same prompt will not help.
var ErrRefused = errors.New("model: the request was refused by the provider")

// Anthropic implements engine.Planner.
type Anthropic struct {
	client anthropic.Client
	cfg    Config
	tools  ToolSchemas
}

// ToolSchemas resolves the JSON Schema a tool accepts. The model is only ever
// offered the tools in the run's capability pack, so a tool outside the pack
// cannot be proposed in the first place (PRD SE-04).
type ToolSchemas interface {
	Schema(domain.ToolID) (name, description string, input map[string]any, ok bool)
}

// Config is the model configuration for one agent.
type Config struct {
	// Model defaults to claude-opus-5.
	Model string
	// Effort is the primary cost lever. Sweep it before reaching for a smaller
	// model: on Opus 5 the lower levels are unusually strong.
	Effort string
	// MaxTokens bounds thinking plus response text together. Thinking is on by
	// default on Opus 5, so a value sized for a non-thinking model truncates.
	MaxTokens int64
	// SystemPrompt is the agent's instructions, from its specification.
	SystemPrompt string
	// PricePerMTok converts usage into the ledger's money. Set per model from
	// the installation's own price list; zero means cost is recorded as tokens
	// only, never as a guess.
	PricePerMTok Prices
}

// Prices are the installation's rates, in micros per million tokens.
//
// Cache reads are a separate rate because they cost a fraction of input, and
// collapsing them into one number is what makes an agent's cost impossible to
// diagnose (PRD FO-08).
type Prices struct {
	InputMicros      int64
	OutputMicros     int64
	CacheReadMicros  int64
	CacheWriteMicros int64
}

func (c *Config) withDefaults() {
	if c.Model == "" {
		c.Model = "claude-opus-5"
	}
	if c.Effort == "" {
		c.Effort = string(anthropic.OutputConfigEffortHigh)
	}
	if c.MaxTokens <= 0 {
		// Thinking and response text share this ceiling.
		c.MaxTokens = 16000
	}
}

func New(client anthropic.Client, cfg Config, tools ToolSchemas) *Anthropic {
	cfg.withDefaults()
	return &Anthropic{client: client, cfg: cfg, tools: tools}
}

var _ engine.Planner = (*Anthropic)(nil)

// Plan asks the model for the next action.
//
// It performs exactly one round trip and never executes anything: the tool the
// model picks comes back as a Proposal for the Gate to rule on.
func (a *Anthropic) Plan(ctx context.Context, in engine.PlanInput) (engine.Proposal, error) {
	tools := a.toolParams(in.Tools)

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.cfg.Model),
		MaxTokens: a.cfg.MaxTokens,
		System:    a.system(in),
		Messages:  messagesFrom(in.Transcript),
		Tools:     tools,
		// Adaptive is the only supported mode on current models; a fixed
		// thinking budget is rejected.
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(a.cfg.Effort),
		},
	})
	if err != nil {
		return engine.Proposal{}, fmt.Errorf("model: %w", err)
	}

	// Check the stop reason before reading content. A refusal returns HTTP 200
	// with an empty or partial content list, so indexing straight into
	// content[0] turns a policy decision into a nil dereference.
	if resp.StopReason == anthropic.StopReasonRefusal {
		return engine.Proposal{Cost: a.cost(resp.Usage)}, ErrRefused
	}

	return a.proposalFrom(resp), nil
}

// system builds the system prompt.
//
// Order matters for caching: the API renders tools, then system, then
// messages, and a cache breakpoint on the last system block covers everything
// before it. Nothing volatile goes in here — no timestamps, no run identifiers
// — or the prefix changes on every request and nothing is ever read from cache
// (PRD FO-09).
func (a *Anthropic) system(in engine.PlanInput) []anthropic.TextBlockParam {
	blocks := []anthropic.TextBlockParam{{Text: a.cfg.SystemPrompt}}
	blocks = append(blocks, anthropic.TextBlockParam{Text: loopContract})

	// The breakpoint sits on the last stable block. The remaining-budget note
	// below it changes every turn and is deliberately outside the cached span.
	blocks[len(blocks)-1].CacheControl = anthropic.NewCacheControlEphemeralParam()

	if in.Budget.Micros > 0 {
		blocks = append(blocks, anthropic.TextBlockParam{
			Text: fmt.Sprintf(
				"Budget remaining for this run: %s. Pace yourself and finish cleanly rather than being cut off.",
				formatMicros(in.Remaining.Micros)),
		})
	}
	return blocks
}

// loopContract tells the model how this platform works. It is stable across
// every agent, so it sits inside the cached prefix.
const loopContract = `You are running inside a governed agent platform.

Every action you propose passes through a deterministic gate before it happens.
A refused call is reported back to you with the rule that refused it — treat
that as final for this run and choose another approach rather than retrying.

Propose one tool call at a time. When the task is complete, reply with a short
plain-text summary of the outcome and make no tool call; that is how you finish.`

func (a *Anthropic) toolParams(ids []domain.ToolID) []anthropic.ToolUnionParam {
	if a.tools == nil {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(ids))
	for _, id := range ids {
		name, desc, schema, ok := a.tools.Schema(id)
		if !ok {
			continue
		}
		tool := anthropic.ToolParam{
			Name:        name,
			Description: anthropic.String(desc),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: schema},
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return out
}

// proposalFrom reads the model's answer.
//
// A tool_use block is the next action; text alone means the model considers
// the run finished.
func (a *Anthropic) proposalFrom(resp *anthropic.Message) engine.Proposal {
	p := engine.Proposal{Cost: a.cost(resp.Usage)}

	var summary strings.Builder
	for _, block := range resp.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			summary.WriteString(variant.Text)
		case anthropic.ToolUseBlock:
			p.Tool = domain.ToolID(variant.Name)
			// Input is raw JSON; never string-match it. Escaping of Unicode
			// and slashes varies between models.
			p.Args = []byte(variant.JSON.Input.Raw())
		}
	}

	if p.Tool == "" {
		p.Done = true
		p.Outcome = strings.TrimSpace(summary.String())
	}
	return p
}

// cost converts provider usage into the ledger's units.
//
// Cache reads and writes are priced separately and recorded separately: a
// cache read costs a fraction of an input token, and folding them together is
// what makes an expensive agent impossible to diagnose.
func (a *Anthropic) cost(u anthropic.Usage) domain.Cost {
	pr := a.cfg.PricePerMTok
	const perMillion = 1_000_000

	micros := u.InputTokens*pr.InputMicros/perMillion +
		u.OutputTokens*pr.OutputMicros/perMillion +
		u.CacheReadInputTokens*pr.CacheReadMicros/perMillion +
		u.CacheCreationInputTokens*pr.CacheWriteMicros/perMillion

	return domain.Cost{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
		Micros:           micros,
	}
}

func formatMicros(m int64) string {
	return fmt.Sprintf("%d.%06d", m/1_000_000, m%1_000_000)
}

// messagesFrom converts the ledger-derived transcript into the wire format.
//
// Tool calls and their results must pair by identifier, and the transcript
// derives those identifiers from the run and sequence — so a rebuilt
// transcript pairs identically to the one the previous worker sent.
func messagesFrom(turns []engine.Turn) []anthropic.MessageParam {
	var out []anthropic.MessageParam

	for _, t := range turns {
		switch t.Kind {
		case engine.TurnInput:
			out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(t.Text)))

		case engine.TurnNote:
			out = append(out, anthropic.NewUserMessage(anthropic.NewTextBlock(t.Text)))

		case engine.TurnToolUse:
			var input any
			if len(t.Args) > 0 {
				_ = json.Unmarshal(t.Args, &input)
			}
			out = append(out, anthropic.NewAssistantMessage(
				anthropic.NewToolUseBlock(t.CallID, input, string(t.Tool)),
			))

		case engine.TurnToolResult:
			content := string(t.Content)
			if content == "" {
				content = "(no content)"
			}
			out = append(out, anthropic.NewUserMessage(
				anthropic.NewToolResultBlock(t.CallID, content, t.Failed),
			))
		}
	}
	return out
}
