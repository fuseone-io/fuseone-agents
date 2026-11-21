package trigger_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Two attempts under one key, both past the lookup before either writes (#432). Each derives its
run from the moment it was attempted, so they name different runs, and only one may open: the
other answers with the run the key now names, and keeps nothing behind it. Kept first and
named after, the loser's input stayed with no run naming it, beyond every erasure that starts
from a run.
*/

// bothPastTheLookup asks the store first, then holds each of the first two answers until
// both are in hand: both attempts have learned the key names no run before either writes.
// The race, made to happen every time; missed records whether it did.
type bothPastTheLookup struct {
	*ledger.Memory
	arrived sync.WaitGroup
	asked   int
	missed  int
	mu      sync.Mutex
}

func (b *bothPastTheLookup) RunByIdemKey(ctx context.Context, key string) (domain.RunID, error) {
	run, err := b.Memory.RunByIdemKey(ctx, key)
	b.mu.Lock()
	b.asked++
	first := b.asked <= 2
	if first && err != nil {
		b.missed++
	}
	b.mu.Unlock()
	if first {
		b.arrived.Done()
		b.arrived.Wait()
	}
	return run, err
}

// countingInputs counts every input kept, whoever keeps it.
type countingInputs struct {
	mu   sync.Mutex
	kept int
}

func (c *countingInputs) Put(context.Context, domain.RunID, int64, []byte) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kept++
	return "run://kept", nil
}

func (c *countingInputs) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.kept
}

// ticking is a clock a millisecond later each time it is read.
type ticking struct {
	mu sync.Mutex
	at time.Time
}

func (c *ticking) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(time.Millisecond)
	return c.at
}

func TestOpen_twoAttemptsPastTheLookup_openOneRunAndKeepOneInput(t *testing.T) {
	t.Parallel()
	inputs := &countingInputs{}
	store := &bothPastTheLookup{Memory: ledger.NewMemory().WithInputs(inputs)}
	store.arrived.Add(2)
	reg := registry{versions: []domain.AgentSummary{{
		ID: "triage", VersionID: "v2", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true,
	}}}
	opener := trigger.NewOpener(store, reg, &ticking{at: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)})

	results := make([]trigger.Result, 2)
	var done sync.WaitGroup
	for i, body := range []string{`{"ticket":"a"}`, `{"ticket":"b"}`} {
		done.Add(1)
		go func() {
			defer done.Done()
			var err error
			results[i], err = opener.Open(context.Background(), trigger.Request{
				Agent: "triage", IdemKey: "hook:delivery-1", Trigger: "webhook", Input: []byte(body),
			})
			if err != nil {
				t.Errorf("attempt %d: %v", i+1, err)
			}
		}()
	}
	done.Wait()

	if store.missed != 2 {
		t.Fatalf("%d attempts found no run before writing, want both: the race did not happen", store.missed)
	}
	if results[0].RunID != results[1].RunID || results[0].Created == results[1].Created {
		t.Fatalf("results %+v, want one run, opened by exactly one attempt", results)
	}
	if runs, _ := store.Runs(context.Background()); len(runs) != 1 {
		t.Errorf("%d runs opened, want one", len(runs))
	}
	if kept := inputs.count(); kept != 1 {
		t.Errorf("%d inputs kept, want the one the run names", kept)
	}
}

// appendOnly is a ledger that can append and look a key up, and nothing else.
type appendOnly struct{ m *ledger.Memory }

func (a appendOnly) Append(ctx context.Context, s domain.Step) (domain.Step, error) {
	return a.m.Append(ctx, s)
}
func (a appendOnly) RunByIdemKey(ctx context.Context, key string) (domain.RunID, error) {
	return a.m.RunByIdemKey(ctx, key)
}

// There is no second way to open a run with an input: the second way is the one that left
// inputs behind. A ledger that cannot keep the input with the opening opens no such run, and
// still opens one without an input.
func TestOpen_onALedgerThatCannotKeepTheInputWithTheOpening_opensNoRunWithAnInput(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	reg := registry{versions: []domain.AgentSummary{{
		ID: "triage", VersionID: "v2", Scope: domain.Scope{Company: "acme", Area: "cx"}, Latest: true,
	}}}
	opener := trigger.NewOpener(appendOnly{store}, reg, fixedClock{t: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)})

	_, err := opener.Open(context.Background(), trigger.Request{Agent: "triage", IdemKey: "hook:1", Trigger: "webhook", Input: []byte(`{"a":1}`)})
	if !errors.Is(err, trigger.ErrNoInputLedger) {
		t.Fatalf("err = %v, want %v", err, trigger.ErrNoInputLedger)
	}
	if runs, _ := store.Runs(context.Background()); len(runs) != 0 {
		t.Errorf("%d runs opened with an input nothing kept", len(runs))
	}
	if _, err := opener.Open(context.Background(), trigger.Request{Agent: "triage", IdemKey: "cron:1", Trigger: "cron"}); err != nil {
		t.Errorf("a run with no input: %v", err)
	}
}
