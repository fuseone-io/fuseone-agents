package spec_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Every write to an agent judges, under the agent's row lock, who may write and
what the writer last saw (NT-017 phase 1, NT-018 1a).

In the transaction that writes, after the event graph's lock and the agent's
row: first whether the caller may publish where the agent lives now — an agent
it may not reach is one that does not exist, so a stale tag never tells it the
agent changed — then the precondition. Judged by an earlier read, either would
leave the window it exists to close.
*/

func ifMatch(t *testing.T, tag domain.Revision) domain.IfMatch {
	t.Helper()
	header := string(tag)
	parsed, err := domain.ParseIfMatch(&header)
	if err != nil {
		t.Fatalf("ParseIfMatch(%s): %v", tag, err)
	}
	return parsed
}

func inScope(want domain.Scope) func(domain.Scope) bool {
	return func(got domain.Scope) bool { return got == want }
}

var cx = domain.Scope{Company: "acme", Area: "cx"}

func staleAt(t *testing.T, err error) domain.Revision {
	t.Helper()
	var stale *domain.StaleError
	if !errors.As(err, &stale) {
		t.Fatalf("err = %v, want a stale precondition", err)
	}
	return stale.Current
}

func TestPublisher_ifMatchTheAgentsCurrentTag_publishes(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)

	_, err := spec.NewPublisher(pool, fixedClock{}).Publish(t.Context(), spec.Publication{
		Spec: second, By: "usr_ana", Company: "acme",
		Guard: spec.Guard{Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(first.Version, false))},
	})
	if err != nil {
		t.Fatalf("Publish = %v, want the second version published", err)
	}
}

func TestPublisher_ifMatchAnOlderTag_refusesWithTheCurrentAndWritesNothing(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)

	_, err := spec.NewPublisher(pool, fixedClock{}).Publish(t.Context(), spec.Publication{
		Spec: second, By: "usr_ana", Company: "acme",
		Guard: spec.Guard{Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(first.Version, true))},
	})
	if got := staleAt(t, err); got != domain.AgentRevision(first.Version, false) {
		t.Errorf("current = %s, want the live first version's tag", got)
	}
	wroteNothing(t, pool, first, second)
}

func TestPublisher_ifNoneMatchOnAnAgentThatExists_refusesAndWritesNothing(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)

	_, err := spec.NewPublisher(pool, fixedClock{}).Publish(t.Context(), spec.Publication{
		Spec: second, By: "usr_ana", Company: "acme",
		Guard: spec.Guard{Allow: inScope(cx), Absent: true},
	})
	if got := staleAt(t, err); got != domain.AgentRevision(first.Version, false) {
		t.Errorf("current = %s, want the first version's tag", got)
	}
	wroteNothing(t, pool, first, second)
}

func TestPublisher_ifNoneMatchOnANewAgent_publishes(t *testing.T) {
	pool := openSpecPool(t)
	fresh := published(t, definition)

	// Allow is never asked: there is no agent, so no current scope to judge.
	_, err := spec.NewPublisher(pool, fixedClock{}).Publish(t.Context(), spec.Publication{
		Spec: fresh, By: "usr_ana", Company: "acme",
		Guard: spec.Guard{Allow: func(domain.Scope) bool { return false }, Absent: true},
	})
	if err != nil {
		t.Fatalf("Publish = %v, want the new agent published", err)
	}
}

// Any If-Match asks about a current representation, and a new agent has none.
func TestPublisher_ifMatchOnANewAgent_refuses(t *testing.T) {
	pool := openSpecPool(t)
	fresh := published(t, definition)

	_, err := spec.NewPublisher(pool, fixedClock{}).Publish(t.Context(), spec.Publication{
		Spec: fresh, By: "usr_ana", Company: "acme",
		Guard: spec.Guard{Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(fresh.Version, false))},
	})
	if got := staleAt(t, err); got != "" {
		t.Errorf("current = %s, want none for an agent that does not exist", got)
	}
}

/*
Moving an agent needs the scope it leaves as well as the one it enters: the
destination is the caller's to name, the current scope is not. A caller who
may not publish where the agent lives is told it does not exist, before its
precondition is judged, so a stale tag never reveals that it changed.
*/
func TestPublisher_callerCannotPublishWhereTheAgentLives_notFoundBeforeThePrecondition(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)
	moved := published(t, strings.Replace(definition, "area: cx", "area: ops", 1))
	var asked []domain.Scope

	_, err := spec.NewPublisher(pool, fixedClock{}).Publish(t.Context(), spec.Publication{
		Spec: moved, By: "usr_ana", Company: "acme",
		Guard: spec.Guard{
			Allow: func(s domain.Scope) bool {
				asked = append(asked, s)
				return s.Area == "ops"
			},
			Expect: ifMatch(t, `"v1-stale"`),
		},
	})
	if !errors.Is(err, spec.ErrAgentNotFound) {
		t.Fatalf("Publish = %v, want ErrAgentNotFound", err)
	}
	if len(asked) != 1 || asked[0] != cx {
		t.Errorf("Allow asked about %v, want the agent's current scope alone", asked)
	}
	wroteNothing(t, pool, first, second)
}

