package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/vault"
)

const syncedDefinition = `---
id: %s
name: Synced
area: cx
provider: openai
model: test-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
---

Read the ticket.
`

/*
The disk sync republishes every version on disk at each boot (NT-018).

A version whose id was claimed without a digest cannot be published again, and
the sync must not take the worker down for it: the version stays where it was,
and the boot goes on with every other definition. The digest it records for the
rest is the SHA-256 of the file's own bytes, CRLF and all, never of a Spec
written back out.
*/
func TestPublishSpecs_aVersionClaimedWithoutDigest_isSkippedAndTheBootGoesOn(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	dir := t.TempDir()

	legacy := []byte(strings.Replace(syncedDefinition, "%s", "legacy", 1))
	crlf := []byte(strings.ReplaceAll(strings.Replace(syncedDefinition, "%s", "fresh", 1), "\n", "\r\n"))
	for name, bytes := range map[string][]byte{"legacy.agent.md": legacy, "fresh.agent.md": crlf} {
		if err := os.WriteFile(filepath.Join(dir, name), bytes, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	old, err := spec.Parse("legacy.agent.md", legacy)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, null)`,
		string(old.ID), string(old.Version)); err != nil {
		t.Fatalf("plant the null claim: %v", err)
	}

	booted, err := publishSpecs(ctx, spec.NewRegistry(pool), dir)
	if err != nil {
		t.Fatalf("publishSpecs = %v, want the boot to go on", err)
	}
	if booted.published != 1 || booted.skipped != 1 {
		t.Errorf("published %d and skipped %d, want one of each", booted.published, booted.skipped)
	}
	var claimed string
	if err := pool.QueryRow(ctx,
		`select digest from agent_version_digests where agent_id = 'fresh'`).Scan(&claimed); err != nil {
		t.Fatalf("read the fresh claim: %v", err)
	}
	sum := sha256.Sum256(crlf)
	if want := hex.EncodeToString(sum[:]); claimed != want {
		t.Errorf("claimed %q, want the SHA-256 of the file's bytes %q", claimed, want)
	}
}

/*
The triggers come from the version the boot left current.

The sync publishes every version on disk in order and makes each current that
it can publish, so the version left current is the last one it could prove,
not the newest file. Reconciling schedules and webhook paths from the newest
file would leave an agent running A with B's triggers, or with triggers nobody
declared, whenever the newest was skipped.
*/
func TestBoot_theNewestFileSkippedForItsDigest_triggersFollowTheVersionLeftCurrent(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	dir := t.TempDir()

	a := withTriggers(t, dir, "a-legacy.agent.md", "0 1 * * *", "/hooks/a")
	b := withTriggers(t, dir, "b-legacy.agent.md", "0 3 * * *", "/hooks/b")
	// B as the migration leaves a version published before claims: its row,
	// and a claim without a digest.
	plantLegacy(t, pool, b)

	boot(t, pool, dir)

	var current string
	if err := pool.QueryRow(ctx,
		`select current_version from agent_state where agent_id = 'legacy'`).Scan(&current); err != nil {
		t.Fatalf("read the current version: %v", err)
	}
	if current != string(a.Version) {
		t.Errorf("current = %s, want A (%s), the last version the boot could publish", current, a.Version)
	}
	if got := schedulesOf(t, pool); got != "0 1 * * *" {
		t.Errorf("schedules = %q, want A's", got)
	}
	if got := pathsOf(t, pool); got != "hooks/a" {
		t.Errorf("paths = %q, want A's", got)
	}
}

// When no version of an agent on disk can be published, the boot decided
// nothing for it, and its triggers stay as they were.
func TestBoot_everyFileSkippedForItsDigest_changesNoTrigger(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	dir := t.TempDir()

	b := withTriggers(t, dir, "legacy.agent.md", "0 3 * * *", "/hooks/b")
	plantLegacy(t, pool, b)
	if err := trigger.NewPostgresSchedules(pool).Sync(ctx, b.ID, []string{"*/15 * * * *"}, time.Now()); err != nil {
		t.Fatalf("seed the running schedule: %v", err)
	}

	boot(t, pool, dir)

	if got := schedulesOf(t, pool); got != "*/15 * * * *" {
		t.Errorf("schedules = %q, want the running one unchanged", got)
	}
	if got := pathsOf(t, pool); got != "" {
		t.Errorf("paths = %q, want none from the skipped definition", got)
	}
}

// withTriggers writes a definition of agent "legacy" with one cron and one path.
func withTriggers(t *testing.T, dir, name, cron, path string) spec.Spec {
	t.Helper()
	bytes := []byte(strings.Replace(strings.Replace(syncedDefinition, "%s", "legacy", 1),
		"budget:", "triggers:\n  - { type: cron, schedule: \""+cron+"\" }\n"+
			"  - { type: webhook, path: "+path+" }\nbudget:", 1))
	if err := os.WriteFile(filepath.Join(dir, name), bytes, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	s, err := spec.Parse(name, bytes)
	if err != nil {
		t.Fatalf("Parse %s: %v", name, err)
	}
	return s
}

// plantLegacy is a version as release C1's migration finds it: the row, then a
// NULL claim.
func plantLegacy(t *testing.T, pool *pgxpool.Pool, s spec.Spec) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		insert into agent_specs (
			agent_id, version_id, company_id, area_id, name,
			provider, model, effort, tools, budget, triggers,
			instructions, source, published_by, emits, steps,
			memory_learning, approvals
		) values ($1, $2, 'acme', 'cx', 'Synced',
			'openai', 'test-model', '', '{}', '{}'::jsonb, '[]'::jsonb,
			'old text', 'old.agent.md', 'worker', '[]'::jsonb,
			'[]'::jsonb, '{"mode":"off"}'::jsonb, '{}'::jsonb)`,
		string(s.ID), string(s.Version)); err != nil {
		t.Fatalf("plant the legacy row: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, null)`,
		string(s.ID), string(s.Version)); err != nil {
		t.Fatalf("plant the null claim: %v", err)
	}
}

// boot runs the worker's start-up publication, as worker_cmd does.
func boot(t *testing.T, pool *pgxpool.Pool, dir string) {
	t.Helper()
	v, err := vault.New(make([]byte, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	parts := &workerParts{
		configPool:   pool,
		registry:     spec.NewRegistry(pool),
		catalog:      tools.NewCatalog(nil),
		integrations: admin.NewIntegrations(pool, settings.NewStore(pool, v)),
	}
	if _, err := parts.loadAndPublish(context.Background(), dir); err != nil {
		t.Fatalf("boot: %v", err)
	}
}

func schedulesOf(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	return joined(t, pool, `select schedule from trigger_schedules where agent_id = 'legacy' order by 1`)
}

func pathsOf(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	return joined(t, pool, `select path from webhook_triggers where agent_id = 'legacy' order by 1`)
}

func joined(t *testing.T, pool *pgxpool.Pool, query string) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var one string
		if err := rows.Scan(&one); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, one)
	}
	return strings.Join(out, ",")
}
