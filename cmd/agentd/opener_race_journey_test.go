package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The opener the worker builds, on Postgres, under a race it is made to lose every time (#432):
two attempts under one key, both past the lookup before either writes, a few milliseconds
apart so each derives a run of its own. One run opens, and one input is kept: the one that
run names. Kept first and named after, the losing attempt's input stayed behind with no run
naming it, beyond every erasure that starts from a run.
*/

// bothAskFirst asks Postgres first, then holds each of the first two answers until both are
// in hand, and lets the second go a few milliseconds after the first so the two derive runs
// of their own. Both have learned the key names no run before either writes; missed records
// that they did.
type bothAskFirst struct {
	Store
	mu      sync.Mutex
	asked   int
	missed  int
	arrived sync.WaitGroup
}

func (b *bothAskFirst) RunByIdemKey(ctx context.Context, key string) (domain.RunID, error) {
	run, err := b.Store.RunByIdemKey(ctx, key)
	b.mu.Lock()
	b.asked++
	turn := b.asked
	if turn <= 2 && err != nil {
		b.missed++
	}
	b.mu.Unlock()
	if turn <= 2 {
		b.arrived.Done()
		b.arrived.Wait()
		if turn == 2 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return run, err
}

func TestJourney_twoAttemptsUnderOneKey_openOneRunAndKeepOneInput(t *testing.T) {
	rig := newInheritRig(t, nil)
	agent := rig.run + "-hook"
	rig.publish(t, openapi.AgentDefinition{Name: agent})
	racing := &bothAskFirst{Store: rig.store}
	racing.arrived.Add(2)
	parts := &workerParts{store: racing, registry: spec.NewRegistry(rig.pool), configPool: rig.pool}
	opener := parts.opener()

	var done sync.WaitGroup
	for _, body := range []string{`{"ticket":"a"}`, `{"ticket":"b"}`} {
		done.Add(1)
		go func() {
			defer done.Done()
			if _, err := opener.Open(context.Background(), trigger.Request{
				Agent: domain.AgentID(agent), IdemKey: "hook:" + rig.run, Trigger: "webhook", Input: []byte(body),
			}); err != nil {
				t.Errorf("Open: %v", err)
			}
		}()
	}
	done.Wait()

	if racing.missed != 2 {
		t.Fatalf("%d attempts found no run before writing, want both: the race did not happen", racing.missed)
	}
	var runs, inputs int
	if err := rig.pool.QueryRow(t.Context(), `
		select (select count(*) from run_steps where kind = 'run_started' and idem_key = $1),
		       (select count(*) from run_content where owner_kind = 'run' and seq = 1 and run_id like $2)`,
		"hook:"+rig.run, "run_"+agent+"_%").Scan(&runs, &inputs); err != nil {
		t.Fatalf("count: %v", err)
	}
	if runs != 1 || inputs != 1 {
		t.Errorf("%d runs opened and %d inputs kept under one key, want one of each", runs, inputs)
	}
}
