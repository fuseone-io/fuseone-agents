package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
)

// Store is the draft store a turn reads, reserves, settles and proposes in.
// Declared here by the consumer.
type Store interface {
	Get(ctx context.Context, agent domain.AgentID, c drafts.Caller) (drafts.Opened, error)
	Turns(ctx context.Context, agent domain.AgentID, c drafts.Caller) ([]drafts.Turn, error)
	ReplayTurn(ctx context.Context, r drafts.Replaying) (drafts.Started, bool, error)
	BeginTurn(ctx context.Context, b drafts.Beginning) (drafts.Started, error)
	SettleTurn(ctx context.Context, s drafts.Settlement) (drafts.Settled, error)
	Proposals(ctx context.Context, agent domain.AgentID, c drafts.Caller) ([]drafts.Proposal, error)
}

// Engine runs turns: the store, the installation's model for authoring, its
// configured price, the day's ceiling, what the scope offers and the checks
// every proposal is judged with.
type Engine struct {
	Store       Store
	Completer   model.Completer
	Price       model.Prices
	Priced      bool
	DailyMicros int64
	Offer       Offer
	Validator   draft.Validator
}

// Request is one author's turn.
type Request struct {
	Agent   domain.AgentID
	Key     string
	Message string
	Locale  string
	Pasted  bool
	Caller  drafts.Caller
	Now     time.Time
}

// Result is a turn and what it proposed: proposals past the cap, that the
// model's answer did not carry as operations, or of a thread erased during
// the call are Dropped; those made against a draft that moved during the call
// are Stale. Both are counted, never kept.
type Result struct {
	Turn      drafts.Turn
	Replayed  bool
	Proposals []drafts.Proposal
	Dropped   int
	Stale     int
}

/*
Run is one turn (NT-018 phase 3, contracts 7 and 8).

A key already stored answers what was stored, before anything else is
judged. Otherwise the prompt is built and its worst case reserved before the
model is called, in a short transaction, so a refusal costs nothing and
nothing is locked while the model thinks. What the model answers is stored
whole, the reply and every proposal with the turn's real cost, or the turn
fails at that cost: it is never done with part of its answer.
*/
func (e Engine) Run(ctx context.Context, r Request) (Result, error) {
	if strings.TrimSpace(r.Message) == "" {
		return Result{}, ErrEmptyMessage
	}
	if len(r.Message) > MaxMessageBytes {
		return Result{}, fmt.Errorf("%w: a message of %d bytes, at most %d", ErrTurnTooLarge, len(r.Message), MaxMessageBytes)
	}
	if replayed, found, err := e.Replay(ctx, r); err != nil || found {
		return replayed, err
	}
	prompt, read, reserve, err := e.prepare(ctx, r)
	if err != nil {
		return Result{}, err
	}
	started, err := e.Store.BeginTurn(ctx, drafts.Beginning{
		Agent: r.Agent, Key: r.Key, Message: r.Message, Pasted: r.Pasted, Read: read,
		Reserve: reserve, DailyMicros: e.DailyMicros, Lease: Lease, Caller: r.Caller,
	})
	if err != nil || started.Replayed {
		return Result{Turn: started.Turn, Replayed: started.Replayed}, err
	}
	completion, err := e.Completer.Complete(ctx, prompt)
	if err != nil {
		return e.fail(ctx, r, started.Turn, completion, err)
	}
	answer, err := ReadReply(completion.Text)
	if err != nil {
		return e.fail(ctx, r, started.Turn, completion, err)
	}
	settled, err := e.Store.SettleTurn(ctx, drafts.Settlement{
		Agent: r.Agent, Caller: r.Caller, Turn: started.Turn.ID, Cost: completion.Cost.Micros, Reply: answer.Text,
		Proposed: answer.Proposals, Validator: e.Validator, Now: r.Now,
	})
	if err != nil {
		return e.fail(ctx, r, started.Turn, completion, err)
	}
	return Result{Turn: settled.Turn, Proposals: settled.Proposals, Stale: settled.Stale,
		Dropped: answer.Dropped + settled.Withheld}, nil
}

// Replay answers the turn the request's key already names, if any. It needs
// only the store: a stored turn is answered even where no new one could run.
func (e Engine) Replay(ctx context.Context, r Request) (Result, bool, error) {
	started, found, err := e.Store.ReplayTurn(ctx, drafts.Replaying{Agent: r.Agent, Key: r.Key, Caller: r.Caller})
	if err != nil || !found {
		return Result{}, false, err
	}
	return Result{Turn: started.Turn, Replayed: true}, true, nil
}

// prepare reads the draft and the window, renders the prompt and prices its
// worst case.
func (e Engine) prepare(ctx context.Context, r Request) (string, domain.Revision, int64, error) {
	opened, err := e.Store.Get(ctx, r.Agent, r.Caller)
	if err != nil {
		return "", "", 0, err
	}
	turns, err := e.Store.Turns(ctx, r.Agent, r.Caller)
	if err != nil {
		return "", "", 0, err
	}
	missing := make([]string, 0, len(opened.Missing))
	for _, m := range opened.Missing {
		missing = append(missing, string(m))
	}
	projection, err := Project(opened.Draft, missing, e.Offer)
	if err != nil {
		return "", "", 0, err
	}
	// What became of each earlier proposal, so the model does not take one still waiting
	// for one in place (#413).
	thread, err := e.Store.Proposals(ctx, r.Agent, r.Caller)
	if err != nil {
		return "", "", 0, err
	}
	prompt, err := Prompt(r.Locale, PromptInput{
		Projection: projection, Window: Window(turns), Outcomes: Outcomes(thread), Message: r.Message, Pasted: r.Pasted,
	})
	if err != nil {
		return "", "", 0, err
	}
	reserve, err := Reserve(prompt, e.Price, e.Priced)
	return prompt, opened.Revision, reserve, err
}

// fail settles a turn the model failed, whose answer could not be read, or
// whose answer could not be stored, at what the call cost, and returns why.
// Settled failed, it keeps none of the answer.
func (e Engine) fail(ctx context.Context, r Request, turn drafts.Turn, c model.Completion, cause error) (Result, error) {
	settled, err := e.Store.SettleTurn(ctx, drafts.Settlement{
		Agent: r.Agent, Caller: r.Caller, Turn: turn.ID, Cost: c.Cost.Micros, Failed: true,
	})
	if err != nil {
		return Result{Turn: turn}, errors.Join(cause, err)
	}
	return Result{Turn: settled.Turn}, cause
}
