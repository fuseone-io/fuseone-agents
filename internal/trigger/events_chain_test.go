package trigger_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
A chain of event-opened runs carries its length in the ledger, and stops at
domain.MaxEventHops (#203). Without it an A → B → A cycle is bounded only by
budget: each hop has a fresh idempotency key from its new source run.
*/

var chainScope = domain.Scope{Company: "acme", Area: "cx"}

// byAgent publishes each agent under its own id, so a run opened for B is B's.
type byAgent []domain.AgentID

func (b byAgent) Versions(_ context.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	for _, known := range b {
		if known == agent {
			return []domain.AgentSummary{{ID: agent, VersionID: "v1", Scope: chainScope, Latest: true}}, nil
		}
	}
	return nil, nil
}

// pingPong is the cycle: A's event starts B, B's starts A.
var pingPong = wiring{
	emits: map[domain.AgentID][]domain.AgentEvent{
		"agent-a": {{Event: "ping"}}, "agent-b": {{Event: "pong"}},
	},
	listen: map[string][]domain.AgentID{"ping": {"agent-b"}, "pong": {"agent-a"}},
}

// chainDispatcher is one worker's dispatcher over a shared ledger: the
// finished runs it sweeps are the ledger's own, as in production.
func chainDispatcher(store *ledger.Memory) *trigger.Dispatcher {
	return dispatcherWith(store, pingPong)
}

func dispatcherWith(store *ledger.Memory, w trigger.Subscriptions) *trigger.Dispatcher {
	clock := fixedClock{t: time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)}
	opener := trigger.NewOpener(store, byAgent{"agent-a", "agent-b"}, clock).
		WithContent(engine.NewMemoryContent())
	return trigger.NewDispatcher(w, store, opener, clock, nil)
}

// startRun seals a run_started: opened by an event carrying chain, or by a
// person when chain is nil.
func startRun(t *testing.T, store *ledger.Memory, id domain.RunID, agent domain.AgentID, chain *domain.EventChain) {
	t.Helper()
	trigger := "manual"
	if chain != nil {
		trigger = domain.TriggerEvent
	}
	sealStart(t, store, id, agent, domain.RunStartedPayload{Trigger: trigger, Chain: chain})
}

func sealStart(t *testing.T, store *ledger.Memory, id domain.RunID, agent domain.AgentID, p domain.RunStartedPayload) {
	t.Helper()
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: id, Kind: domain.StepRunStarted, Scope: chainScope, AgentID: agent, VersionID: "v1",
		IdemKey: "start:" + string(id), At: time.Date(2026, 10, 31, 8, 0, 0, 0, time.UTC),
		Payload: mustJSON(p),
	}); err != nil {
		t.Fatalf("start %s: %v", id, err)
	}
}

func finishRun(t *testing.T, store *ledger.Memory, id domain.RunID) {
	t.Helper()
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: id, Kind: domain.StepRunFinished, Scope: chainScope,
		At: time.Date(2026, 10, 31, 8, 30, 0, 0, time.UTC), Payload: mustJSON(domain.RunFinishedPayload{}),
	}); err != nil {
		t.Fatalf("finish %s: %v", id, err)
	}
}

// openedChains is the chain sealed on every run an event opened.
func openedChains(t *testing.T, store *ledger.Memory) map[domain.RunID]domain.EventChain {
	t.Helper()
	ids, err := store.Runs(context.Background())
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	out := map[domain.RunID]domain.EventChain{}
	for _, id := range ids {
		steps, err := store.Read(context.Background(), id, domain.FirstSeq)
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if strings.HasPrefix(steps[0].IdemKey, "start:") {
			continue // sealed by the test, not opened by the sweep
		}
		var p domain.RunStartedPayload
		if err := json.Unmarshal(steps[0].Payload, &p); err != nil {
			t.Fatalf("decode start of %s: %v", id, err)
		}
		if p.Trigger == "event" {
			if p.Chain == nil {
				t.Fatalf("%s was opened by an event and carries no chain", id)
			}
			out[id] = *p.Chain
		}
	}
	return out
}

func TestSweep_aRunNoEventOpened_opensItsListenerAtHopOne(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	startRun(t, store, "run-root", "agent-a", nil)
	finishRun(t, store, "run-root")

	if _, err := chainDispatcher(store).Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	chains := openedChains(t, store)
	if len(chains) != 1 {
		t.Fatalf("opened %d runs, want the listener", len(chains))
	}
	for _, chain := range chains {
		if chain != (domain.EventChain{Root: "run-root", Hop: 1}) {
			t.Fatalf("chain = %+v, want root run-root at hop 1", chain)
		}
	}
}

