package trigger_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
One agent publishes a typed event, another consumes it as a trigger. Neither
names the other, and the graph is a fact about the definitions rather than
about what a model decided today (PRD SE-10).
*/

type wiring struct {
	emits  map[domain.AgentID][]string
	listen map[string][]domain.AgentID
}

func (w wiring) Emitters(context.Context) (map[domain.AgentID][]string, error) {
	return w.emits, nil
}

func (w wiring) Listeners(context.Context) (map[string][]domain.AgentID, error) {
	return w.listen, nil
}

type finished []domain.RunSummary

func (f finished) ListRuns(context.Context, domain.RunFilter, string, int) ([]domain.RunSummary, error) {
	return f, nil
}

// dispatcherFor wires a triage agent that emits, and a billing agent that
// listens. Both are published, both may run.
func dispatcherFor(t *testing.T, w wiring, done finished) (*trigger.Dispatcher, *ledger.Memory) {
	t.Helper()
	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{
		{ID: "triage", VersionID: "v1", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true},
		{ID: "cobranca", VersionID: "v1", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true},
	}}
	clock := fixedClock{t: time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)}
	opener := trigger.NewOpener(store, reg, clock).WithContent(engine.NewMemoryContent())
	return trigger.NewDispatcher(w, done, opener, clock, nil), store
}

func aFinishedRun(agent domain.AgentID, id domain.RunID) finished {
	return finished{{
		RunID: id, AgentID: agent, Phase: "finished",
		Scope: domain.Scope{Company: "acme", Area: "cx"},
	}}
}

func TestSweep_aFinishedRun_startsWhoeverListens(t *testing.T) {
	t.Parallel()

	d, store := dispatcherFor(t, wiring{
		emits:  map[domain.AgentID][]string{"triage": {"ticket.triado"}},
		listen: map[string][]domain.AgentID{"ticket.triado": {"cobranca"}},
	}, aFinishedRun("triage", "run-1"))

	opened, err := d.Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 1 {
		t.Fatalf("opened %d runs, want 1", opened)
	}

	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want the one the event opened", len(runs))
	}
}

func TestSweep_runTwice_opensTheSameRunOnce(t *testing.T) {
	t.Parallel()

	// The sweep exists because a worker can die between finishing a run and
	// publishing its event. Running again must reach the run the last pass
	// opened rather than opening a second one.
	d, store := dispatcherFor(t, wiring{
		emits:  map[domain.AgentID][]string{"triage": {"ticket.triado"}},
		listen: map[string][]domain.AgentID{"ticket.triado": {"cobranca"}},
	}, aFinishedRun("triage", "run-1"))

	if _, err := d.Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	opened, err := d.Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if opened != 0 {
		t.Errorf("the second sweep opened %d runs, want none", opened)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Errorf("the ledger holds %d runs, want one", len(runs))
	}
}

func TestSweep_anEventNobodyListensTo_opensNothing(t *testing.T) {
	t.Parallel()

	d, _ := dispatcherFor(t, wiring{
		emits:  map[domain.AgentID][]string{"triage": {"ticket.triado"}},
		listen: map[string][]domain.AgentID{"outro.evento": {"cobranca"}},
	}, aFinishedRun("triage", "run-1"))

	opened, err := d.Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 0 {
		t.Errorf("opened %d runs for an event nobody listens to", opened)
	}
}

func TestSweep_anAgentListeningToItself_doesNotLoop(t *testing.T) {
	t.Parallel()

	// It would trigger itself for ever, and the run it opened would finish and
	// trigger itself again. Refused where the whole graph is visible.
	d, store := dispatcherFor(t, wiring{
		emits:  map[domain.AgentID][]string{"triage": {"ticket.triado"}},
		listen: map[string][]domain.AgentID{"ticket.triado": {"triage"}},
	}, aFinishedRun("triage", "run-1"))

	if _, err := d.Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	runs, _ := store.Runs(context.Background())
	if len(runs) != 0 {
		t.Errorf("the ledger holds %d runs, want none: the agent listens to itself", len(runs))
	}
}

func TestSweep_aStoppedPlatform_publishesNothingAndDoesNotFail(t *testing.T) {
	t.Parallel()

	// A listener that cannot start is the platform doing what somebody
	// configured. Treating it as a failure would stop every other listener of
	// the same event.
	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{{
		ID: "cobranca", VersionID: "v1", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true,
	}}}
	clock := fixedClock{t: time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)}
	opener := trigger.NewOpener(store, reg, clock).
		WithContent(engine.NewMemoryContent()).
		WithStops(stops{{Level: domain.StopInstallation, Reason: "incidente"}})

	d := trigger.NewDispatcher(wiring{
		emits:  map[domain.AgentID][]string{"triage": {"ticket.triado"}},
		listen: map[string][]domain.AgentID{"ticket.triado": {"cobranca"}},
	}, aFinishedRun("triage", "run-1"), opener, clock, nil)

	opened, err := d.Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep under a stop = %v, want it to carry on quietly", err)
	}
	if opened != 0 {
		t.Errorf("opened %d runs while the platform is stopped", opened)
	}
}
