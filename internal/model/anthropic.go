// Package model turns a run's state into the next proposed action by asking a
// language model.
//
// It is the only non-deterministic component in the platform, which is why its
// output is a proposal the Gate rules on rather than an instruction the loop
// obeys.
package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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
	client   anthropic.Client
	provider string
	cfg      Config
	tools    ToolSchemas

	// autoToolChoice remembers, per model, that "any" was refused with a 400
	// and "auto" is the mode this model takes. Learned from the provider's
	// own answer rather than shipped as a list, because a list in a binary
	// ages — and scoped to the model that said it, never to the planner.
	autoToolChoice sync.Map
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
	// CachePrefixTTL stretches the stable prefix's cache window: "1h" pays
	// the doubled write rate so a run on the next hour reads the prefix back
	// instead of re-writing it. Empty or "5m" keeps the provider default.
	// Applied to the prefix breakpoint only — the conversation tail changes
	// every run, and an hour of it is money for tokens never read again.
	CachePrefixTTL string
	// CacheKey names the cache this agent's requests should share — the
	// agent's own id, set by the resolver. Only sent to providers that
	// declare SupportsPromptCacheKey; the Anthropic path has no use for it,
	// since its cache is keyed by the prefix itself.
	CacheKey string
	// PricePerMTok converts usage into the ledger's money. Set per model from
	// the installation's own price list; zero means cost is recorded as tokens
	// only, never as a guess.
	PricePerMTok Prices
	// RateFor resolves the rate for a model this call actually used, which is
	// not always cfg.Model: a step may name its own. Without it the planner
	// prices every call at the agent's base rate, and a step that switches to
	// a cheaper model is billed as the expensive one — a FinOps figure that is
	// wrong in a direction nobody notices.
	RateFor func(model string) (Prices, bool)
	// PriceConfigured distinguishes an absent rate from an intentionally zero
	// one. The numeric rate alone cannot: both are Prices{}.
	PriceConfigured bool
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
	// CacheWrite1hMicros is the hour-window write rate, which the provider
	// bills above the short window's. Zero means unconfigured, and hour
	// writes then bill at CacheWriteMicros — today's figure, corrected on
	// the prices screen, never a multiplier invented in code.
	CacheWrite1hMicros int64
}

func (p Prices) IsZero() bool {
	return p == Prices{}
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

func New(client anthropic.Client, provider string, cfg Config, tools ToolSchemas) *Anthropic {
	cfg.withDefaults()
	return &Anthropic{client: client, provider: provider, cfg: cfg, tools: tools}
}

var _ engine.Planner = (*Anthropic)(nil)

// Plan asks the model for the next action.
//
// It performs exactly one round trip and never executes anything: the tool the
// model picks comes back as a Proposal for the Gate to rule on.
func (a *Anthropic) Plan(ctx context.Context, in engine.PlanInput) (engine.Proposal, error) {
	// Built once for this request and used in both directions: the names the
	// provider is offered, and the identifier a proposal is read back as.
	offered := namesFor(in)
	tools := a.toolParams(in.Tools, offered)
	stableGuidance := prefixStablePlanningNote(in, a.tools, offered)
	volatileGuidance := volatilePlanningNote(in)
	promptGuidance := stableGuidance + volatileGuidance
	prompt := promptInputBreakdown(in, a.cfg, a.tools, offered, promptGuidance)

	// The pair this call is actually made against — the step's, when it
	// named one. The provider is not overridable: it carries a credential,
	// and a definition able to choose one could route an installation's
	// traffic wherever its author liked.
	model := or(in.Model, a.cfg.Model)
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: a.cfg.MaxTokens,
		System:    a.system(stableGuidance),
		Messages:  a.messages(in, offered, volatileGuidance),
		Tools:     tools,
		// The engine can execute at most one proposal per turn, and finishing
		// is a platform tool. Requiring a single tool use prevents the model
		// from returning free text that the ledger cannot record as an action.
		// A model that refuses the requirement is downgraded to auto — see
		// toolChoiceFor — and free text then reads back as the run's outcome.
		ToolChoice: a.toolChoiceFor(model),
		// Adaptive is the only supported mode on current models; a fixed
		// thinking budget is rejected.
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(or(in.Effort, a.cfg.Effort)),
		},
	}
	resp, err := a.client.Messages.New(ctx, params)
	if err != nil && a.learnAutoToolChoice(model, err) {
		params.ToolChoice = a.toolChoiceFor(model)
		resp, err = a.client.Messages.New(ctx, params)
	}
	if err != nil {
		return engine.Proposal{}, classifyAnthropic(a.provider, err)
	}
	resp, extra, nudgeErr := a.nudgeFreeText(ctx, model, params, resp)
	if nudgeErr != nil {
		rate, configured := a.rateFor(model)
		return engine.Proposal{}, &engine.SpentPlanError{
			Cost:     extra,
			Prompt:   prompt,
			Price:    priceUse(rate, configured, extra),
			Provider: a.provider,
			Model:    model,
			Err:      classifyAnthropic(a.provider, nudgeErr),
		}
	}

	// Check the stop reason before reading content. A refusal returns HTTP 200
	// with an empty or partial content list, so indexing straight into
	// content[0] turns a policy decision into a nil dereference.
	rate, configured := a.rateFor(model)
	cost := a.cost(resp.Usage, model).Add(extra)
	price := priceUse(rate, configured, cost)
	if resp.StopReason == anthropic.StopReasonRefusal {
		return engine.Proposal{Cost: cost, Prompt: prompt, Price: price, Provider: a.provider, Model: model}, providerRefused(a.provider)
	}

	return a.proposalFrom(resp, offered, prompt, cost, price, model), nil
}

