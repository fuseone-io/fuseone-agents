package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Turning one ask into a run, and deciding what to say when it does not.

Split from the sweep that claims it because they answer different questions.
The sweep is about ownership — one ask, one consumer, settled once. This is
about authority: which scope, which person, which agent, and whether that
person has already had their share.

Every refusal here is a Refusal and not a sentence. A code travels with it, and
the code is what an operator counts and what decides whether the same person
hears the same thing twice.
*/

/*
open takes an ask through the narrowing and returns the run, or why not.

Scope before agent, agent before ask, ask before run. Each step bounds what the
next may do, and none of them consults the text for anything the platform is
supposed to decide: the scope comes from the conversation, never from what
somebody wrote in it.
*/
func (c *Consumer) open(ctx context.Context, a Claimed) (domain.RunID, Refusal, error) {
	scope, err := c.scopes.ScopeOf(ctx, a.Channel, a.Conversation)
	switch {
	case errors.Is(err, ErrNoConversation), errors.Is(err, ErrAmbiguousConversation):
		// Ambiguity is refused rather than resolved, and the person is told
		// plainly: this conversation is not set up to start anything, which is
		// an operator's job and not theirs.
		c.log.Warn("an ask arrived in a conversation that speaks for no scope",
			"channel", a.Channel, "conversation", a.Conversation, "err", err)
		return "", Refusal{
			Why:    "This conversation is not set up to start agents. An administrator maps it to an area.",
			Reason: "no_scope",
		}, nil
	case err != nil:
		return "", Refusal{}, fmt.Errorf("channel: read the scope of %s: %w", a.Conversation, err)
	}

	startable, err := c.startable(ctx, scope)
	if err != nil {
		// Not a refusal. The ask is fine and this side is not, so it waits.
		return "", Refusal{}, fmt.Errorf("channel: read what is startable in %s: %w", scope, err)
	}

	asker, ask, refusal, err := c.resolveAsk(ctx, a, startable)
	if err != nil || refusal.Why != "" {
		return "", refusal, err
	}

	// The share this person has already had. Last of the narrowing on
	// purpose: a mention naming no agent opened nothing, and charging it
	// against a ceiling would let a typo spend somebody's morning.
	over, err := c.overCeiling(ctx, a)
	if err != nil {
		return "", Refusal{}, err
	}
	if over != nil {
		return "", *over, nil
	}

	input, err := json.Marshal(c.structured(ctx, a, ask, asker))
	if err != nil {
		return "", Refusal{}, fmt.Errorf("channel: record the ask %s: %w", a.EventID, err)
	}

	opened, err := c.opener.Open(ctx, Request{
		Agent:   ask.Agent,
		IdemKey: AskKey(a.Arrival),
		Trigger: "channel",
		By:      asker,
		Input:   input,
		// Somebody outside the platform typed this. On an internal channel
		// they are a colleague and the text is still theirs, not ours — and
		// the taint check is what stands between a sentence and an effect.
		Labels: domain.NewLabels(domain.LabelUntrusted),
		Origin: &domain.RunOrigin{
			Channel: a.Channel, Conversation: a.Conversation,
			// The message somebody typed, never the delivery that carried it.
			// A retry is the same message and the origin has to point at what
			// was said, which is also what a thread is keyed by.
			Message: a.Message, Thread: a.Thread,
		},
	})
	switch {
	case errors.Is(err, ErrWontStart):
		// Paused, stopped by a switch, still a draft. It will not start on a
		// retry either, so the person is told and the ask is closed.
		return "", Refusal{
			Why:    fmt.Sprintf("%s will not start: %v", ask.Agent, err),
			Reason: "wont_start",
		}, nil
	case err != nil:
		// Anything else is this side failing, and the ask waits for a sweep
		// that works. Closing it here would answer a good question with a
		// refusal that was never about the question.
		return "", Refusal{}, fmt.Errorf("channel: open a run for %s: %w", ask.Agent, err)
	}
	return opened.RunID, Refusal{}, nil
}

