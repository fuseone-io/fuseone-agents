package spec_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

var refuseAutonomous = []domain.Stage{domain.StageAutonomous}

// twoVersions publishes a first version unguarded and returns a second one,
// not yet published.
func twoVersions(t *testing.T, pool *pgxpool.Pool) (first, second spec.Spec) {
	t.Helper()
	first = published(t, definition)
	second = published(t, strings.Replace(definition,
		"Read the ticket and classify it.", "Read the ticket twice and classify it.", 1))
	if first.Version == second.Version {
		t.Fatal("test setup did not change the version")
	}
	if _, err := spec.NewPublisher(pool, fixedClock{}).Publish(context.Background(),
		spec.Publication{Spec: first, By: "usr_ana", Company: "acme"}); err != nil {
		t.Fatalf("Publish(first): %v", err)
	}
	return first, second
}

// wroteNothing asserts the second version is neither recorded nor current.
func wroteNothing(t *testing.T, pool *pgxpool.Pool, first, second spec.Spec) {
	t.Helper()
	versions, err := spec.NewRegistry(pool).Versions(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 1 || versions[0].VersionID != first.Version {
		t.Fatalf("versions = %+v, want the first alone, still current", versions)
	}
	if second.Version == versions[0].VersionID {
		t.Fatal("the refused version is current")
	}
}

func TestPublisher_agentIsAutonomous_refusesAndWritesNothing(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	first, second := twoVersions(t, pool)
	if err := spec.NewState(pool).SetStage(ctx, first.ID, domain.StageAutonomous, "usr_ops"); err != nil {
		t.Fatalf("SetStage: %v", err)
	}

	_, err := spec.NewPublisher(pool, fixedClock{}).Publish(ctx, spec.Publication{
		Spec: second, By: "usr_ana", Company: "acme", RefuseStages: refuseAutonomous,
	})
	if !errors.Is(err, spec.ErrStageRefused) {
		t.Fatalf("Publish = %v, want ErrStageRefused", err)
	}
	wroteNothing(t, pool, first, second)
}

func TestPublisher_agentIsCopilot_publishesUnderTheSameCondition(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	first, second := twoVersions(t, pool)
	if err := spec.NewState(pool).SetStage(ctx, first.ID, domain.StageCopilot, "usr_ops"); err != nil {
		t.Fatalf("SetStage: %v", err)
	}

	if _, err := spec.NewPublisher(pool, fixedClock{}).Publish(ctx, spec.Publication{
		Spec: second, By: "usr_ana", Company: "acme", RefuseStages: refuseAutonomous,
	}); err != nil {
		t.Fatalf("Publish = %v, want the copilot agent's version published", err)
	}
	versions, err := spec.NewRegistry(pool).Versions(ctx, first.ID)
	if err != nil || len(versions) == 0 || versions[0].VersionID != second.Version {
		t.Fatalf("versions = %+v (%v), want the second current", versions, err)
	}
}

// A role published for the first time has no state row yet; the guard must
// create the row it locks rather than fail on its absence.
func TestPublisher_agentNeverPublished_guardedPublicationCreatesItAtDraft(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	fresh := published(t, definition)

	if _, err := spec.NewPublisher(pool, fixedClock{}).Publish(ctx, spec.Publication{
		Spec: fresh, By: "usr_ana", Company: "acme", RefuseStages: refuseAutonomous,
	}); err != nil {
		t.Fatalf("Publish = %v, want a new agent published", err)
	}
	stage, err := spec.NewState(pool).StageOf(ctx, fresh.ID)
	if err != nil || stage != domain.StageDraft {
		t.Fatalf("stage = %s (%v), want draft", stage, err)
	}
	paused, err := spec.NewState(pool).IsPaused(ctx, fresh.ID)
	if err != nil || !paused {
		t.Fatalf("paused = %v (%v), want a new agent paused", paused, err)
	}
}

/*
A promotion committed while the publication waits is seen.

The competitor promotes the agent in a transaction the test holds open, so it
holds the agent's row. The publication starts, and the test waits until
pg_stat_activity shows it queued on a lock (no sleep decides the order). Then
the promotion commits. The publication must refuse: it judged the stage after
its lock was granted, not from a read taken before the promotion committed.
*/
func TestPublisher_promotionCommittedWhileThePublicationWaits_refuses(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	first, second := twoVersions(t, pool)

	promotion, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin promotion: %v", err)
	}
	defer func() { _ = promotion.Rollback(ctx) }()
	if _, err := promotion.Exec(ctx,
		`update agent_state set stage = 'autonomous' where agent_id = $1`, string(first.ID)); err != nil {
		t.Fatalf("promote: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- errOf(spec.NewPublisher(pool, fixedClock{}).Publish(ctx, spec.Publication{
			Spec: second, By: "usr_ana", Company: "acme", RefuseStages: refuseAutonomous,
		}))
	}()
	waitForLockWait(t, pool, "%agent_state%", done)
	if err := promotion.Commit(ctx); err != nil {
		t.Fatalf("commit promotion: %v", err)
	}

	if err := <-done; !errors.Is(err, spec.ErrStageRefused) {
		t.Fatalf("Publish = %v, want ErrStageRefused: the promotion committed before the publication judged", err)
	}
	wroteNothing(t, pool, first, second)
}

