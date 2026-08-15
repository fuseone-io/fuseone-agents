package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

/*
How large an instruction is, in the units it will be billed in.

A token count cannot be computed here. Tokenisation belongs to the model that
will read the text, it differs between vendors and between generations of the
same vendor, and a table shipped in a binary would be wrong for a model
released after the release. So the platform asks, and where the provider has
no way to answer it says so — an estimate printed under the word "tokens"
would be a wrong number in the one place somebody goes to size a prompt.
*/

// ErrNoTokeniser means this provider cannot say how it will read a text.
//
// It is a state, not a failure: nothing is broken and nothing needs fixing.
// Only Anthropic's wire format has a counting endpoint; the chat-completions
// shape everyone else settled on has none.
var ErrNoTokeniser = errors.New("model: this provider does not count tokens")

// Counter answers how many tokens a model reads an instruction as.
type Counter interface {
	Count(ctx context.Context, instructions string) (int64, error)
}

// Counter builds one for a configured provider, or reports that the provider
// has no way to answer.
func (r *Registry) Counter(providerName string, cfg Config) (Counter, error) {
	r.mu.RLock()
	p, ok := r.providers[providerName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("model: no provider named %q", providerName)
	}
	if p.Kind != KindAnthropic {
		return nil, fmt.Errorf("%s: %w", p.Name, ErrNoTokeniser)
	}

	// Built through the same path a planner is, so the count is asked of the
	// same endpoint and the same credential a run would use.
	planner, err := r.Planner(providerName, cfg, nil)
	if err != nil {
		return nil, err
	}
	return planner.(*Anthropic), nil
}

// Count asks the provider how it reads this instruction.
//
// Sent as the system prompt, which is where an instruction goes at run time —
// counted as a user message it would be a different number, for a request the
// platform never makes. It is the instruction alone: a turn also carries the
// loop contract, the transcript so far and the remaining budget, so this is
// the size of what the author wrote, never the size of a turn.
func (a *Anthropic) Count(ctx context.Context, instructions string) (int64, error) {
	resp, err := a.client.Messages.CountTokens(ctx, anthropic.MessageCountTokensParams{
		Model:  anthropic.Model(a.cfg.Model),
		System: anthropic.MessageCountTokensParamsSystemUnion{OfString: param.NewOpt(instructions)},
		// The endpoint requires a turn to count. This one is fixed and
		// tiny, and it is the same for every call, so what moves between
		// two counts is the instruction and nothing else.
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(".")),
		},
	})
	if err != nil {
		return 0, fmt.Errorf("model: %w", err)
	}
	return resp.InputTokens, nil
}
