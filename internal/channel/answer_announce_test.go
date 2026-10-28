package channel_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

/*
A finished run nobody asked for carries its own answer into the announcement.

The card used to be all a scheduled agent ever produced: "finished", and a
link. The person the report was written for opened Slack, saw that a run ended,
and had to leave for the console to read the one sentence the agent had to say.
A conversation that ticked Finished has already said where the answer should
go; what was missing was the answer travelling.

A run somebody asked for is the deliberate exception: the ask path already says
the answer in the thread that asked, and an announcement carrying it again
would say everything twice.
*/
func TestSweep_aFinishedRunNobodyAskedFor_announcesTheAnswerItself(t *testing.T) {
	t.Parallel()
	for _, one := range []struct {
		name  string
		asked bool
		want  string
	}{
		{"a scheduled run carries its answer", false, "tudo normal"},
		{"an asked run stays a card", true, ""},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			posts := &recorder{}
			finished := report("run-1", "acme", "ops", channel.EventFinished)
			finished.Asked = one.asked
			r := channel.NewReporter(
				&fixedReports{reports: []channel.Report{finished}},
				rooms(roomWanting("C07-ops", channel.EventFinished)),
				posts, func() time.Time { return noon }, nil,
			).WithDeliveries(&memoryDeliveries{}).
				WithOutcomes(outcomesSaying(t, "run-1", "tudo normal"))

			if _, err := r.Sweep(t.Context(), 50); err != nil {
				t.Fatalf("sweep: %v", err)
			}
			if len(posts.sent) != 1 {
				t.Fatalf("sent = %+v, want one announcement", posts.sent)
			}
			if got := posts.sent[0].message.Answer; got != one.want {
				t.Errorf("Answer = %q, want %q", got, one.want)
			}
		})
	}
}

// A reporter nobody gave the outcome ports to announces exactly what it always
// announced. That is what an installation running an older worker gets, and it
// must be the card, not an error and not silence.
func TestSweep_withoutOutcomePorts_theCardIsUnchanged(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{
			report("run-1", "acme", "ops", channel.EventFinished),
		}},
		rooms(roomWanting("C07-ops", channel.EventFinished)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{})

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 1 || posts.sent[0].message.Answer != "" {
		t.Fatalf("sent = %+v, want the plain card", posts.sent)
	}
}

/*
An answer past the block ceiling is cut, says so, and still links.

Slack refuses a section past its own limit, so an uncut answer would not be a
long message — it would be no message, failing every hour for as long as the
answer stays long. The link is the recovery: the console has the whole text.
*/
func TestSweep_anAnswerPastTheCeiling_isTruncatedAndSaysSo(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	finished := report("run-1", "acme", "ops", channel.EventFinished)
	long := strings.Repeat("x", channel.MaxAnnouncedAnswer+100)
	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{finished}},
		rooms(roomWanting("C07-ops", channel.EventFinished)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).
		WithOutcomes(outcomesSaying(t, "run-1", long))

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	got := posts.sent[0].message.Answer
	if len(got) > channel.MaxAnnouncedAnswer {
		t.Fatalf("announced %d bytes, ceiling is %d", len(got), channel.MaxAnnouncedAnswer)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("a cut answer does not say it was cut: %q", got[len(got)-80:])
	}
}

