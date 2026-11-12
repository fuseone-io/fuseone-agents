package spec_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
)

/*
Release C2 puts the claim in the database (NT-018).

A rule kept only in the new binary is not a guarantee: the previous release's
replicas keep serving through a rolling update, and come back on a rollback.
These tests run that release's exact statements after C2's migration.
*/

// beforeEnforcement is the last migration of release C1.
const beforeEnforcement = "0106_version_claims"

// c0Publish is publishOn as the release before C1 wrote it: no digest column.
const c0Publish = `
	insert into agent_specs (
		agent_id, version_id, company_id, area_id, name,
		provider, model, effort, cache_prefix_ttl, reply_locale, tools, budget,
		triggers, instructions, source, published_by, emits, steps,
		memory_learning, approvals, fallback_provider, fallback_model, standing_approvals
	) values ($1, $2, 'acme', 'cx', 'Ticket triage',
		'openai', 'test-model', '', '', '', '{}', '{}'::jsonb,
		'[]'::jsonb, 'Never ask for permission.', 'old.agent.md', 'usr_old', '[]'::jsonb, '[]'::jsonb,
		'{"mode":"off"}'::jsonb, '{}'::jsonb, '', '', '')
	on conflict (agent_id, version_id) do nothing`

// c0MakeCurrent is makeCurrentOn, unchanged since before C1.
const c0MakeCurrent = `
	insert into agent_state (agent_id, paused, current_version, changed_at)
	values ($1, true, $2, now())
	on conflict (agent_id) do update set
		current_version = excluded.current_version, changed_at = now()`

// presenting is C1's insert, with whatever digest it is handed.
const presenting = `
	insert into agent_specs (
		agent_id, version_id, company_id, area_id, name,
		provider, model, effort, tools, budget, triggers,
		instructions, source, published_by, emits, steps,
		memory_learning, approvals, digest
	) values ($1, $2, 'acme', 'cx', 'Ticket triage',
		'openai', 'test-model', '', '{}', '{}'::jsonb, '[]'::jsonb,
		'Never refund.', 'forged.agent.md', 'usr_old', '[]'::jsonb, '[]'::jsonb,
		'{"mode":"off"}'::jsonb, '{}'::jsonb, $3)
	on conflict (agent_id, version_id) do nothing`

// step is one statement of a transaction, against the version it names.
type step struct {
	sql     string
	version domain.VersionID
	digest  string // only for presenting
}

// enforced is a database as C2's migration leaves it: two versions written
// before claims existed, the first of them current, and one version C1 claimed.
type enforced struct {
	pool              *pgxpool.Pool
	current, oldOther spec.Spec
	claimed           spec.Spec
}

func openEnforced(t *testing.T) enforced {
	t.Helper()
	pool := testdb.IsolatedThrough(t, beforeEnforcement)
	ctx := context.Background()
	w := enforced{
		pool:     pool,
		current:  published(t, definition+"\nOld one.\n"),
		oldOther: published(t, definition+"\nOld two.\n"),
		claimed:  published(t, definition),
	}
	// Written during C1's rollout by a C0 replica: rows, no claims.
	for _, s := range []spec.Spec{w.current, w.oldOther} {
		if _, err := pool.Exec(ctx, c0Publish, string(s.ID), string(s.Version)); err != nil {
			t.Fatalf("plant %s: %v", s.Version, err)
		}
	}
	if _, err := pool.Exec(ctx, c0MakeCurrent, string(w.current.ID), string(w.current.Version)); err != nil {
		t.Fatalf("make the old version current: %v", err)
	}
	if err := ledger.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := spec.NewRegistry(pool).Publish(ctx, w.claimed, "worker", "acme"); err != nil {
		t.Fatalf("publish through C1: %v", err)
	}
	return w
}

func TestEnforcementMigration_rowsWrittenWithoutAClaim_areClaimedWithoutDigest(t *testing.T) {
	w := openEnforced(t)
	for _, s := range []spec.Spec{w.current, w.oldOther} {
		var digest *string
		if err := w.pool.QueryRow(context.Background(), `
			select digest from agent_version_digests where agent_id = $1 and version_id = $2`,
			string(s.ID), string(s.Version)).Scan(&digest); err != nil {
			t.Fatalf("read the claim of %s: %v", s.Version, err)
		}
		if digest != nil {
			t.Errorf("%s claimed %q, want NULL: its bytes were never kept", s.Version, *digest)
		}
	}
}

