package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/spec"
)

/*
An experiment on a candidate and an erasure of its seal, in both orders (NT-018 phase 2,
release 2b).

Both take the agent's content fence exclusive, so one waits for the other, and the seal is
read inside that fence. With the erasure first, the experiment finds the tombstone and
opens nothing. With the experiment first, the erasure waits for it and then reaches the
runs it opened. Read outside the fence, the seal could be found readable, the erasure could
commit, and the experiment open runs on bytes that no longer exist.
*/

// eraseCandidate erases a seal and the runs derived from it, under the agent's fence, the
// way a thread or subject erasure reaching the seal does. erase runs when release is
// closed, so the fence is held while nothing is erased yet.
func eraseCandidate(
	ctx context.Context, pool interface {
		Begin(context.Context) (pgx.Tx, error)
	}, store *regression.Store, w *experimentWorld, seal string, entered, release chan struct{},
) error {
	return store.Exclusive(ctx, []domain.AgentID{w.agent}, func(ctx context.Context) error {
		close(entered)
		<-release
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		content := ledger.NewContent(w.pool)
		if _, err := content.EraseOwnersTx(ctx, tx, "authoring", []string{seal}, "subject"); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `select run_id from runs where seal_id = $1::uuid`, seal)
		if err != nil {
			return err
		}
		runs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if _, err := content.EraseOwnersTx(ctx, tx, "run", runs, "subject"); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
}

func TestExperiment_onASealAnErasureHoldsTheFenceFor_findsTheTombstone(t *testing.T) {
	world := newExperimentWorld(t)
	world.resume(t)
	world.seal(t, routeSeal)
	ctx := context.Background()
	entered, release, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		erased <- eraseCandidate(ctx, world.pool, regression.NewStore(world.pool), world, routeSeal, entered, release)
	}()
	<-entered

	started := make(chan openapi.StartExperimentResponseObject, 1)
	go func() { started <- world.startOnSeal(t, author(ctx), "contention-seal-1", onSeal(routeSeal)) }()
	select {
	case resp := <-started:
		t.Fatalf("the experiment did not wait for the erasure's fence: %T", resp)
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	if err := <-erased; err != nil {
		t.Fatalf("erase: %v", err)
	}

	code, isConflict := conflictCode(<-started)
	if !isConflict || code != "fuseone:candidate-unreadable" {
		t.Fatalf("after the erasure = %q (conflict=%v), want fuseone:candidate-unreadable", code, isConflict)
	}
	if runs, _ := world.store.SimulationRuns(ctx, experimentIDFor(world.agent, "contention-seal-1")); len(runs) != 0 {
		t.Errorf("runs opened on an erased seal: %v", runs)
	}
}

// stallingFence holds the experiment inside its fence until released.
type stallingFence struct {
	*regression.Store
	entered, release chan struct{}
}

func (f stallingFence) Exclusive(ctx context.Context, agents []domain.AgentID, inside func(context.Context) error) error {
	return f.Store.Exclusive(ctx, agents, func(ctx context.Context) error {
		close(f.entered)
		<-f.release
		return inside(ctx)
	})
}

func TestExperiment_onASealHoldingTheFence_isWaitedForAndItsRunsAreErased(t *testing.T) {
	world := newExperimentWorld(t)
	world.resume(t)
	world.seal(t, routeSeal)
	ctx := context.Background()
	fence := stallingFence{Store: regression.NewStore(world.pool), entered: make(chan struct{}), release: make(chan struct{})}
	world.server.WithRegressions(fence)

	started := make(chan openapi.StartExperimentResponseObject, 1)
	go func() { started <- world.startOnSeal(t, author(ctx), "contention-seal-2", onSeal(routeSeal)) }()
	<-fence.entered
	eraseEntered, eraseRelease, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	close(eraseRelease)
	go func() {
		erased <- eraseCandidate(ctx, world.pool, regression.NewStore(world.pool), world, routeSeal, eraseEntered, eraseRelease)
	}()
	select {
	case <-eraseEntered:
		t.Fatal("the erasure entered the fence the experiment holds")
	case <-time.After(400 * time.Millisecond):
	}
	close(fence.release)

	accepted, ok := (<-started).(openapi.StartExperiment202JSONResponse)
	if !ok || accepted.Runs != 1 {
		t.Fatalf("the experiment that held the fence = %+v, want its run opened", accepted)
	}
	if err := <-erased; err != nil {
		t.Fatalf("erase: %v", err)
	}
	runs, err := world.store.SimulationRuns(ctx, accepted.Id)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v (%v)", runs, err)
	}
	var total, live int
	if err := world.pool.QueryRow(ctx, `select count(*), count(*) filter (where erased_at is null)
		from run_content where owner_kind = 'run' and run_id = $1`, string(runs[0])).Scan(&total, &live); err != nil {
		t.Fatal(err)
	}
	if total == 0 || live != 0 {
		t.Errorf("the run opened before the erasure has %d content objects, %d still live; want some, none live", total, live)
	}
}

// An agent moved to another area after the seal still runs its candidate where it was
// sealed, for whoever may publish there.
func TestExperiment_onASealOfAnAgentMovedSince_runsInTheSealedScope(t *testing.T) {
	world := newExperimentWorld(t)
	world.resume(t)
	world.seal(t, routeSeal)
	moved, err := spec.Parse("triage.agent.md", []byte(fmt.Sprintf(`---
id: %s
name: Triage
area: sales
provider: primary
model: declared-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 20
---

Classify the ticket, from sales.
`, world.agent)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := spec.NewPublisher(world.pool, engine.SystemClock{}).Publish(t.Context(), spec.Publication{
		Spec: moved, By: "usr_ops", Company: "acme",
	}); err != nil {
		t.Fatalf("move the agent to sales: %v", err)
	}

	accepted, ok := world.startOnSeal(t, author(t.Context()), "contention-seal-3", onSeal(routeSeal)).(openapi.StartExperiment202JSONResponse)
	if !ok || accepted.Runs != 1 {
		t.Fatalf("an experiment on the cx seal of an agent now in sales = %+v, want its run", accepted)
	}
	runs, err := world.store.SimulationRuns(t.Context(), accepted.Id)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v (%v)", runs, err)
	}
	steps, err := world.store.Read(t.Context(), runs[0], domain.FirstSeq)
	if err != nil || steps[0].Scope != (domain.Scope{Company: "acme", Area: "cx"}) {
		t.Errorf("the run opened in %v (%v), want the sealed scope acme/cx", steps[0].Scope, err)
	}
}
