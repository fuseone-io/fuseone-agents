package channel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

/*
Reporter says what has happened, once.

A sweep rather than a hook on the run that parked, for the reason the event
dispatcher is one too: a worker that died between parking a run and announcing
it would drop the announcement, and a sweep that runs again cannot.

Delivery is recorded after the message leaves and never before. The boundary
offers no idempotency key, so exactly-once is not for sale at any price;
between the two failures available this takes the one that is noise. A repeated
approval request in a channel is an irritation. An approval nobody ever saw is
the thing this stage exists to prevent.
*/
type Reporter struct {
	reports       Reports
	conversations Conversations
	poster        Poster
	deliveries    Deliveries
	approvers     Approvers
	accounts      Accounts
	approvals     Approvals
	connections   Connections
	tickets       ticket.ApprovalRoutes
	outcomes      Outcomes
	content       engine.ContentStore
	clock         func() time.Time
	baseURL       string
	log           *slog.Logger
}

// Window is how far back a first sweep looks.
//
// Bounded so that configuring a conversation does not replay a year of runs
// into it, and generous enough that a process away for the afternoon still
// says what it missed.
const Window = 24 * time.Hour

func NewReporter(
	reports Reports, conversations Conversations, poster Poster,
	clock func() time.Time, log *slog.Logger,
) *Reporter {
	if log == nil {
		log = slog.Default()
	}
	return &Reporter{
		reports: reports, conversations: conversations,
		poster: poster, clock: clock, log: log,
		deliveries: noDeliveries{},
	}
}

// Sweep says what has not been said, and answers how many messages left.
//
// One unreachable conversation does not silence the others: a workspace the
// bot was removed from would otherwise stop every area's notifications, and
// the failure would show up as silence, which is the hardest thing to notice.
func (r *Reporter) Sweep(ctx context.Context, limit int) (int, error) {
	pending, err := r.reports.Unreported(ctx, r.clock().Add(-Window), limit)
	if err != nil {
		return 0, fmt.Errorf("channel: read what is unreported: %w", err)
	}

	// One fan-out for the pass. Who may decide and where they are reachable
	// are asked once and remembered: both are configuration, and re-reading
	// them between reports would let the set of people one sweep messages
	// change halfway through the sweep.
	pass := r.newFanout()
	sent, failures := 0, []error{}
	for _, report := range pending {
		n, told, err := r.announce(ctx, pass, report)
		sent += n
		if err != nil {
			// Left unreported on purpose. The next sweep tries the
			// conversations that did not hear, and the ones that did are
			// skipped at the post rather than told twice.
			failures = append(failures, err)
			continue
		}
		if told == 0 {
			// Nowhere to say it. Marking it said would spend the window on
			// silence: the whole reason the window exists is that turning a
			// conversation on replays the last day into it, and a run marked
			// here is a run that conversation never hears about.
			continue
		}
		if err := r.reports.Reported(ctx, report, r.clock()); err != nil {
			failures = append(failures, err)
		}
	}
	return sent, errors.Join(failures...)
}

// announce tells every conversation that speaks for the run's scope and wants
// to hear about this.
//
// It answers how many messages left and how many conversations were owed one
// at all — which are different questions. Nothing sent because everybody had
// already heard is finished; nothing sent because nobody was listening is not.
func (r *Reporter) announce(
	ctx context.Context, pass *fanout, report Report,
) (sent, told int, err error) {
	places, err := r.conversations.For(ctx, report.Scope)
	if err != nil {
		err = WrapError(
			CodeConfigurationReadFailed,
			fmt.Errorf("channel: conversations for %s: %w", report.Scope, err),
		)
		return 0, 0, errors.Join(err, r.recordFailures(ctx, r.failuresFor(report, Conversation{}, err)))
	}
	places, err = pass.ticketPlaces(ctx, report, places)
	if errors.Is(err, errReviewPending) {
		// Owed and not yet sayable. Left unreported with nothing recorded
		// against it: the room's thread is being opened by another sweep, and
		// a failure row here would describe a configuration problem that is
		// not one.
		return 0, 0, nil
	}
	if err != nil {
		err = WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: ticket approval route: %w", err))
		return 0, 0, errors.Join(err,
			r.recordFailures(ctx, r.failuresFor(report, Conversation{}, err)))
	}

	failures := []error{}
	deliveryFailures := []DeliveryFailure{}
	for _, place := range places {
		if !place.wants(report.Event) || !place.reportsAgent(report.AgentID) {
			continue
		}
		// Owed a message, whether or not one goes out now: a conversation
		// that already heard is one this run has finished with.
		told++

		n, refused := r.tell(ctx, pass, report, place)
		sent += n
		failures = append(failures, refused.blocking...)
		deliveryFailures = append(deliveryFailures, refused.recorded...)
	}
	// The other beginning: the agent's own word, needing no room at all. After
	// the rooms, because a card in a channel is what somebody else can see, and
	// a private message is not a replacement for it where both were asked for.
	privately, owed, refusedPrivately := r.directForOwner(ctx, pass, report, places)
	sent += privately
	told += owed
	failures = append(failures, refusedPrivately.blocking...)
	deliveryFailures = append(deliveryFailures, refusedPrivately.recorded...)

	/*
		Nobody was told and nothing above said why.

		A run with no destination at all — no conversation covers its scope and
		its agent asked for nothing — is kept, because marking it announced
		would spend the window on silence: configuring a conversation replays
		the last day into it, and a run marked here is one that conversation
		never hears about.

		Kept and never written down, it is "never tried" for ever, and the sweep
		takes what has not been tried first. So a handful of runs nobody can be
		told about sit at the front of every page, and an older run that *could*
		be told is never reached before it leaves the window. This is the same
		starvation the per-cause records fixed, arriving through the exit that
		is by far the most common: an installation part-way through being
		configured.

		Recorded once for the whole announcement, and only when nothing else
		explained it, so a run that already said why nobody heard is not
		counted twice.
	*/
	if told == 0 && len(deliveryFailures) == 0 {
		deliveryFailures = append(deliveryFailures, r.failuresFor(report, Conversation{},
			NewError(CodeNowhereToSayIt,
				"channel: nothing is configured to hear about this run"))...)
	}

	if err := r.recordFailures(ctx, deliveryFailures); err != nil {
		failures = append(failures, err)
	}
	return sent, told, errors.Join(failures...)
}