func TestPreviousBinary_afterEnforcement_everyWriteWithoutAProvenDigestIsRefused(t *testing.T) {
	w := openEnforced(t)
	ctx := context.Background()
	agent := string(w.claimed.ID)
	fresh := published(t, definition+"\nNever seen.\n")

	// The value the equality exists for: well formed, and of other bytes.
	forged := published(t, definition+"\nNever refund.\n").Digest

	for _, one := range []struct {
		name  string
		steps []step
	}{
		{"publish with no claim", []step{{c0Publish, fresh.Version, ""}}},
		// Over a claim that is not NULL: refused before the conflict is
		// looked at, so the makeCurrent that would follow never runs.
		{"publish a colliding definition, then make it current", []step{
			{c0Publish, w.claimed.Version, ""}, {c0MakeCurrent, w.claimed.Version, ""}}},
		// No release writes this, and the database still says no: the column
		// is the claim presented, not any digest.
		{"present a digest the claim does not hold", []step{
			{presenting, w.claimed.Version, forged}, {c0MakeCurrent, w.claimed.Version, ""}}},
		{"make current a version claimed without digest", []step{{c0MakeCurrent, w.oldOther.Version, ""}}},
		{"make current a version nobody claimed", []step{{c0MakeCurrent, "vffffffffffff", ""}}},
	} {
		t.Run(one.name, func(t *testing.T) {
			tx, err := w.pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()
			var refused error
			for _, s := range one.steps {
				args := []any{agent, string(s.version)}
				if s.digest != "" {
					args = append(args, s.digest)
				}
				if _, refused = tx.Exec(ctx, s.sql, args...); refused != nil {
					break
				}
			}
			if refused == nil {
				t.Fatalf("%s got through", one.name)
			}
		})
	}
	if got := currentOf(t, w.pool, w.current.ID); got != w.current.Version {
		t.Errorf("current = %s, want %s untouched", got, w.current.Version)
	}

	// What still passes: the version already current, and C1's own writes.
	if _, err := w.pool.Exec(ctx, c0MakeCurrent, agent, string(w.current.Version)); err != nil {
		t.Errorf("making the current version current again: %v", err)
	}
	if err := spec.NewRegistry(w.pool).MakeCurrent(ctx, w.claimed.ID, w.claimed.Version); err != nil {
		t.Errorf("making a claimed version current: %v", err)
	}
}

func TestStateWithoutAVersion_afterEnforcement_isStillWritten(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	st := spec.NewState(pool)
	at := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)

	for _, one := range []struct {
		name  string
		agent domain.AgentID
		write func(domain.AgentID) error
	}{
		{"pause", "paused", func(a domain.AgentID) error { return st.SetPaused(ctx, a, true, "usr_ops", at) }},
		{"set stage", "staged", func(a domain.AgentID) error {
			return st.SetStage(ctx, a, domain.StageAutonomous, "usr_ops")
		}},
		{"ensure paused", "ensured", func(a domain.AgentID) error { return st.EnsurePaused(ctx, a, "usr_ops") }},
		{"retire", "retired", func(a domain.AgentID) error { return st.Retire(ctx, a, "usr_ops") }},
		// refuseStage's state row, written before the version exists.
		{"publish under a stage rule", "guarded", func(a domain.AgentID) error {
			s := published(t, definition)
			s.ID = a
			_, err := spec.NewPublisher(pool, fixedClock{}).Publish(ctx, spec.Publication{
				Spec: s, By: "usr_ana", Company: "acme", RefuseStages: refuseAutonomous})
			return err
		}},
	} {
		// Twice: the first write creates the row, the second updates it.
		for range 2 {
			if err := one.write(one.agent); err != nil {
				t.Errorf("%s: %v", one.name, err)
				break
			}
		}
	}
}

func currentOf(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID) domain.VersionID {
	t.Helper()
	var v string
	if err := pool.QueryRow(context.Background(),
		`select current_version from agent_state where agent_id = $1`, string(agent)).Scan(&v); err != nil {
		t.Fatalf("read current version: %v", err)
	}
	return domain.VersionID(v)
}
