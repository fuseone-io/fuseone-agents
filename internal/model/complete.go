package model

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/fuseone/agents/internal/domain"
)

/*
Completer is one prompt in, one text out.

Separate from Planner on purpose. A Planner is agent-shaped: it sees a run's
state and transcript and proposes the next tool call. The authoring assistant
does neither — it reads a person's description of a process and hands back
fields. Forcing it through Planner would mean inventing a run that never
happened, and the distortion would surface later as a bug nobody could place.

It returns the cost because this is the only place the platform spends outside
a run: there is no ledger to fold and no per-run ceiling to check, so what it
consumed has to come back with the answer or it is lost.
*/
type Completer interface {
	Complete(ctx context.Context, prompt string) (Completion, error)
}

// Completion is the answer and what it cost.
type Completion struct {
	Text string
	Cost domain.Cost
}

// Completer builds one for a configured provider.
func (r *Registry) Completer(providerName string, cfg Config) (Completer, error) {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("model: no provider named %q", providerName)
	}

	// Built through the same paths a planner is, so an authoring call reaches
	// a provider exactly the way a run does — same credential, same base URL,
	// same HTTP client. No tools are offered to either shape here: the
	// assistant is asked to read prose and answer in JSON, and a tool list
	// would invite it to act instead.
	planner, err := r.Planner(providerName, cfg, nil)
	if err != nil {
		return nil, err
	}

	switch p.Kind {
	case KindAnthropic:
		return &anthropicCompleter{Anthropic: planner.(*Anthropic)}, nil
	default:
		return &openAICompleter{OpenAICompatible: planner.(*OpenAICompatible)}, nil
	}
}

type openAICompleter struct{ *OpenAICompatible }

func (o *openAICompleter) Complete(ctx context.Context, prompt string) (Completion, error) {
	var out chatResponse
	err := o.post(ctx, "/chat/completions", map[string]any{
		"model": o.cfg.Model,
		// No tools offered. The assistant is being asked to read prose and
		// answer in JSON; a tool list here would invite it to act.
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	}, &out)
	if err != nil {
		return Completion{}, err
	}
	if len(out.Choices) == 0 {
		return Completion{}, fmt.Errorf("model: %s returned no choices", o.provider.Name)
	}
	return Completion{Text: out.Choices[0].Message.Content, Cost: o.cost(out.Usage)}, nil
}

type anthropicCompleter struct{ *Anthropic }

func (a *anthropicCompleter) Complete(ctx context.Context, prompt string) (Completion, error) {
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.cfg.Model),
		MaxTokens: a.cfg.MaxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(a.cfg.Effort),
		},
	})
	if err != nil {
		return Completion{}, fmt.Errorf("model: %w", err)
	}

	// Thinking blocks come back beside the answer; only the text is the
	// answer, and concatenating the rest would feed reasoning into a parser.
	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return Completion{Text: text, Cost: a.cost(resp.Usage, a.cfg.Model)}, nil
}