func (r *Reporter) failuresFor(report Report, place Conversation, cause error) []DeliveryFailure {
	var failures []DeliveryFailure
	for _, code := range FailureCodes(cause) {
		failures = append(failures, DeliveryFailure{
			Announcement: report.AnnouncementTo(place),
			// About the scope rather than about a room, whenever no room was
			// involved — a connection with nobody bound on it is not one
			// conversation affected, and counting it as one tells the cockpit
			// a conversation was there.
			ScopeWide: place.ID == "",
			Code:      code, Scope: report.Scope, AgentID: report.AgentID,
			SeenAt: r.clock(),
		})
	}
	return failures
}

func (r *Reporter) recordFailures(ctx context.Context, failures []DeliveryFailure) error {
	if len(failures) == 0 {
		return nil
	}
	if err := r.deliveries.RecordFailures(ctx, failures); err != nil {
		return fmt.Errorf("channel: record delivery failures: %w", err)
	}
	return nil
}

// post sends one message, unless it has already been sent.
func (r *Reporter) post(ctx context.Context, report Report, place Conversation) (bool, error) {
	owed := report.AnnouncementTo(place)
	said, err := r.deliveries.Delivered(ctx, owed)
	if err != nil {
		return false, fmt.Errorf("channel: read deliveries: %w", err)
	}
	if said {
		return false, nil
	}

	at, err := placed(ctx, r.poster, place, r.message(ctx, report))
	if err != nil {
		// Named, because the ordinary cause is a bot removed from one channel
		// and the symptom is silence in that channel alone.
		return false, fmt.Errorf("channel: post to %s: %w", place.Label, err)
	}

	return true, r.deliveries.Record(ctx, Delivery{
		Announcement: owed, Ref: at.Ref, Placed: at.Conversation, PostedAt: r.clock(),
	})
}

func (r *Reporter) message(ctx context.Context, report Report) Message {
	m := Message{
		Event: report.Event, RunID: report.RunID, Agent: report.AgentID,
		Scope: report.Scope, Reason: report.Reason, Tool: report.Tool,
		AtSeq: report.AtSeq, AwaitingDecision: report.AwaitingDecision,
	}
	if r.baseURL != "" {
		m.Link = fmt.Sprintf("%s/runs/%s", r.baseURL, report.RunID)
	}
	m.Answer = r.answerFor(ctx, report)
	return m
}

/*
answerFor is the run's own closing text, when this announcement should carry it.

Only a finished run, and only one nobody asked for: the ask path already says
the answer in the thread that asked, and carrying it here as well would say
everything twice. A reporter without the outcome ports announces the card it
always announced, which is what an installation on an older worker gets.

A failure to read the outcome degrades to the card rather than blocking the
announcement: the person still learns the run finished, and the link still
leads to the whole answer. Erasure is the exception with words of its own —
the sentence the ask path already uses — because "the platform removed this"
and "nothing was said" must not read the same.
*/
func (r *Reporter) answerFor(ctx context.Context, report Report) string {
	if report.Event != EventFinished || report.Asked || r.outcomes == nil {
		return ""
	}
	payload, err := r.outcomes.FinishedOutcome(ctx, report.RunID)
	if err != nil {
		return ""
	}
	text, err := engine.OutcomeOf(ctx, r.content, payload)
	if err != nil {
		if errors.Is(err, domain.ErrContentErased) {
			return erasedAnswer
		}
		return ""
	}
	return truncateAnswer(text)
}

// MaxAnnouncedAnswer bounds what an announcement carries. Slack refuses a
// section past its own 3000-character limit, so an uncut answer would not be a
// long message — it would be no message, failing every hour for as long as the
// answer stays long. The margin below the vendor limit leaves room for the
// truncation notice and mrkdwn escaping.
const MaxAnnouncedAnswer = 2800

const erasedAnswer = "The agent finished, but its closing answer was erased by retention or a data erasure request."

const truncationNotice = "\n\n_(truncated — the full answer is in the console)_"

func truncateAnswer(text string) string {
	if len(text) <= MaxAnnouncedAnswer {
		return text
	}
	cut := MaxAnnouncedAnswer - len(truncationNotice)
	// Never split a rune: a cut through a multibyte character renders as
	// garbage in the one place a person reads.
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + truncationNotice
}

// noDeliveries remembers nothing, which makes every sweep repeat itself. It is
// the zero value rather than a nil check so a Reporter built without a store
// fails loudly in a channel instead of panicking in a worker.
type noDeliveries struct{}

func (noDeliveries) Record(context.Context, Delivery) error { return nil }
func (noDeliveries) RecordFailure(context.Context, DeliveryFailure) error {
	return nil
}
func (noDeliveries) RecordFailures(context.Context, []DeliveryFailure) error {
	return nil
}
func (noDeliveries) Delivered(context.Context, Announcement) (bool, error) {
	return false, nil
}
