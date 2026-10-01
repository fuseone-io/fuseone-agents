package channel_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
)

// The exact scenario: a run somebody opened by talking to the bot. Its answer
// already lives in the thread that asked, so with the quiet option on, the
// channel hears nothing about it at all — no card, no text.
func TestSweep_quietOption_aRunOpenedByInteraction_announcesNothing(t *testing.T) {
	posts := &recorder{}
	finished := report("run-1", "acme", "ops", channel.EventFinished)
	finished.Asked = true
	place := roomWanting("C07-ops", channel.EventFinished)
	place.FinishedAnswerOnly = true
	reports := &fixedReports{reports: []channel.Report{finished}}
	r := channel.NewReporter(reports, rooms(place), posts,
		func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).
		WithOutcomes(outcomesSaying(t, "run-1", "an answer that already lives in the thread"))

	if _, err := r.Sweep(t.Context(), 50); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent = %+v, want silence for an interaction-opened run", posts.sent)
	}
	if len(reports.done) != 1 {
		t.Fatal("the run was not retired")
	}
}