/*
A promotion arriving after the publication judged waits for it.

The other half of the race. The publication has judged the agent copilot and
not yet made the version current; a promotion committing now would have
makeCurrent hand the new version to an agent that is already autonomous. To
stop the publication there, a gate transaction holds agent_specs in share
mode, so the version's insert queues behind it. The promotion is started then,
and must queue on the agent's row rather than finish. Only after the gate lets
go and the publication commits does the promotion land.
*/
func TestPublisher_promotionAfterTheJudgement_waitsForThePublicationToCommit(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	first, second := twoVersions(t, pool)

	gate, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin gate: %v", err)
	}
	defer func() { _ = gate.Rollback(ctx) }()
	if _, err := gate.Exec(ctx, `lock table agent_specs in share mode`); err != nil {
		t.Fatalf("hold agent_specs: %v", err)
	}

	published := make(chan error, 1)
	go func() {
		published <- errOf(spec.NewPublisher(pool, fixedClock{}).Publish(ctx, spec.Publication{
			Spec: second, By: "usr_ana", Company: "acme", RefuseStages: refuseAutonomous,
		}))
	}()
	waitForLockWait(t, pool, "%insert into agent_specs%", published)

	promoted := make(chan error, 1)
	go func() { promoted <- spec.NewState(pool).SetStage(ctx, first.ID, domain.StageAutonomous, "usr_ops") }()
	waitForLockWait(t, pool, "%excluded.stage%", promoted)

	if err := gate.Rollback(ctx); err != nil {
		t.Fatalf("release the gate: %v", err)
	}
	if err := <-published; err != nil {
		t.Fatalf("Publish = %v, want the copilot agent's version published", err)
	}
	if err := <-promoted; err != nil {
		t.Fatalf("SetStage after the publication committed: %v", err)
	}
}

// errOf is a publication's error alone, for the tests that only judge stages.
func errOf(_ spec.Published, err error) error { return err }

// waitForLockWait returns once another backend running a query matching
// pattern is queued on a lock, and fails if that write finished first.
func waitForLockWait(t *testing.T, pool *pgxpool.Pool, pattern string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("the write matching %s finished (%v) without waiting for the lock", pattern, err)
		default:
		}
		var waiting int
		if err := pool.QueryRow(context.Background(), `
			select count(*) from pg_stat_activity
			where wait_event_type = 'Lock' and query like $1 and pid <> pg_backend_pid()`, pattern,
		).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the write matching %s never queued on a lock", pattern)
}
