package channel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Turning an ask into a run.

Everything before this wrote things down; this is where they meet. It runs
away from the request that delivered the ask, which is the whole reason the
inbox exists — a channel wants an answer in seconds and opening a run is not
something that fits in seconds.

The order is the argument. **Scope before agent, agent before ask, ask before
run.** Each step narrows what the next one may do, and each refusal is answered
in the conversation rather than logged where the person who asked will never
look: a channel that goes quiet is indistinguishable from a channel that is
broken, and the second time somebody is ignored they stop asking.
*/

// Scopes answers which scope a conversation speaks for, declared here by the
// consumer.
type Scopes interface {
	ScopeOf(ctx context.Context, channel, conversation string) (domain.Scope, error)
}

// Published lists what an ask in a scope could start.
type Published interface {
	List(ctx context.Context, scope domain.Scope, allVersions bool) ([]domain.AgentSummary, error)
}

// Willing answers whether an agent declared that a conversation may start it.
type Willing interface {
	StartableFromConversation(ctx context.Context, agent domain.AgentID, version domain.VersionID) (bool, error)
}

// Subjects resolves a reference to what the platform itself put in the
// conversation.
type Subjects interface {
	AboutRun(ctx context.Context, channel, conversation, ref string) (domain.RunID, bool, error)
}

// Opens turns an intention into a run.
type Opens interface {
	Open(ctx context.Context, req Request) (Opened, error)
}

// Request and Opened mirror the trigger package's shapes, declared here so this
// package does not depend on it — the dependency runs the other way everywhere
// else and one edge pointing back would be the cycle.
type Request struct {
	Agent   domain.AgentID
	IdemKey string
	Trigger string
	By      domain.UserID
	Input   []byte
	Labels  domain.Labels
	Origin  *domain.RunOrigin
}

// Opened is the run an ask became.
type Opened struct {
	RunID   domain.RunID
	Created bool
}

// Answers says something back where the ask was made.
type Answers interface {
	Reply(ctx context.Context, channel, conversation, thread, text string) error
}

// Consumer opens the runs that asks became.
type Consumer struct {
	inbox     *Inbox
	scopes    Scopes
	published Published
	willing   Willing
	subjects  Subjects
	opener    Opens
	answers   Answers
	bindings  func(ctx context.Context, channel, account string) (domain.UserID, bool)
	clock     func() time.Time
	owner     string
	log       *slog.Logger
}

// Ask is the structured record of what somebody asked for.
//
// This is what the ledger holds, and it is not the sentence. A run whose
// explanation a year later is *somebody typed "investiga isso aí" in a thread*
// is a screenshot, not an audit record. The sentence stays — dropping it would
// make the record dishonest — but it is filed as evidence rather than as
// instruction (NT-005 §2).
type structuredAsk struct {
	// Subject is what the ask is about, when the platform put it there itself.
	// Absent for anything else, and absent is the honest answer: an agent that
	// needs a specific alert can go and search for one, which is a tool call
	// somebody can audit rather than a guess this edge made silently.
	Subject *askSubject `json:"subject,omitempty"`
	Text    string      `json:"text"`
	AskedBy string      `json:"asked_by,omitempty"`
}

type askSubject struct {
	Kind string `json:"kind"`
	Run  string `json:"run"`
}

// NewConsumer builds one. Everything is required: a consumer missing any of
// these would refuse every ask, silently, in a way that reads as a channel
// nobody configured.
func NewConsumer(inbox *Inbox, owner string, log *slog.Logger) *Consumer {
	if log == nil {
		log = slog.Default()
	}
	return &Consumer{inbox: inbox, owner: owner, clock: time.Now, log: log}
}

// With wires what the consumer reads and what it can do.
func (c *Consumer) With(
	scopes Scopes, published Published, willing Willing,
	subjects Subjects, opener Opens, answers Answers,
) *Consumer {
	c.scopes, c.published, c.willing = scopes, published, willing
	c.subjects, c.opener, c.answers = subjects, opener, answers
	return c
}

