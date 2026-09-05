package channel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
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

// Consumer turns the asks an inbox holds into runs, or into refusals said back
// where they were asked.
type Consumer struct {
	inbox        *Inbox
	mapping      Mapping
	published    Published
	willing      Willing
	subjects     Subjects
	threadPolicy ThreadContextPolicy
	threads      ThreadReader
	opener       Opens
	answers      Answers
	outcomes     Outcomes
	content      engine.ContentStore
	bindings     func(ctx context.Context, channel, account string) (domain.UserID, bool, error)
	ceiling      Ceiling
	clock        func() time.Time
	owner        string
	log          *slog.Logger
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
	Subject *askSubject    `json:"subject,omitempty"`
	Text    string         `json:"text"`
	AskedBy string         `json:"asked_by,omitempty"`
	Source  string         `json:"source,omitempty"`
	Thread  *ThreadContext `json:"thread,omitempty"`
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
	return &Consumer{
		inbox: inbox, owner: owner,
		ceiling: DefaultCeiling, clock: time.Now, log: log,
	}
}

// With wires what the consumer reads and what it can do.
func (c *Consumer) With(
	mapping Mapping, published Published, willing Willing,
	subjects Subjects, opener Opens, answers Answers,
) *Consumer {
	c.mapping, c.published, c.willing = mapping, published, willing
	c.subjects, c.opener, c.answers = subjects, opener, answers
	return c
}

// WithOutcomes wires the store that can read the final answer of a run that
// began in a channel. Separate from With because opening and refusing asks can
// work without it; only the success path needs to read content back.
func (c *Consumer) WithOutcomes(outcomes Outcomes, content engine.ContentStore) *Consumer {
	c.outcomes, c.content = outcomes, content
	return c
}

// WithThreadContext wires optional retrieval of evidence from a channel
// thread. Optional because most conversations do not ask for it; explicit
// because reading surrounding Slack text is a different decision from allowing
// a mention to start a run.
func (c *Consumer) WithThreadContext(policy ThreadContextPolicy, reader ThreadReader) *Consumer {
	c.threadPolicy, c.threads = policy, reader
	return c
}

// Binding wires who a channel account speaks for.
//
// A run acts on somebody's behalf, and an ask from an account nobody bound
// speaks for nobody: refused, and refused by name, because "something went
// wrong" would send an operator to debug a platform working exactly as
// intended.
func (c *Consumer) Binding(
	f func(ctx context.Context, channel, account string) (domain.UserID, bool, error),
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
	run, refusal, err := c.open(ctx, claimed)
	switch {
	case err != nil:
		// Left pending. Something on this side failed and the ask is still a
		// good ask: closing it would tell somebody their question was refused
		// when what happened is that a database was away.
		return false, err
	case refusal.Why != "":
		return false, c.decline(ctx, claimed, refusal)
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
func (c *Consumer) decline(ctx context.Context, a Claimed, r Refusal) error {
	return c.inbox.Refused(ctx, a, r, c.clock())
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
AnswerFinished says the final answer of runs that began in a conversation.

This is deliberately not the reporter's "finished" announcement. The reporter
says that a run ended and links to the console; this says the answer itself in
the thread that asked. Only asks that opened a run are eligible, so the platform
does not invent recipients for scheduled, manual or event-composed runs.
*/
func (c *Consumer) AnswerFinished(ctx context.Context, lease time.Duration, limit int) (int, error) {
	if err := c.wiredForFinishedAnswers(); err != nil {
		return 0, err
	}

	owed, err := c.inbox.Finished(ctx, c.owner, lease, limit)
	if err != nil {
		return 0, err
	}

	said, failures := 0, []error{}
	for _, one := range owed {
		payload, err := c.outcomes.FinishedOutcome(ctx, one.RunID)
		if err != nil {
			failures = append(failures, fmt.Errorf("channel: read finished answer for %s: %w", one.RunID, err))
			continue
		}

		text, err := engine.OutcomeOf(ctx, c.content, payload)
		if err != nil {
			if errors.Is(err, domain.ErrContentErased) {
				text = "The agent finished, but its closing answer was erased by retention or a data erasure request."
			} else {
				failures = append(failures, fmt.Errorf("channel: resolve finished answer for %s: %w", one.RunID, err))
				continue
			}
		}

		if text != "" {
			if err := c.answers.ReplyOutcome(ctx, one.Channel, one.Conversation, one.Thread, text); err != nil {
				// Left owed, so somebody tries again. A finished run whose
				// answer never appears in the conversation reads as silence,
				// which is the failure this sweep exists to avoid.
				failures = append(failures, fmt.Errorf("channel: answer %s: %w", one.EventID, err))
				continue
			}
			said++
		}
		if err := c.inbox.FinishedAnswered(ctx, one, c.clock()); err != nil {
			failures = append(failures, err)
			continue
		}
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
		"a conversation map":  c.mapping != nil,
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

func (c *Consumer) wiredForFinishedAnswers() error {
	if err := c.wired(); err != nil {
		return err
	}
	missing := []string{}
	if c.outcomes == nil {
		missing = append(missing, "a finished-run reader")
	}
	if c.content == nil {
		missing = append(missing, "a content store")
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("%w: %s", ErrNotWired, strings.Join(missing, ", "))
}
