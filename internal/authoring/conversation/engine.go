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
	BeginTurn(ctx context.Context, b drafts.Beginning) (drafts.Started, error)
	SettleTurn(ctx context.Context, s drafts.Settlement) (drafts.Turn, error)
	Propose(ctx context.Context, p drafts.Proposing) (drafts.Proposal, error)
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

// Result is a turn and what it proposed: proposals past the cap or that the
// model's answer did not carry as operations are Dropped; those made against
// a draft that moved during the call are Stale. Both are counted, never kept.
type Result struct {
	Turn      drafts.Turn
	Replayed  bool
	Proposals []drafts.Proposal
	Dropped   int
	Stale     int
}

/*
Run is one turn (NT-018 phase 3, contracts 7 and 8).

The prompt is built and its worst case reserved before the model is called,
in a short transaction, so a refusal costs nothing and nothing is locked while
the model thinks. A key already used answers what was stored. Whatever the
model answers, the turn settles at what the call cost; its proposals are bound
to the ETag the prompt was built from, and dropped if the draft moved since.
*/
func (e Engine) Run(ctx context.Context, r Request) (Result, error) {
	if strings.TrimSpace(r.Message) == "" {
		return Result{}, ErrEmptyMessage
	}
	if len(r.Message) > MaxMessageBytes {
		return Result{}, fmt.Errorf("%w: a message of %d bytes, at most %d", ErrTurnTooLarge, len(r.Message), MaxMessageBytes)
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
		return e.fail(ctx, started.Turn, completion, err)
	}
	answer, err := ReadReply(completion.Text)
	if err != nil {
		return e.fail(ctx, started.Turn, completion, err)
	}
	turn, err := e.Store.SettleTurn(ctx, drafts.Settlement{Turn: started.Turn.ID, Cost: completion.Cost.Micros, Reply: answer.Text})
	if err != nil {
		return Result{Turn: started.Turn}, err
	}
	return e.propose(ctx, r, turn, answer)
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
	prompt, err := Prompt(r.Locale, projection, Window(turns), r.Message, r.Pasted)
	if err != nil {
		return "", "", 0, err
	}
	reserve, err := Reserve(prompt, e.Price, e.Priced)
	return prompt, opened.Revision, reserve, err
}

// fail settles a turn the model failed, or whose answer could not be read, at
// what the call cost, and returns why.
func (e Engine) fail(ctx context.Context, turn drafts.Turn, c model.Completion, cause error) (Result, error) {
	settled, err := e.Store.SettleTurn(ctx, drafts.Settlement{Turn: turn.ID, Cost: c.Cost.Micros, Failed: true})
	if err != nil {
		return Result{Turn: turn}, errors.Join(cause, err)
	}
	return Result{Turn: settled}, cause
}

// propose records the answer's operations against the ETag the prompt was
// built from. One the draft moved past is stale and dropped; any other
// refusal is the store's to report.
func (e Engine) propose(ctx context.Context, r Request, turn drafts.Turn, answer Reply) (Result, error) {
	out := Result{Turn: turn, Dropped: answer.Dropped}
	for _, op := range answer.Proposals {
		p, err := e.Store.Propose(ctx, drafts.Proposing{
			Agent: r.Agent, Turn: turn.ID, Op: op, Read: turn.Read,
			Caller: r.Caller, Validator: e.Validator, Now: r.Now,
		})
		var stale *domain.StaleError
		switch {
		case errors.As(err, &stale):
			out.Stale++
			continue
		case err != nil:
			return out, err
		}
		out.Proposals = append(out.Proposals, p)
	}
	return out, nil
}