// Binding wires who a channel account speaks for.
//
// A run acts on somebody's behalf, and an ask from an account nobody bound
// speaks for nobody: refused, and refused by name, because "something went
// wrong" would send an operator to debug a platform working exactly as
// intended.
func (c *Consumer) Binding(
	f func(ctx context.Context, channel, account string) (domain.UserID, bool),
) *Consumer {
	c.bindings = f
	return c
}

// ErrNotWired means a consumer is missing something it cannot work without.
//
// Reported rather than left to panic in a worker sweep. A nil dereference in a
// background loop is a crash with a stack trace pointing at the line that
// found the gap, not at the wiring that left it.
var ErrNotWired = errors.New("channel: the consumer is missing a dependency")

// Sweep opens what has arrived, and answers what it cannot.
func (c *Consumer) Sweep(ctx context.Context, lease time.Duration, limit int) (int, error) {
	if err := c.wired(); err != nil {
		return 0, err
	}

	claimed, err := c.inbox.Claim(ctx, c.owner, lease, limit)
	if err != nil {
		return 0, err
	}

	opened, failures := 0, []error{}
	for _, one := range claimed {
		did, err := c.handle(ctx, one)
		if err != nil {
			// One ask that could not be settled must not stop the rest: a
			// conversation nobody can reach would otherwise silence every
			// other conversation on the installation.
			failures = append(failures, err)
			continue
		}
		if did {
			opened++
		}
	}
	return opened, errors.Join(failures...)
}

/*
handle takes one ask as far as it can go and records what became of it.

It says nothing. A refusal is recorded here — which only the holder of the ask
can do — and delivering it is claimed separately by Answer, because a reply
sent while deciding goes out before ownership is proven.

It is still answered. The person who asked is the one who needs to know, and a
channel that goes quiet is indistinguishable from a channel that is broken: the
second time somebody is ignored they stop asking. What changed is who says it
and when, not whether.
*/
func (c *Consumer) handle(ctx context.Context, claimed Claimed) (opened bool, err error) {
	run, why := c.open(ctx, claimed)
	if why != "" {
		return false, c.decline(ctx, claimed, why)
	}
	if err := c.inbox.Opened(ctx, claimed, string(run), c.clock()); err != nil {
		// The run exists and the inbox does not know. Reported rather than
		// swallowed: the next sweep would claim the ask again, and the opener
		// gives back the same run for the same key rather than a second one —
		// which is what makes this recoverable instead of duplicating.
		return false, err
	}
	return true, nil
}