// autoNudge is said once to a model that answered free text under the
// learned auto mode. Under the structural mode text cannot end a run; auto
// traded that guarantee for compatibility, and this hands the model one
// chance to keep its side of it.
const autoNudge = "Continue the run: call the next tool now, or finish " +
	"through the finish tool. Free text does not end the run and is not " +
	"delivered to anyone."

/*
nudgeFreeText restores, behaviourally, what the auto downgrade traded away.

A model on tool_choice any cannot end a run with prose — the mode refuses it
structurally. A model learned down to auto can, and a polite status line
mid-run then reads back as the run's outcome: the run in the field ended at
its memory step, reporting the scan as pending, because the model summarised
before acting. The text is handed back with one instruction to continue; a
model that answers text twice has said its piece, and insisting further
would burn budget against a decision already made. The extra call's usage is
returned so the turn's cost stays the whole truth.
*/
func (a *Anthropic) nudgeFreeText(
	ctx context.Context, model string,
	params anthropic.MessageNewParams, resp *anthropic.Message,
) (*anthropic.Message, domain.Cost, error) {
	if _, auto := a.autoToolChoice.Load(model); !auto {
		return resp, domain.Cost{}, nil
	}
	if resp.StopReason == anthropic.StopReasonRefusal || hasToolUse(resp) {
		return resp, domain.Cost{}, nil
	}
	// The superseded answer's usage travels back as the extra: the caller
	// prices the final response, and the invoice is the sum of both calls.
	firstCost := a.cost(resp.Usage, model)
	nudged := params
	// The answer goes back whole — thinking blocks included — the way the
	// provider wants a multi-turn conversation continued.
	nudged.Messages = append(append([]anthropic.MessageParam{}, params.Messages...),
		resp.ToParam(),
		anthropic.NewUserMessage(anthropic.NewTextBlock(autoNudge)))
	second, err := a.client.Messages.New(ctx, nudged)
	if err != nil {
		// Quietly accepting the status text here would be the bug this
		// exists to remove; the turn fails and the worker's retry runs —
		// with the first call's spend travelling along, because it was
		// billed whether or not this one answered.
		return nil, firstCost, err
	}
	return second, firstCost, nil
}

func hasToolUse(resp *anthropic.Message) bool {
	for _, block := range resp.Content {
		if _, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			return true
		}
	}
	return false
}

// toolChoiceFor is the structural guarantee where the model takes it, and
// the learned downgrade where it refused: with auto, free text reads back as
// the run's outcome instead of an error nobody can act on.
func (a *Anthropic) toolChoiceFor(model string) anthropic.ToolChoiceUnionParam {
	if _, auto := a.autoToolChoice.Load(model); auto {
		return anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{
				DisableParallelToolUse: anthropic.Bool(true),
			},
		}
	}
	return anthropic.ToolChoiceUnionParam{
		OfAny: &anthropic.ToolChoiceAnyParam{
			DisableParallelToolUse: anthropic.Bool(true),
		},
	}
}

// learnAutoToolChoice reads a refusal of the required-tool mode and records
// it, answering whether the call is worth one retry. Only the named refusal
// counts: a plain 400 retried on a hunch would hide a genuinely bad request
// behind a second identical failure.
func (a *Anthropic) learnAutoToolChoice(model string, err error) bool {
	if _, already := a.autoToolChoice.Load(model); already {
		return false
	}
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		return false
	}
	if !strings.Contains(anthropicErrorMessage(apiErr), "tool_choice") {
		return false
	}
	a.autoToolChoice.Store(model, struct{}{})
	return true
}

