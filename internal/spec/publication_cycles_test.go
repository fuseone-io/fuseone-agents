package spec_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/spec"
)

// looping is the definition of an agent that emits one event and starts on
// another.
func looping(t *testing.T, id, emits, listens string) spec.Spec {
	t.Helper()
	source := strings.NewReplacer(
		"id: triage", "id: "+id,
		`  - { type: cron, schedule: "*/15 * * * *" }`, "  - { type: event, on: "+listens+" }",
		"tools:", "emits:\n  - "+emits+"\ntools:",
	).Replace(definition)
	return published(t, source)
}

/*
Two publications that close a cycle only together cannot both miss it (#227,
review round 1).

Each judges the graph and then writes. Judged against the same graph — the
one from before either wrote — neither sees the other's half, both publish,
and nobody is warned about the loop they made. Developer is held between its
judgement and its write; reviewer, closing the loop, must then wait for
developer to commit and judge the graph developer left, rather than finish
on the graph from before.
*/
func TestPublisher_twoPublicationsCloseACycleTogether_theSecondToJudgeWarns(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	developer := looping(t, "developer", "code.ready", "review.done")
	reviewer := looping(t, "reviewer", "review.done", "code.ready")

	judged, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })
	held := spec.PauseAfterJudging(spec.NewPublisher(pool, fixedClock{}), func() {
		close(judged)
		<-release
	})
	first := make(chan publishResult, 1)
	go func() { first <- publishOn(ctx, held, developer) }()
	<-judged

	second := make(chan error, 1)
	var closing spec.Published
	go func() {
		var err error
		closing, err = spec.NewPublisher(pool, fixedClock{}).Publish(ctx,
			spec.Publication{Spec: reviewer, By: "usr_ana", Company: "acme"})
		second <- err
	}()
	waitForLockWait(t, pool, "%", second)
	release <- struct{}{}

	if got := <-first; got.err != nil || len(got.published.EventCycles.Cycles) != 0 {
		t.Fatalf("developer = %+v; want it published, judged before reviewer existed", got)
	}
	if err := <-second; err != nil || len(closing.EventCycles.Cycles) != 1 {
		t.Fatalf("reviewer = %+v, %v; want it published and warned of reviewer → developer", closing, err)
	}
}

/*
The other writers of a current version wait for a judgement too.

Publishing a file with no version chosen makes the newest current, and making
a version current changes which one the graph is drawn from. Either one
landing between a publication's judgement and its commit would change the
graph under it, so both queue behind a publication held after judging.
*/
func TestRegistry_writesThatChangeTheCurrentVersion_waitForAJudgement(t *testing.T) {
	cases := map[string]func(context.Context, *spec.Registry, spec.Spec) error{
		"publishing a version": func(ctx context.Context, r *spec.Registry, s spec.Spec) error {
			return r.Publish(ctx, s, "usr_ana", "acme")
		},
		"making a version current": func(ctx context.Context, r *spec.Registry, s spec.Spec) error {
			return r.MakeCurrent(ctx, s.ID, s.Version)
		},
	}
	for name, write := range cases {
		t.Run(name, func(t *testing.T) {
			pool := openSpecPool(t)
			ctx := context.Background()
			registry := spec.NewRegistry(pool)
			reviewer := looping(t, "reviewer", "review.done", "code.ready")
			if err := registry.Publish(ctx, looping(t, "reviewer", "other", "code.ready"), "usr_ana", "acme"); err != nil {
				t.Fatalf("seed reviewer: %v", err)
			}
			if name == "making a version current" {
				if err := registry.Publish(ctx, reviewer, "usr_ana", "acme"); err != nil {
					t.Fatalf("publish reviewer: %v", err)
				}
			}

			release := holdAJudgement(t, pool, looping(t, "developer", "code.ready", "review.done"))
			done := make(chan error, 1)
			go func() { done <- write(ctx, registry, reviewer) }()
			waitForLockWait(t, pool, "%", done)
			release()
			if err := <-done; err != nil {
				t.Fatalf("%s after the judgement committed: %v", name, err)
			}
		})
	}
}

// holdAJudgement starts publishing s and holds it between its judgement and
// its write; the returned function lets it finish and waits for it.
func holdAJudgement(t *testing.T, pool *pgxpool.Pool, s spec.Spec) func() {
	t.Helper()
	judged, release := make(chan struct{}), make(chan struct{})
	held := spec.PauseAfterJudging(spec.NewPublisher(pool, fixedClock{}), func() {
		close(judged)
		<-release
	})
	finished := make(chan publishResult, 1)
	go func() { finished <- publishOn(context.Background(), held, s) }()
	<-judged
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	return func() {
		once.Do(func() { close(release) })
		if got := <-finished; got.err != nil {
			t.Fatalf("the held publication: %v", got.err)
		}
	}
}

type publishResult struct {
	published spec.Published
	err       error
}

func publishOn(ctx context.Context, p *spec.Publisher, s spec.Spec) publishResult {
	published, err := p.Publish(ctx, spec.Publication{Spec: s, By: "usr_ana", Company: "acme"})
	return publishResult{published: published, err: err}
}
