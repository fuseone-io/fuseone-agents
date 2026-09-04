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
	scope, err := c.mapping.ScopeOf(ctx, a.Channel, a.Conversation)
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

	record, err := c.structured(ctx, a, ask, asker)
	if err != nil {
		return "", Refusal{}, err
	}
	input, err := json.Marshal(record)
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

/*
resolveAsk answers who this ask runs as and which agent it is for.

Two paths with two sources of authority, which is why they are two functions
rather than one with a flag. A watched message runs as the principal an
administrator configured; a mention runs as the person whose channel account is
bound. Neither borrows the other's answer, and Agent being set on the arrival is
what says which one this is.
*/
func (c *Consumer) resolveAsk(
	ctx context.Context, a Claimed, startable []Startable,
) (domain.UserID, Ask, Refusal, error) {
	if a.Agent != "" {
		return watched(a, startable)
	}
	return c.mentioned(ctx, a, startable)
}

// watched resolves a message the conversation itself was configured to act on.
func watched(a Claimed, startable []Startable) (domain.UserID, Ask, Refusal, error) {
	if a.RunAs == "" {
		return "", Ask{}, Refusal{
			Why:    "This conversation is configured to watch messages, but no platform principal was chosen to run them.",
			Reason: "misconfigured",
		}, nil
	}
	if !canStart(startable, a.Agent) {
		return "", Ask{}, cannotStartHere(a.Agent), nil
	}
	return a.RunAs, Ask{Agent: a.Agent, Text: a.Text}, Refusal{}, nil
}

/*
mentioned resolves a message somebody addressed to the bot.

The agent may be named in the text or configured on the conversation, and
neither is authority: the name selects, and the person's binding is what the
run acts on behalf of. So the binding is read first — an unbound account is
refused whether or not the conversation would have chosen an agent for them.
*/
func (c *Consumer) mentioned(
	ctx context.Context, a Claimed, startable []Startable,
) (domain.UserID, Ask, Refusal, error) {
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

	agent, err := c.mapping.AgentOf(ctx, a.Channel, a.Conversation)
	if err != nil {
		return "", Ask{}, Refusal{}, fmt.Errorf(
			"channel: read the agent of %s: %w", a.Conversation, err)
	}
	ask, err := Read(a.Text, startable, agent)
	if err != nil {
		// The refusal already names what would have worked. It is the only
		// teaching surface a channel has.
		return "", Ask{}, Refusal{Why: err.Error(), Reason: "no_agent"}, nil
	}
	if !canStart(startable, ask.Agent) {
		// Only the configured agent reaches this: Read returns nothing else
		// that is not on the list. A conversation pointed at an agent that
		// cannot run in its scope is a misconfiguration, and saying so is what
		// stops an administrator debugging the person who asked.
		return "", Ask{}, cannotStartHere(ask.Agent), nil
	}
	if agent != "" && ask.Agent != agent {
		// Somebody reached past the binding to something else in the scope.
		// Refused rather than quietly rerouted: a name that was typed was
		// meant, and running a different agent on that sentence is the
		// confusion a binding exists to remove.
		return "", Ask{}, Refusal{
			Why: fmt.Sprintf(
				"This conversation starts %s. Mention the bot without naming another agent.", agent),
			Reason: "not_this_agent",
		}, nil
	}
	return asker, ask, Refusal{}, nil
}

func canStart(startable []Startable, agent domain.AgentID) bool {
	for _, one := range startable {
		if one.ID == agent {
			return true
		}
	}
	return false
}

func cannotStartHere(agent domain.AgentID) Refusal {
	return Refusal{
		Why: fmt.Sprintf(
			"This conversation is configured to start %s, but that agent cannot start here.", agent),
		Reason: "no_agent",
	}
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
) (structuredAsk, error) {
	out := structuredAsk{Text: ask.Text, AskedBy: string(asker)}
	if a.Source.Key() != "" {
		out.Source = a.Source.Key()
	}

	// An ask that started its own thread is its own parent, and has no subject
	// to resolve. Compared against the message and not the delivery: those are
	// never the same string in a real channel, and comparing them meant this
	// branch never fired.
	if a.Thread == "" || a.Thread == a.Message {
		return out, nil
	}
	run, found, err := c.subjects.AboutRun(ctx, a.Channel, a.Conversation, a.Thread)
	if err != nil {
		c.log.Warn("could not resolve what a thread is about",
			"channel", a.Channel, "thread", a.Thread, "err", err)
		return out, nil
	}
	if found {
		out.Subject = &askSubject{Kind: "run", Run: string(run)}
		return out, nil
	}

	include, err := c.includeThreadContext(ctx, a)
	if err != nil {
		return structuredAsk{}, err
	}
	if !include {
		return out, nil
	}
	out.Thread = c.readThreadContext(ctx, a)
	return out, nil
}

func (c *Consumer) includeThreadContext(ctx context.Context, a Claimed) (bool, error) {
	if c.threadPolicy == nil {
		return false, nil
	}
	include, err := c.threadPolicy.IncludeThreadContext(ctx, a.Channel, a.Conversation)
	if err != nil {
		return false, fmt.Errorf("channel: read thread context policy for %s: %w", a.Conversation, err)
	}
	return include, nil
}

func (c *Consumer) readThreadContext(ctx context.Context, a Claimed) *ThreadContext {
	out := &ThreadContext{Conversation: a.Conversation, Thread: a.Thread}
	if c.threads == nil {
		out.Unavailable = "thread context is not wired"
		return out
	}
	got, err := c.threads.Thread(ctx, a.Channel, a.Conversation, a.Thread, a.Message)
	if err != nil {
		c.log.Warn("could not read channel thread context",
			"channel", a.Channel, "conversation", a.Conversation, "thread", a.Thread, "err", err)
		out.Unavailable = err.Error()
		return out
	}
	if got.Conversation == "" {
		got.Conversation = a.Conversation
	}
	if got.Thread == "" {
		got.Thread = a.Thread
	}
	return &got
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
