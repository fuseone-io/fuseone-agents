package trigger_test

import (
	"context"
	"encoding/json"
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
	emits  map[domain.AgentID][]domain.AgentEvent
	listen map[string][]domain.AgentID
}

func (w wiring) Emitters(context.Context) (map[domain.AgentID][]domain.AgentEvent, error) {
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
		emits:  map[domain.AgentID][]domain.AgentEvent{"triage": {{Event: "ticket.triado"}}},
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

func TestSweep_eventContextReachesTheListeningRunInput(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	content := engine.NewMemoryContent()
	reg := registry{versions: []domain.AgentSummary{
		{ID: "cobranca", VersionID: "v1", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true},
	}}
	clock := fixedClock{t: time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)}
	opener := trigger.NewOpener(store, reg, clock).WithContent(content)
	d := trigger.NewDispatcher(wiring{
		emits: map[domain.AgentID][]domain.AgentEvent{"triage": {{
			Event: "incident.triaged", Context: "incident",
			Artifacts: []string{"triage_summary", "suspected_cause"},
		}}},
		listen: map[string][]domain.AgentID{"incident.triaged": {"cobranca"}},
	}, aFinishedRun("triage", "run-1"), opener, clock, nil)

	if _, err := d.Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Fatalf("runs = %v, want the listener run", runs)
	}
	steps, err := store.Read(context.Background(), runs[0], domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		t.Fatalf("decode run_started: %v", err)
	}
	body, err := content.Get(context.Background(), started.InputRef)
	if err != nil {
		t.Fatalf("input content: %v", err)
	}
	var input struct {
		Event     string   `json:"event"`
		FromRun   string   `json:"from_run"`
		FromAgent string   `json:"from_agent"`
		Context   string   `json:"context"`
		Artifacts []string `json:"artifacts"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		t.Fatalf("decode event input: %v", err)
	}
	if input.Event != "incident.triaged" || input.FromRun != "run-1" ||
		input.FromAgent != "triage" || input.Context != "incident" ||
		len(input.Artifacts) != 2 || input.Artifacts[1] != "suspected_cause" {
		t.Errorf("input = %+v, want the source run and declared context", input)
	}
}

func TestSweep_theListeningRunInheritsTheSourceAuthorityAndLabels(t *testing.T) {
	t.Parallel()

	d, store := dispatcherFor(t, wiring{
		emits:  map[domain.AgentID][]domain.AgentEvent{"triage": {{Event: "incident.triaged"}}},
		listen: map[string][]domain.AgentID{"incident.triaged": {"cobranca"}},
	}, finished{{
		RunID: "run-1", AgentID: "triage", Phase: "finished",
		Scope:      domain.Scope{Company: "acme", Area: "cx"},
		OnBehalfOf: "usr_ana",
		Labels:     domain.NewLabels(domain.LabelPersonal, domain.LabelUntrusted),
	}})

	if _, err := d.Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 1 {
		t.Fatalf("runs = %v, want the listener run", runs)
	}
	steps, err := store.Read(context.Background(), runs[0], domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	started := steps[0]
	if started.OnBehalfOf != "usr_ana" {
		t.Errorf("on behalf of = %q, want the person behind the source run", started.OnBehalfOf)
	}
	if !started.Labels.Has(domain.LabelUntrusted) || !started.Labels.Has(domain.LabelPersonal) {
		t.Errorf("labels = %v, want the source run labels inherited", started.Labels)
	}
	if !started.Labels.Has(domain.LabelArea(domain.Scope{Company: "acme", Area: "cx"})) {
		t.Errorf("labels = %v, want the listening run's scope sealed too", started.Labels)
	}
}

func TestSweep_crossAreaSourceLabelsDoNotOpenAListeningRun(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{
		{ID: "cobranca", VersionID: "v1", Scope: domain.Scope{Company: "acme", Area: "billing"}, Latest: true},
	}}
	clock := fixedClock{t: time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)}
	opener := trigger.NewOpener(store, reg, clock).WithContent(engine.NewMemoryContent())
	d := trigger.NewDispatcher(wiring{
		emits:  map[domain.AgentID][]domain.AgentEvent{"triage": {{Event: "incident.triaged"}}},
		listen: map[string][]domain.AgentID{"incident.triaged": {"cobranca"}},
	}, finished{{
		RunID: "run-1", AgentID: "triage", Phase: "finished",
		Scope:      domain.Scope{Company: "acme", Area: "cx"},
		OnBehalfOf: "usr_ana",
		Labels:     domain.ScopeLabels(domain.Scope{Company: "acme", Area: "cx"}),
	}}, opener, clock, nil)

	opened, err := d.Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 0 {
		t.Fatalf("opened %d runs, want none", opened)
	}
	runs, _ := store.Runs(context.Background())
	if len(runs) != 0 {
		t.Fatalf("ledger holds %d runs, want none", len(runs))
	}
}

func TestSweep_runTwice_opensTheSameRunOnce(t *testing.T) {
	t.Parallel()

	// The sweep exists because a worker can die between finishing a run and
	// publishing its event. Running again must reach the run the last pass
	// opened rather than opening a second one.
	d, store := dispatcherFor(t, wiring{
		emits:  map[domain.AgentID][]domain.AgentEvent{"triage": {{Event: "ticket.triado"}}},
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
		emits:  map[domain.AgentID][]domain.AgentEvent{"triage": {{Event: "ticket.triado"}}},
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
		emits:  map[domain.AgentID][]domain.AgentEvent{"triage": {{Event: "ticket.triado"}}},
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
		emits:  map[domain.AgentID][]domain.AgentEvent{"triage": {{Event: "ticket.triado"}}},
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
