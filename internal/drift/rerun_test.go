package drift_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/drift"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Running the corpus on a clock.

Every pass costs a real set of model calls at a real provider, which is why
this is bounded by an interval and skips an agent whose corpus was run
recently. It is also why it only runs where there is a corpus at all: a
battery over no corrections spends money to report nothing.
*/

func TestRerun_theCorpusHasNotRunSinceTheInterval_opensABattery(t *testing.T) {
	t.Parallel()

	opener := &openerSpy{}
	rerun := drift.NewRerun(corpusOf("triage"), lastAt{"triage@v2": daysAgo(3)},
		occurrencesOf("estorno"), opener, nil).Every(24 * time.Hour)

	opened, err := rerun.Sweep(context.Background(), at())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 1 {
		t.Fatalf("batteries = %d, want one", opened)
	}
	if len(opener.opened) != 1 || opener.opened[0].Case != "estorno" {
		t.Errorf("runs = %+v, want one per case", opener.opened)
	}
	// Simulated, always. A clock that opened real runs would have the
	// platform doing the customer's work at three in the morning, unasked.
	if opener.opened[0].Simulation == "" {
		t.Error("the run does not name a simulation, so no report can find it")
	}
}

func TestRerun_ranWithinTheInterval_opensNothing(t *testing.T) {
	t.Parallel()

	opener := &openerSpy{}
	rerun := drift.NewRerun(corpusOf("triage"), lastAt{"triage@v2": daysAgo(0)},
		occurrencesOf("estorno"), opener, nil).Every(24 * time.Hour)

	if _, err := rerun.Sweep(context.Background(), at()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(opener.opened) != 0 {
		t.Errorf("runs = %+v, want none — it ran this morning", opener.opened)
	}
}

// A corpus nobody has ever run is the first battery, not a skip. Otherwise
// drift is never detectable for an agent published before this existed.
func TestRerun_neverRun_opensTheFirstBattery(t *testing.T) {
	t.Parallel()

	opener := &openerSpy{}
	rerun := drift.NewRerun(corpusOf("triage"), lastAt{},
		occurrencesOf("estorno"), opener, nil).Every(24 * time.Hour)

	if _, err := rerun.Sweep(context.Background(), at()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(opener.opened) != 1 {
		t.Errorf("runs = %+v, want the first battery", opener.opened)
	}
}

// Two workers sweep at the same minute. Both open the same battery, and the
// ledger accepts one of them — the same property the scheduler relies on.
func TestRerun_twoWorkersOnTheSameDay_askForTheSameBattery(t *testing.T) {
	t.Parallel()

	first, second := &openerSpy{}, &openerSpy{}
	for _, opener := range []*openerSpy{first, second} {
		rerun := drift.NewRerun(corpusOf("triage"), lastAt{},
			occurrencesOf("estorno"), opener, nil).Every(24 * time.Hour)
		if _, err := rerun.Sweep(context.Background(), at()); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
	}
	if first.opened[0].IdemKey != second.opened[0].IdemKey {
		t.Errorf("keys = %q and %q, want the same intention",
			first.opened[0].IdemKey, second.opened[0].IdemKey)
	}
}

// --- fixtures ---------------------------------------------------------------

func daysAgo(n int) time.Time { return at().AddDate(0, 0, -n) }

type lastAt map[string]time.Time

func (l lastAt) LastBatteryAt(
	_ context.Context, agent domain.AgentID, version domain.VersionID,
) (time.Time, bool, error) {
	at, found := l[string(agent)+"@"+string(version)]
	return at, found, nil
}

type occurrences []simulate.Occurrence

func occurrencesOf(ids ...string) occurrences {
	out := make(occurrences, 0, len(ids))
	for _, id := range ids {
		out = append(out, simulate.Occurrence{ID: id, Input: []byte(`{}`)})
	}
	return out
}

func (o occurrences) Occurrences(
	context.Context, domain.AgentID,
) ([]simulate.Occurrence, error) {
	return o, nil
}

type openerSpy struct{ opened []trigger.Request }

func (o *openerSpy) Open(_ context.Context, req trigger.Request) (trigger.Result, error) {
	o.opened = append(o.opened, req)
	return trigger.Result{RunID: domain.RunID("run-" + req.IdemKey)}, nil
}
