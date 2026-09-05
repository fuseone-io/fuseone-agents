package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	mapped, refusal, err := c.mappingOf(ctx, a)
	if err != nil || refusal.Why != "" {
		return "", refusal, err
	}

	startable, err := c.startable(ctx, mapped.Scope)
	if err != nil {
		// Not a refusal. The ask is fine and this side is not, so it waits.
		return "", Refusal{}, fmt.Errorf(
			"channel: read what is startable in %s: %w", mapped.Scope, err)
	}

	asker, ask, refusal, err := c.resolveAsk(ctx, a, mapped, startable)
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

	input, refusal, err := c.recorded(ctx, a, ask, asker)
	if err != nil || refusal.Why != "" {
		return "", refusal, err
	}

	return c.start(ctx, a, starting{ask: ask, asker: asker, input: input})
}

/*
recorded builds what the ledger will hold, or refuses an ask holding no
question.

The two belong together because the second can only be decided by looking at
the first. A mention that said nothing and found nothing is not an ask, and
being inside a thread is not the same as having one: a thread the conversation
never opted into reading, or one Slack would not give us, leaves the agent
exactly as empty-handed as a bare mention — and the run is paid for either way.

One rule for both paths. It was a mention-only rule for a while, excused by
alerts whose words live in blocks rather than in `text` — but the Slack adapter
reads those into the text now, so a watched message that arrives empty is one
that really said nothing, and paying for it is no more defensible than paying
for a bare mention. Only the sentence differs, because only one of the two
readers can do anything about it.
*/
func (c *Consumer) recorded(
	ctx context.Context, a Claimed, ask Ask, asker domain.UserID,
) ([]byte, Refusal, error) {
	record, err := c.structured(ctx, a, ask, asker)
	if err != nil {
		return nil, Refusal{}, err
	}
	if !record.carriesAQuestion() {
		why := "Nothing in this message could be read as a question, so no agent was started."
		if fromMention(a) {
			why = "Say what you need in the same message — a mention on its own starts nothing."
		}
		return nil, Refusal{Why: why, Reason: "no_ask"}, nil
	}
	input, err := json.Marshal(record)
	if err != nil {
		return nil, Refusal{}, fmt.Errorf("channel: record the ask %s: %w", a.EventID, err)
	}
	return input, Refusal{}, nil
}

/*
mappingOf reads the conversation and settles what it will not do.

Both refusals here are about configuration rather than about the message, and
both are answered before anything is enumerated: neither depends on what is
published, so a catalogue that was away would leave somebody unanswered over a
question already decided.
*/
func (c *Consumer) mappingOf(ctx context.Context, a Claimed) (Mapped, Refusal, error) {
	mapped, err := c.mapping.Resolve(ctx, a.Channel, a.Conversation)
	switch {
	case errors.Is(err, ErrNoConversation), errors.Is(err, ErrAmbiguousConversation):
		// Ambiguity is refused rather than resolved, and the person is told
		// plainly: this conversation is not set up to start anything, which is
		// an operator's job and not theirs.
		c.log.Warn("an ask arrived in a conversation that speaks for no scope",
			"channel", a.Channel, "conversation", a.Conversation, "err", err)
		return Mapped{}, Refusal{
			Why:    "This conversation is not set up to start agents. An administrator maps it to an area.",
			Reason: "no_scope",
		}, nil
	case err != nil:
		return Mapped{}, Refusal{}, fmt.Errorf(
			"channel: read the scope of %s: %w", a.Conversation, err)
	}

	// The mode is a boundary, not a label. Neither door filters a mention by
	// it — they cannot reply, and a conversation that answers nothing is
	// indistinguishable from a broken one — so it is held here, where the
	// configuration is already read and the person can be told.
	if fromMention(a) && !StartsFromMentions(mapped.Mode) {
		return Mapped{}, Refusal{
			Why:    "This conversation only starts agents from its configured message sources, not from mentions.",
			Reason: "not_from_mentions",
		}, nil
	}
	return mapped, Refusal{}, nil
}

// starting is everything decided about an ask, ready to become a run.
//
// A struct because the parts are easy to transpose: the asker and the agent
// are both names, and a caller that swapped them would open somebody else's
// run under somebody else's authority and say nothing about it.
type starting struct {
	ask   Ask
	asker domain.UserID
	input []byte
}