/*
rateFor answers for the model a call actually used.

Falls back to the planner's own rate only when nothing can resolve the model,
so a step override is priced as itself rather than as the agent's default.
*/
func (a *Anthropic) rateFor(model string) (Prices, bool) {
	if a.cfg.RateFor != nil {
		if price, ok := a.cfg.RateFor(model); ok || model != a.cfg.Model {
			return price, ok
		}
	}
	return a.cfg.PricePerMTok, a.cfg.PriceConfigured
}

// proposalFrom reads the model's answer.
//
// A tool_use block is the next action. Finishing is also a tool_use block,
// owned by the platform, so text alone no longer closes a run by omission.
func (a *Anthropic) proposalFrom(
	resp *anthropic.Message, offered names, prompt domain.PromptInputBreakdown,
	cost domain.Cost, price domain.ModelPriceUse, model string,
) engine.Proposal {
	p := engine.Proposal{Cost: cost, Prompt: prompt, Price: price, Provider: a.provider, Model: model}

	var summary strings.Builder
	for _, block := range resp.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			summary.WriteString(variant.Text)
		case anthropic.ToolUseBlock:
			tool := offered.idOf(variant.Name)
			if isFinishTool(tool) {
				return finishProposal([]byte(variant.JSON.Input.Raw()), p)
			}
			p.Tool = tool
			// Input is raw JSON; never string-match it. Escaping of Unicode
			// and slashes varies between models.
			p.Args = []byte(variant.JSON.Input.Raw())
		}
	}

	if p.Tool == "" {
		p.Outcome, p.StoppedBy = readOutcome(summary.String())
	}
	return p
}

// cost converts provider usage into the ledger's units.
//
// Cache reads and writes are priced separately and recorded separately: a
// cache read costs a fraction of an input token, and folding them together is
// what makes an expensive agent impossible to diagnose.
func (a *Anthropic) cost(u anthropic.Usage, model string) domain.Cost {
	pr, _ := a.rateFor(model)
	const perMillion = 1_000_000

	// The provider documents the total as the sum of the two windows. A
	// proxy may strip one or both nested fields; whatever the breakdown
	// fails to explain is presumed to have paid the window this agent asked
	// for — upward on an hour agent, never silently down.
	hourWrites := min(u.CacheCreation.Ephemeral1hInputTokens, u.CacheCreationInputTokens)
	if a.cfg.CachePrefixTTL == "1h" {
		explained := u.CacheCreation.Ephemeral1hInputTokens + u.CacheCreation.Ephemeral5mInputTokens
		if unexplained := u.CacheCreationInputTokens - explained; unexplained > 0 {
			hourWrites = min(hourWrites+unexplained, u.CacheCreationInputTokens)
		}
	}
	hourRate := pr.CacheWrite1hMicros
	if hourRate == 0 {
		hourRate = pr.CacheWriteMicros
	}

	micros := u.InputTokens*pr.InputMicros/perMillion +
		u.OutputTokens*pr.OutputMicros/perMillion +
		u.CacheReadInputTokens*pr.CacheReadMicros/perMillion +
		(u.CacheCreationInputTokens-hourWrites)*pr.CacheWriteMicros/perMillion +
		hourWrites*hourRate/perMillion

	return domain.Cost{
		InputTokens:        u.InputTokens,
		OutputTokens:       u.OutputTokens,
		CacheReadTokens:    u.CacheReadInputTokens,
		CacheWriteTokens:   u.CacheCreationInputTokens,
		CacheWrite1hTokens: hourWrites,
		Micros:             micros,
	}
}

func formatMicros(m int64) string {
	return fmt.Sprintf("%d.%06d", m/1_000_000, m%1_000_000)
}

// or is the step's choice, falling back to the agent's. Almost every step
// takes the fallback: the lever exists for the one that does not.
func or(step, agent string) string {
	if step != "" {
		return step
	}
	return agent
}

/*
readOutcome separates what the run says happened from the exception it says
stopped it.

The marker is a convention the model was told about in as many words, and its
absence means only that nothing was asserted. Nothing is inferred from the
prose: a trail that decided for itself that a summary "sounds like" the
author's exception would be recording a claim nobody made.
*/
func readOutcome(text string) (outcome, stoppedBy string) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, stopMarker) {
		return trimmed, ""
	}

	line, rest, _ := strings.Cut(strings.TrimPrefix(trimmed, stopMarker), "\n")
	return strings.TrimSpace(rest), strings.TrimSpace(line)
}

const stopMarker = "STOP:"