// An erased outcome is said to have been erased — the sentence the ask path
// already uses — rather than failing the sweep or announcing nothing.
func TestSweep_anErasedOutcome_saysSo(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	finished := report("run-1", "acme", "ops", channel.EventFinished)
	content := engine.NewMemoryContent()
	ref, err := content.Put(t.Context(), "run-1", 1, []byte("was here"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := content.Erase(t.Context(), "run", "run-1", "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{finished}},
		rooms(roomWanting("C07-ops", channel.EventFinished)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).
		WithOutcomes(fixedOutcomes{run: "run-1",
			payload: domain.RunFinishedPayload{OutcomeRef: ref}}, content)

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := posts.sent[0].message.Answer; !strings.Contains(got, "erased") {
		t.Errorf("Answer = %q, want the erasure said", got)
	}
}

// roomWanting is one conversation subscribed to one event.
func roomWanting(id string, ev channel.Event) channel.Conversation {
	return channel.Conversation{
		Channel: "acme-slack", ID: id, Label: "#" + id,
		Wants: []channel.Event{ev},
	}
}

// outcomesSaying answers one run's closing text through a real content store,
// so the reference resolution is the production one rather than a shortcut.
/*
The agent declares, the reporter decides (#165): a finished answer carrying a
headline posts as two messages — the headline at the channel's top level and
the full answer threaded on it — so an hourly report is one scannable line,
not wallpaper. The delivery stays the channel layer's: same bot, same rooms,
same tracking; the agent only formatted.
*/
func TestSweep_aHeadlinedAnswer_postsTheHeadlineAndThreadsTheAnswer(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	finished := report("run-1", "acme", "ops", channel.EventFinished)
	outcomes, content := outcomesWithHeadline(t, "run-1",
		"o relatório inteiro, achado por achado", "2 achados · pior: CRÍTICO")

	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{finished}},
		rooms(roomWanting("C07-ops", channel.EventFinished)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).WithOutcomes(outcomes, content)

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 2 {
		t.Fatalf("sent = %d posts, want the headline and its threaded answer", len(posts.sent))
	}
	header, body := posts.sent[0], posts.sent[1]
	if header.message.Answer != "2 achados · pior: CRÍTICO" || header.conversation.Thread != "" {
		t.Errorf("header = %q in thread %q, want the headline at the top level",
			header.message.Answer, header.conversation.Thread)
	}
	if body.message.Answer != "o relatório inteiro, achado por achado" {
		t.Errorf("body answer = %q, want the full report", body.message.Answer)
	}
	if body.conversation.Thread != "1786000000.000100" {
		t.Errorf("body thread = %q, want the header's own timestamp", body.conversation.Thread)
	}
}

// The reporter bounds the headline itself: the finish guard caps new runs,
// but a line stored by anything older or other arrives here unbounded, and a
// channel refusing an oversized block would leave the announcement owed and
// retried for ever — the permanent loop this ceiling exists to prevent.
func TestSweep_anOversizedStoredHeadline_isBoundedBeforeTheChannel(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	finished := report("run-1", "acme", "ops", channel.EventFinished)
	outcomes, content := outcomesWithHeadline(t, "run-1",
		"o relatório", strings.Repeat("ã", 2000))

	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{finished}},
		rooms(roomWanting("C07-ops", channel.EventFinished)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).WithOutcomes(outcomes, content)

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 2 {
		t.Fatalf("sent = %d posts, want the bounded headline and its thread", len(posts.sent))
	}
	header := posts.sent[0].message.Answer
	if len(header) > channel.MaxAnnouncedHeadline {
		t.Fatalf("header carried %d bytes, want at most %d", len(header), channel.MaxAnnouncedHeadline)
	}
	if !utf8.ValidString(header) {
		t.Fatal("the bound cut through a rune")
	}
}

// A failure after the headline posted leaves the announcement unrecorded, so
// the sweep retries it whole. A repeated one-liner is the chosen cost; a
// report that never arrives is the one this path must not have.
func TestSweep_aThreadReplyThatFails_leavesTheAnnouncementOwed(t *testing.T) {
	t.Parallel()
	posts := &recorder{failAfter: 1}
	finished := report("run-1", "acme", "ops", channel.EventFinished)
	outcomes, content := outcomesWithHeadline(t, "run-1", "o relatório", "1 achado")
	deliveries := &memoryDeliveries{}

	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{finished}},
		rooms(roomWanting("C07-ops", channel.EventFinished)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(deliveries).WithOutcomes(outcomes, content)

	if _, err := r.Sweep(t.Context(), 50); err == nil {
		t.Fatal("sweep: want the refused thread reply surfaced as an error")
	}
	if len(deliveries.recorded) != 0 {
		t.Errorf("recorded = %+v, want the half-delivered announcement still owed", deliveries.recorded)
	}
	if len(deliveries.failures) == 0 {
		t.Error("no delivery failure was recorded for the refused reply")
	}
}

func outcomesWithHeadline(
	t *testing.T, run, text, headline string,
) (fixedOutcomes, engine.ContentStore) {
	t.Helper()
	content := engine.NewMemoryContent()
	ref, err := content.Put(t.Context(), domain.RunID(run), 1, []byte(text))
	if err != nil {
		t.Fatalf("put outcome: %v", err)
	}
	headlineRef, err := content.Put(t.Context(), domain.RunID(run), 1, []byte(headline))
	if err != nil {
		t.Fatalf("put headline: %v", err)
	}
	return fixedOutcomes{run: domain.RunID(run), payload: domain.RunFinishedPayload{
		OutcomeRef: ref, HeadlineRef: headlineRef,
	}}, content
}

func outcomesSaying(t *testing.T, run string, text string) (fixedOutcomes, engine.ContentStore) {
	t.Helper()
	content := engine.NewMemoryContent()
	ref, err := content.Put(t.Context(), domain.RunID(run), 1, []byte(text))
	if err != nil {
		t.Fatalf("put outcome: %v", err)
	}
	return fixedOutcomes{run: domain.RunID(run), payload: domain.RunFinishedPayload{OutcomeRef: ref}}, content
}

type fixedOutcomes struct {
	run     domain.RunID
	payload domain.RunFinishedPayload
}

func (f fixedOutcomes) FinishedOutcome(_ context.Context, run domain.RunID) (domain.RunFinishedPayload, error) {
	if run != f.run {
		return domain.RunFinishedPayload{}, domain.ErrContentErased
	}
	return f.payload, nil
}

/*
A conversation may hear only the finishes that have something to say.

With Finished ticked, every scheduled run posted something — and a run whose
agent wrote no closing text posted the card, once an hour, forever. The option
is the middle ground somebody asked for after muting the channel: the agent's
instruction decides what is worth a message, and a finish with no answer is
heard by nobody.

Chosen silence is not a failure: the run is still retired from the sweep, or a
report nobody will ever post would sit at the front of every page for a day.
*/
func TestSweep_finishedAnswerOnly_silenceIsChosenAndTheRunIsRetired(t *testing.T) {
	t.Parallel()
	for _, one := range []struct {
		name   string
		answer string
		posted int
	}{
		{"a finish with no answer is heard by nobody", "", 0},
		{"a finish that answers is posted", "1 coisa para olhar", 1},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			posts := &recorder{}
			reports := &fixedReports{reports: []channel.Report{
				report("run-1", "acme", "ops", channel.EventFinished),
			}}
			place := roomWanting("C07-ops", channel.EventFinished)
			place.FinishedAnswerOnly = true
			r := channel.NewReporter(
				reports, rooms(place), posts,
				func() time.Time { return noon }, nil,
			).WithDeliveries(&memoryDeliveries{})
			if one.answer != "" {
				r = r.WithOutcomes(outcomesSaying(t, "run-1", one.answer))
			} else {
				r = r.WithOutcomes(outcomesSaying(t, "run-other", "not this run"))
			}

			if _, err := r.Sweep(t.Context(), 50); err != nil {
				t.Fatalf("sweep: %v", err)
			}
			if len(posts.sent) != one.posted {
				t.Fatalf("sent = %+v, want %d messages", posts.sent, one.posted)
			}
			// Retired either way: silence was chosen, not failed.
			if len(reports.done) != 1 {
				t.Fatalf("reported = %v, want the run retired from the sweep", reports.done)
			}
		})
	}
}