// held opens a transaction that changes the agent's state row and leaves it
// uncommitted, so a guarded write queues behind it.
func held(t *testing.T, pool *pgxpool.Pool, statement string, args ...any) func() {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, statement, args...); err != nil {
		t.Fatalf("hold: %v", err)
	}
	return func() {
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
}

/*
A retirement committed while the publication waits is seen: the tag is read
after the lock is granted, not from a snapshot taken before the competitor
committed. The test waits for pg_stat_activity to show the publication queued
on the lock; no sleep decides the order.
*/
func TestPublisher_retirementCommittedWhileThePublicationWaits_isStale(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)
	commit := held(t, pool, `update agent_state set retired_at = now() where agent_id = $1`, string(first.ID))

	done := make(chan error, 1)
	go func() {
		done <- errOf(spec.NewPublisher(pool, fixedClock{}).Publish(context.Background(), spec.Publication{
			Spec: second, By: "usr_ana", Company: "acme",
			Guard: spec.Guard{Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(first.Version, false))},
		}))
	}()
	waitForLockWait(t, pool, "%agent_state%", done)
	commit()

	if got := staleAt(t, <-done); got != domain.AgentRevision(first.Version, true) {
		t.Errorf("current = %s, want the retired tag the competitor committed", got)
	}
	wroteNothing(t, pool, first, second)
}

// A move committed while the publication waits is judged on the scope the
// agent moved to.
func TestPublisher_moveCommittedWhileThePublicationWaits_isNotFound(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)
	moved := published(t, strings.Replace(definition, "area: cx", "area: ops", 1))
	if err := spec.NewRegistry(pool).Publish(t.Context(), moved, "usr_ops", "acme"); err != nil {
		t.Fatalf("publish the ops version: %v", err)
	}
	commit := held(t, pool, `update agent_state set current_version = $2 where agent_id = $1`,
		string(first.ID), string(moved.Version))

	done := make(chan error, 1)
	go func() {
		done <- errOf(spec.NewPublisher(pool, fixedClock{}).Publish(context.Background(), spec.Publication{
			Spec: second, By: "usr_ana", Company: "acme", Guard: spec.Guard{Allow: inScope(cx)},
		}))
	}()
	waitForLockWait(t, pool, "%agent_state%", done)
	commit()

	if err := <-done; !errors.Is(err, spec.ErrAgentNotFound) {
		t.Fatalf("Publish = %v, want ErrAgentNotFound: the agent now lives in ops", err)
	}
}

func TestRetire_ifMatchAnOlderTag_refusesAndLeavesItLive(t *testing.T) {
	state, _ := retirable(t)
	ctx := t.Context()
	live := published(t, definition)

	err := state.Retire(ctx, "triage", "usr_ana", spec.Guard{
		Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(live.Version, true)),
	})
	if got := staleAt(t, err); got != domain.AgentRevision(live.Version, false) {
		t.Errorf("current = %s, want the live tag", got)
	}
	if retired, _ := state.Retired(ctx); retired["triage"] {
		t.Error("a refused retirement retired the agent")
	}
}

func TestRestore_ifMatchTheRetiredTag_restores(t *testing.T) {
	state, _ := retirable(t)
	ctx := t.Context()
	live := published(t, definition)
	if err := state.Retire(ctx, "triage", "usr_ana", spec.Guard{Allow: inScope(cx)}); err != nil {
		t.Fatalf("Retire: %v", err)
	}

	if err := state.Restore(ctx, "triage", "usr_ana", spec.Guard{
		Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(live.Version, true)),
	}); err != nil {
		t.Fatalf("Restore = %v, want it restored", err)
	}
	if retired, _ := state.Retired(ctx); retired["triage"] {
		t.Error("the agent is still retired")
	}
}