// start turns a settled ask into a run, or says why it will never be one.
func (c *Consumer) start(
	ctx context.Context, a Claimed, s starting,
) (domain.RunID, Refusal, error) {
	opened, err := c.opener.Open(ctx, Request{
		Agent:   s.ask.Agent,
		IdemKey: AskKey(a.Arrival),
		Trigger: "channel",
		By:      s.asker,
		Input:   s.input,
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
			Why:    fmt.Sprintf("%s will not start: %v", s.ask.Agent, err),
			Reason: "wont_start",
		}, nil
	case err != nil:
		// Anything else is this side failing, and the ask waits for a sweep
		// that works. Closing it here would answer a good question with a
		// refusal that was never about the question.
		return "", Refusal{}, fmt.Errorf("channel: open a run for %s: %w", s.ask.Agent, err)
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
	ctx context.Context, a Claimed, mapped Mapped, startable []Startable,
) (domain.UserID, Ask, Refusal, error) {
	if !fromMention(a) {
		return watched(a, startable)
	}
	return c.mentioned(ctx, a, mapped, startable)
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
onBehalfOf answers which platform user a mention runs as.

A run acts on somebody's behalf. An account nobody bound speaks for nobody, and
running as nobody is how an ask acquires authority that no person holds — so
this is read before the conversation's agent is consulted at all, and an
unbound account is refused whether or not a conversation would have chosen an
agent for them.
*/
func (c *Consumer) onBehalfOf(ctx context.Context, a Claimed) (domain.UserID, Refusal, error) {
	asker, bound, err := c.bindings(ctx, a.Channel, a.AskedBy)
	if err != nil {
		// A store that was away is not an account nobody bound. Folded
		// together, the refusal below would tell somebody their account is not
		// linked about a state that was never true.
		return "", Refusal{}, fmt.Errorf("channel: read the binding for %s: %w", a.AskedBy, err)
	}
	if !bound {
		return "", Refusal{
			Why:    "Your channel account is not linked to a platform user, so nothing can run on your behalf.",
			Reason: "unbound",
		}, nil
	}
	return asker, Refusal{}, nil
}

/*
mentioned resolves a message somebody addressed to the bot.

The agent may be named in the text or configured on the conversation, and
neither is authority: the name selects, and the person's binding is what the
run acts on behalf of. So the binding is read first — an unbound account is
refused whether or not the conversation would have chosen an agent for them.
*/
func (c *Consumer) mentioned(
	ctx context.Context, a Claimed, mapped Mapped, startable []Startable,
) (domain.UserID, Ask, Refusal, error) {
	asker, refusal, err := c.onBehalfOf(ctx, a)
	if err != nil || refusal.Why != "" {
		return "", Ask{}, refusal, err
	}

	ask, err := Read(a.Text, startable, mapped.Agent)
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
	if mapped.Agent != "" && ask.Agent != mapped.Agent {
		// Somebody reached past the binding to something else in the scope.
		// Refused rather than quietly rerouted: a name that was typed was
		// meant, and running a different agent on that sentence is the
		// confusion a binding exists to remove.
		return "", Ask{}, Refusal{
			Why: fmt.Sprintf(
				"This conversation starts %s. Mention the bot without naming another agent.",
				mapped.Agent),
			Reason: "not_this_agent",
		}, nil
	}
	return asker, ask, Refusal{}, nil
}

/*
fromMention reports that a person addressed the bot, rather than the
conversation acting on a message it was configured to watch.

Agent carries the whole distinction: the door sets it only from a watch rule,
so an empty one means nobody but the asker chose anything. Named once because
three rules turn on it, and three copies of `a.Agent == ""` is three places to
get the sense backwards.
*/
func fromMention(a Claimed) bool { return a.Agent == "" }

/*
carriesAQuestion reports that this record holds something to work on.

The words somebody typed, a run the platform itself posted about, or thread
messages that were actually read. Not merely being in a thread — that is a
position, not a question.
*/
func (r structuredAsk) carriesAQuestion() bool {
	switch {
	case strings.TrimSpace(r.Text) != "":
		return true
	case r.Subject != nil:
		return true
	default:
		return r.Thread != nil && len(r.Thread.Messages) > 0
	}
}

// startsItsOwnThread reports that nothing was said before this ask.
//
// Compared against the message and not the delivery: those are never the same
// string in a real channel, and comparing them meant the callers below never
// fired.
func startsItsOwnThread(a Claimed) bool {
	return a.Thread == "" || a.Thread == a.Message
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
	// to resolve.
	if startsItsOwnThread(a) {
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