func TestSweep_anEventOpenedRun_carriesItsRootOneHopFurther(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	startRun(t, store, "run-mid", "agent-a", &domain.EventChain{Root: "run-root", Hop: 3})
	finishRun(t, store, "run-mid")

	if _, err := chainDispatcher(store).Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	chains := openedChains(t, store)
	if len(chains) != 1 {
		t.Fatalf("opened %d runs, want the listener", len(chains))
	}
	for _, chain := range chains {
		if chain != (domain.EventChain{Root: "run-root", Hop: 4}) {
			t.Fatalf("chain = %+v, want root run-root at hop 4", chain)
		}
	}
}

func TestSweep_aRunAtTheHopLimit_opensNoListener(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	startRun(t, store, "run-last", "agent-a", &domain.EventChain{Root: "run-root", Hop: domain.MaxEventHops})
	finishRun(t, store, "run-last")

	opened, err := chainDispatcher(store).Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 0 {
		t.Fatalf("opened %d runs past the hop limit, want none", opened)
	}
	if ids, _ := store.Runs(context.Background()); len(ids) != 1 {
		t.Fatalf("the ledger holds %d runs, want only the one at the limit", len(ids))
	}
}

func TestSweep_redeliveringTheSameEvent_doesNotCountItAgain(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	startRun(t, store, "run-mid", "agent-a", &domain.EventChain{Root: "run-root", Hop: 2})
	finishRun(t, store, "run-mid")

	for range 3 {
		if _, err := chainDispatcher(store).Sweep(context.Background(), 50); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
	}

	chains := openedChains(t, store)
	if len(chains) != 1 {
		t.Fatalf("event-opened runs = %d after three sweeps, want one listener", len(chains))
	}
	for _, chain := range chains {
		if chain.Hop != 3 {
			t.Fatalf("listener at hop %d after three sweeps, want 3", chain.Hop)
		}
	}
}

/*
The cycle the issue names, from the real entry: a finished run's event opens
the next run, that run finishes, its event opens the next. Each pass is a
different worker's dispatcher over the same ledger, so the count can only
come from the ledger. The chain stops with the run at the limit: the root
plus MaxEventHops event-opened runs, however many more passes run.
*/
func TestSweep_aTwoAgentCycle_stopsAtTheHopLimitAcrossWorkers(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	startRun(t, store, "run-root", "agent-a", nil)
	finishRun(t, store, "run-root")

	for range 3 * domain.MaxEventHops {
		if _, err := chainDispatcher(store).Sweep(context.Background(), 1000); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
		finishOpenRuns(t, store)
	}

	chains := openedChains(t, store)
	if len(chains) != domain.MaxEventHops {
		t.Fatalf("the cycle opened %d runs, want %d", len(chains), domain.MaxEventHops)
	}
	deepest := 0
	for _, chain := range chains {
		if chain.Root != "run-root" {
			t.Fatalf("chain root = %s, want run-root", chain.Root)
		}
		deepest = max(deepest, chain.Hop)
	}
	if deepest != domain.MaxEventHops {
		t.Fatalf("deepest hop = %d, want %d", deepest, domain.MaxEventHops)
	}
}

/*
A run an event opened with no chain was opened by a dispatcher that did not
count — an older binary during a rolling update. Read as a root it would
restart the count; it opens nothing instead (the run's own closing step says
why).
*/
func TestSweep_anEventOpenedSourceWithNoChain_opensNoListener(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	sealStart(t, store, "run-old", "agent-a", domain.RunStartedPayload{Trigger: domain.TriggerEvent})
	finishRun(t, store, "run-old")

	opened, err := chainDispatcher(store).Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 0 {
		t.Fatalf("opened %d runs from a source whose chain is unknown, want none", opened)
	}
}

// A start that will not decode fails the sweep. Read as a root instead, it
// would restart the count, and the cycle would go round again.
func TestSweep_aSourceStartThatWillNotDecode_failsRatherThanRestartingTheCount(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-mid", Kind: domain.StepRunStarted, Scope: chainScope, AgentID: "agent-a",
		VersionID: "v1", Payload: []byte(`{"trigger":"event","chain":{"hop":"eight"}}`),
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	finishRun(t, store, "run-mid")

	if _, err := chainDispatcher(store).Sweep(context.Background(), 50); err == nil {
		t.Fatal("Sweep succeeded on a source whose chain will not decode")
	}
	if ids, _ := store.Runs(context.Background()); len(ids) != 1 {
		t.Fatalf("the ledger holds %d runs, want only the source", len(ids))
	}
}