func TestRetire_callerCannotPublishWhereTheAgentLives_notFoundBeforeThePrecondition(t *testing.T) {
	state, _ := retirable(t)
	ctx := t.Context()

	for name, act := range map[string]func(context.Context, domain.AgentID, domain.UserID, spec.Guard) error{
		"retire": state.Retire, "restore": state.Restore,
	} {
		err := act(ctx, "triage", "usr_ana", spec.Guard{
			Allow: inScope(domain.Scope{Company: "acme", Area: "ops"}), Expect: ifMatch(t, `"v1-stale"`),
		})
		if !errors.Is(err, spec.ErrAgentNotFound) {
			t.Errorf("%s = %v, want ErrAgentNotFound", name, err)
		}
	}
	if retired, _ := state.Retired(ctx); retired["triage"] {
		t.Error("a refused retirement retired the agent")
	}
}

func TestRetire_anAgentNobodyPublished_isNotFound(t *testing.T) {
	state, _ := retirable(t)

	err := state.Retire(t.Context(), "nobody", "usr_ana", spec.Guard{Allow: inScope(cx)})
	if !errors.Is(err, spec.ErrAgentNotFound) {
		t.Fatalf("Retire = %v, want ErrAgentNotFound", err)
	}
}

// A publication committed while the retirement waits moves the tag it judges.
func TestRetire_publicationCommittedWhileTheRetirementWaits_isStale(t *testing.T) {
	state, registry := retirable(t)
	pool := statePool(t)
	live := published(t, definition)
	next := published(t, strings.Replace(definition,
		"Read the ticket and classify it.", "Read the ticket twice and classify it.", 1))
	if err := registry.Publish(t.Context(), next, "usr_ana", "acme"); err != nil {
		t.Fatalf("publish next: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into agent_state (agent_id, paused, current_version, changed_at) values ('triage', true, $1, now())
		on conflict (agent_id) do update set current_version = excluded.current_version`, string(live.Version)); err != nil {
		t.Fatalf("make the first current: %v", err)
	}
	commit := held(t, pool, `update agent_state set current_version = $1 where agent_id = 'triage'`, string(next.Version))

	done := make(chan error, 1)
	go func() {
		done <- state.Retire(context.Background(), "triage", "usr_ana", spec.Guard{
			Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(live.Version, false)),
		})
	}()
	waitForLockWait(t, pool, "%agent_state%", done)
	commit()

	if got := staleAt(t, <-done); got != domain.AgentRevision(next.Version, false) {
		t.Errorf("current = %s, want the version the competitor made current", got)
	}
}

// statePool is a second connection to the database retirable prepared,
// without resetting it.
func statePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), testdb.DSN(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

/*
The guard holds the agent's row until the write commits, not only while it
reads it. A retirement arriving after the publication judged its tag waits for
the publication to commit, so it never lands between the judgement and the
write. The retirement runs on a connection of its own, named, and the watch
counts only that backend queued on a lock.
*/
func TestPublisher_aRetirementArrivingAfterTheGuard_waitsForThePublicationToCommit(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)
	ctx := context.Background()
	competitor := namedConn(t, "nt018-guard-competitor")

	finished := make(chan error, 1)
	publisher := spec.PauseAfterJudging(spec.NewPublisher(pool, fixedClock{}), func() {
		go func() {
			_, err := competitor.Exec(ctx,
				`update agent_state set retired_at = now() where agent_id = $1`, string(first.ID))
			finished <- err
		}()
		waitForBackendLockWait(t, pool, "nt018-guard-competitor", finished)
	})
	if _, err := publisher.Publish(ctx, spec.Publication{
		Spec: second, By: "usr_ana", Company: "acme",
		Guard: spec.Guard{Allow: inScope(cx), Expect: ifMatch(t, domain.AgentRevision(first.Version, false))},
	}); err != nil {
		t.Fatalf("Publish = %v, want it published before the retirement", err)
	}
	if err := <-finished; err != nil {
		t.Fatalf("the retirement, after the publication: %v", err)
	}
}

func namedConn(t *testing.T, application string) *pgx.Conn {
	t.Helper()
	config, err := pgx.ParseConfig(testdb.DSN(t))
	if err != nil {
		t.Fatalf("parse the DSN: %v", err)
	}
	config.RuntimeParams["application_name"] = application
	conn, err := pgx.ConnectConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("connect %s: %v", application, err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// waitForBackendLockWait returns once the named backend is queued on a lock,
// and fails if its write finished first.
func waitForBackendLockWait(t *testing.T, pool *pgxpool.Pool, application string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("%s finished (%v) without waiting for the guard's lock", application, err)
		default:
		}
		var waiting int
		if err := pool.QueryRow(context.Background(), `
			select count(*) from pg_stat_activity
			where wait_event_type = 'Lock' and application_name = $1`, application).Scan(&waiting); err != nil {
			t.Fatalf("read lock waits: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never queued on the guard's lock", application)
}