// startable is what an ask in this scope could name.
//
// The intersection of two facts that already exist: the agent lives here, and
// it declared that a conversation may start it. Neither is the author choosing
// who may ask, and neither is an administrator choosing what an agent does.
func (c *Consumer) startable(ctx context.Context, scope domain.Scope) ([]Startable, error) {
	published, err := c.published.List(ctx, scope, false)
	if err != nil {
		return nil, err
	}

	var out []Startable
	for _, one := range published {
		willing, err := c.willing.StartableFromConversation(ctx, one.ID, one.VersionID)
		if err != nil {
			return nil, err
		}
		if willing {
			out = append(out, Startable{ID: one.ID, Name: one.Name})
		}
	}
	return out, nil
}

func (c *Consumer) resolveAsk(
	ctx context.Context, a Claimed, startable []Startable,
) (domain.UserID, Ask, Refusal, error) {
	if a.Agent != "" {
		if a.RunAs == "" {
			return "", Ask{}, Refusal{
				Why:    "This conversation is configured to watch messages, but no platform principal was chosen to run them.",
				Reason: "misconfigured",
			}, nil
		}
		for _, one := range startable {
			if one.ID == a.Agent {
				return a.RunAs, Ask{Agent: a.Agent, Text: a.Text}, Refusal{}, nil
			}
		}
		return "", Ask{}, Refusal{
			Why:    fmt.Sprintf("This conversation is configured to start %s, but that agent cannot start here.", a.Agent),
			Reason: "no_agent",
		}, nil
	}

	asker, bound, err := c.bindings(ctx, a.Channel, a.AskedBy)
	if err != nil {
		// A store that was away is not an account nobody bound. Folded
		// together, the refusal below would tell somebody their account is
		// not linked about a state that was never true.
		return "", Ask{}, Refusal{}, fmt.Errorf("channel: read the binding for %s: %w", a.AskedBy, err)
	}
	if !bound {
		// A run acts on somebody's behalf. An account nobody bound speaks for
		// nobody, and running as nobody is how an ask acquires authority that
		// no person holds.
		return "", Ask{}, Refusal{
			Why:    "Your channel account is not linked to a platform user, so nothing can run on your behalf.",
			Reason: "unbound",
		}, nil
	}

	ask, err := Read(a.Text, startable)
	if err != nil {
		// The refusal already names what would have worked. It is the only
		// teaching surface a channel has.
		return "", Ask{}, Refusal{Why: err.Error(), Reason: "no_agent"}, nil
	}
	return asker, ask, Refusal{}, nil
}

/*
structured is what the ledger holds about the ask.

Not the sentence. A run whose explanation a year later is *somebody typed
"investiga isso aí" in a thread* is a screenshot, and the subject is what makes
it a record. Resolved only against what the platform itself posted: anything
else stays absent, which is the honest answer and leaves the agent to go and
search — a tool call somebody can audit rather than a guess made here.

The sentence travels too. Dropping it would make the record dishonest about
what was actually said.
*/
func (c *Consumer) structured(
	ctx context.Context, a Claimed, ask Ask, asker domain.UserID,
) structuredAsk {
	out := structuredAsk{Text: ask.Text, AskedBy: string(asker)}
	if a.Source.Key() != "" {
		out.Source = a.Source.Key()
	}

	// An ask that started its own thread is its own parent, and has no subject
	// to resolve. Compared against the message and not the delivery: those are
	// never the same string in a real channel, and comparing them meant this
	// branch never fired.
	if a.Thread == "" || a.Thread == a.Message {
		return out
	}
	run, found, err := c.subjects.AboutRun(ctx, a.Channel, a.Conversation, a.Thread)
	if err != nil {
		c.log.Warn("could not resolve what a thread is about",
			"channel", a.Channel, "thread", a.Thread, "err", err)
		return out
	}
	if !found {
		return out
	}
	out.Subject = &askSubject{Kind: "run", Run: string(run)}
	return out
}

/*
Refusal is an ask that became nothing.

Three parts because three readers. The sentence is for the person who asked and
names what would have worked. The reason is for whoever is counting later, and
a sentence with somebody's agent name in it counts nothing. And Silent is for
the conversation itself: the second refusal of the same kind inside the same
window is recorded and not said, because a limit that answers every message it
rejects amplifies the flood it exists to stop.
*/
type Refusal struct {
	Why    string
	Reason string
	Silent bool
}