/*
open takes an ask through the narrowing and returns the run, or why not.

Scope before agent, agent before ask, ask before run. Each step bounds what the
next may do, and none of them consults the text for anything the platform is
supposed to decide: the scope comes from the conversation, never from what
somebody wrote in it.
*/
func (c *Consumer) open(ctx context.Context, a Claimed) (domain.RunID, string) {
	scope, err := c.scopes.ScopeOf(ctx, a.Channel, a.Conversation)
	if err != nil {
		// Including ambiguity, which is refused rather than resolved. The
		// person is told plainly: this conversation is not set up to start
		// anything, which is an operator's job and not theirs.
		c.log.Warn("an ask arrived in a conversation that speaks for no scope",
			"channel", a.Channel, "conversation", a.Conversation, "err", err)
		return "", "This conversation is not set up to start agents. An administrator maps it to an area."
	}

	asker, bound := c.bindings(ctx, a.Channel, a.AskedBy)
	if !bound {
		// A run acts on somebody's behalf. An account nobody bound speaks for
		// nobody, and running as nobody is how an ask acquires authority that
		// no person holds.
		return "", "Your channel account is not linked to a platform user, so nothing can run on your behalf."
	}

	startable, err := c.startable(ctx, scope)
	if err != nil {
		c.log.Error("could not read what is startable", "scope", scope, "err", err)
		return "", "Something on this side failed while reading which agents are available."
	}

	ask, err := Read(a.Text, startable)
	if err != nil {
		// The refusal already names what would have worked. It is the only
		// teaching surface a channel has.
		return "", err.Error()
	}

	input, err := json.Marshal(c.structured(ctx, a, ask, asker))
	if err != nil {
		return "", "Something on this side failed while recording the ask."
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
	if err != nil {
		c.log.Warn("an ask did not open a run",
			"agent", ask.Agent, "channel", a.Channel, "err", err)
		return "", fmt.Sprintf("%s could not be started: %v", ask.Agent, err)
	}
	return opened.RunID, ""
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
decline records why an ask became nothing. Saying it is separate work.

Both orderings of "record" and "reply" are wrong, which is what took three
attempts to see. Record first and a driver failure closes the ask with nobody
told. Reply first and the message goes out before ownership is proven, so a
worker whose lease lapsed posts a refusal the worker that replaced it posts
again — the duplicated attention the claim exists to prevent, one step earlier.

So this does the half that proves ownership, and leaves a debt. The debt is
claimed and delivered on its own, which is the only arrangement where neither
rule has to give.
*/
func (c *Consumer) decline(ctx context.Context, a Claimed, why string) error {
	return c.inbox.Refused(ctx, a, why, c.clock())
}

/*
Answer says the refusals that were recorded and not yet delivered.

Its own claim, so two consumers do not both say it, and its own retry, so a
driver that was away does not turn a refusal into silence. A reply repeated
because the process died between saying and recording is the failure this
accepts, and it is the one that is noise — the same choice the delivery table
makes for approval requests.
*/
func (c *Consumer) Answer(ctx context.Context, lease time.Duration, limit int) (int, error) {
	if err := c.wired(); err != nil {
		return 0, err
	}

	owed, err := c.inbox.Owed(ctx, c.owner, lease, limit)
	if err != nil {
		return 0, err
	}

	said, failures := 0, []error{}
	for _, one := range owed {
		if err := c.answers.Reply(ctx, one.Channel, one.Conversation, one.Thread, one.Detail); err != nil {
			// Left owed, so somebody tries again. The person has not been
			// told and that is exactly the failure worth retrying.
			failures = append(failures, fmt.Errorf("channel: answer %s: %w", one.EventID, err))
			continue
		}
		if err := c.inbox.Answered(ctx, one, c.clock()); err != nil {
			failures = append(failures, err)
			continue
		}
		said++
	}
	return said, errors.Join(failures...)
}

/*
AskKey names the intention an ask represents, for idempotency.

Hashed rather than joined. A channel is a name somebody typed into a form and a
conversation id is a vendor's, so a separator that appears in either makes two
different asks produce one key — and the run that key names is somebody else's.
The digest cannot collide by punctuation, and the parts are length-prefixed so
they cannot collide by rearrangement either.
*/
func AskKey(a Arrival) string {
	sum := sha256.New()
	for _, part := range []string{a.Channel, a.Conversation, a.EventID} {
		fmt.Fprintf(sum, "%d:%s", len(part), part)
	}
	return "channel:" + hex.EncodeToString(sum.Sum(nil)[:16])
}

// wired names what is missing, all of it, rather than failing on the first gap.
//
// An operator fixing configuration one error at a time is an operator
// restarting a worker five times to learn five things.
func (c *Consumer) wired() error {
	missing := []string{}
	for name, present := range map[string]bool{
		"an inbox":            c.inbox != nil,
		"a scope map":         c.scopes != nil,
		"a published listing": c.published != nil,
		"a willingness check": c.willing != nil,
		"a subject resolver":  c.subjects != nil,
		"an opener":           c.opener != nil,
		"a way to answer":     c.answers != nil,
		"an account binding":  c.bindings != nil,
	} {
		if !present {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("%w: %s", ErrNotWired, strings.Join(missing, ", "))
}