// The option is about finished and nothing else: a parked run still demands
// attention whether or not it has words of its own.
func TestSweep_finishedAnswerOnly_aParkedRunStillPosts(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	place := roomWanting("C07-ops", channel.EventParked)
	place.FinishedAnswerOnly = true
	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{
			report("run-1", "acme", "ops", channel.EventParked),
		}},
		rooms(place), posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{})

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 1 {
		t.Fatalf("sent = %+v, want the parked card", posts.sent)
	}
}

// Two conversations hearing one run resolve its outcome once: the message is
// built per report, not per place.
func TestSweep_twoConversations_resolveTheOutcomeOnce(t *testing.T) {
	t.Parallel()
	posts := &recorder{}
	outcomes, content := outcomesSaying(t, "run-1", "tudo normal")
	counting := &countingOutcomes{inner: outcomes}
	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{
			report("run-1", "acme", "ops", channel.EventFinished),
		}},
		rooms(roomWanting("C07", channel.EventFinished), roomWanting("C08", channel.EventFinished)),
		posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).WithOutcomes(counting, content)

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 2 {
		t.Fatalf("sent = %d, want both conversations told", len(posts.sent))
	}
	if counting.calls != 1 {
		t.Errorf("FinishedOutcome called %d times for one run, want once", counting.calls)
	}
}

type countingOutcomes struct {
	inner channel.Outcomes
	calls int
}

func (c *countingOutcomes) FinishedOutcome(ctx context.Context, run domain.RunID) (domain.RunFinishedPayload, error) {
	c.calls++
	return c.inner.FinishedOutcome(ctx, run)
}