/*
What a source run publishes is what its pinned version declares — the same
version the engine resolved its run from, so the withheld record on its
closing step and the fan-out here cannot disagree. A publish in the middle
of the run changes neither.
*/
func TestSweep_whatARunPublishes_isWhatItsPinnedVersionDeclares(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		pinned []domain.AgentEvent
		want   int
	}{
		"pinned emits, latest stopped": {[]domain.AgentEvent{{Event: "ping"}}, 1},
		"pinned silent, latest emits":  {nil, 0},
	}
	for name, c := range cases {
		store := ledger.NewMemory()
		startRun(t, store, "run-root", "agent-a", nil) // pinned to v1
		finishRun(t, store, "run-root")
		latest := pingPong
		if c.want == 1 {
			latest.emits = map[domain.AgentID][]domain.AgentEvent{} // a later version stopped
		}
		latest.pinned = map[domain.VersionID][]domain.AgentEvent{"v1": c.pinned}

		opened, err := dispatcherWith(store, latest).Sweep(context.Background(), 50)
		if err != nil {
			t.Fatalf("%s: Sweep: %v", name, err)
		}
		if opened != c.want {
			t.Errorf("%s: opened %d runs, want %d", name, opened, c.want)
		}
	}
}

// Two runs of the same agent pinned to different versions, in one sweep: each
// publishes what its own version declares.
func TestSweep_twoRunsPinnedToDifferentVersions_eachPublishesItsOwn(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	startRun(t, store, "run-v1", "agent-a", nil)
	finishRun(t, store, "run-v1")
	if _, err := store.Append(context.Background(), domain.Step{
		RunID: "run-v2", Kind: domain.StepRunStarted, Scope: chainScope, AgentID: "agent-a", VersionID: "v2",
		Payload: mustJSON(domain.RunStartedPayload{Trigger: "manual"}),
	}); err != nil {
		t.Fatalf("start v2: %v", err)
	}
	finishRun(t, store, "run-v2")
	w := pingPong
	w.pinned = map[domain.VersionID][]domain.AgentEvent{"v1": {{Event: "ping"}}, "v2": nil}

	opened, err := dispatcherWith(store, w).Sweep(context.Background(), 50)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if opened != 1 {
		t.Fatalf("opened %d runs, want only the v1 run's listener", opened)
	}
}

// newerSilentVersion publishes agent-b at v2 — current, and no longer
// listening — above the v1 that listened.
type newerSilentVersion struct{}

func (newerSilentVersion) Versions(_ context.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	out := []domain.AgentSummary{{ID: agent, VersionID: "v1", Scope: chainScope}}
	if agent == "agent-b" {
		out = append([]domain.AgentSummary{{ID: agent, VersionID: "v2", Scope: chainScope, Latest: true}}, out...)
	}
	return out, nil
}

/*
The event went out while agent-b's v1 listened to it. A publish of a v2
that stops listening, landing between the listener snapshot and the open,
must not start v2 — the version that never asked for this event. The run
opens on the version that listened: that is the agent that chose to start
from it, when it was published.
*/
func TestSweep_theListenerOpensOnTheVersionThatListened(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	startRun(t, store, "run-root", "agent-a", nil)
	finishRun(t, store, "run-root")
	clock := fixedClock{t: time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)}
	opener := trigger.NewOpener(store, newerSilentVersion{}, clock).WithContent(engine.NewMemoryContent())
	w := pingPong
	w.listenAt = map[domain.AgentID]domain.VersionID{"agent-b": "v1"}

	if _, err := trigger.NewDispatcher(w, store, opener, clock, nil).Sweep(context.Background(), 50); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	ids, _ := store.Runs(context.Background())
	for _, id := range ids {
		steps, _ := store.Read(context.Background(), id, domain.FirstSeq)
		if steps[0].AgentID == "agent-b" && steps[0].VersionID != "v1" {
			t.Fatalf("the listener opened on %s, want v1 — the version that listened", steps[0].VersionID)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("runs = %v, want the source and the listener", ids)
	}
}

// finishOpenRuns ends every run still going, as a worker would.
func finishOpenRuns(t *testing.T, store *ledger.Memory) {
	t.Helper()
	running, err := store.ListRuns(context.Background(), domain.RunFilter{}, "running", 1000)
	if err != nil {
		t.Fatalf("list running: %v", err)
	}
	for _, run := range running {
		finishRun(t, store, run.RunID)
	}
}
