package channel_test

import (
	"context"
	"strings"
	"testing"
	"time"

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
	if _, err := content.Erase(t.Context(), "run-1", "test"); err != nil {
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
