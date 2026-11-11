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

	published, skipped, err := publishSpecs(ctx, spec.NewRegistry(pool), dir)
	if err != nil {
		t.Fatalf("publishSpecs = %v, want the boot to go on", err)
	}
	if published != 1 || skipped != 1 {
		t.Errorf("published %d and skipped %d, want one of each", published, skipped)
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
A skipped version reaches no trigger either.

The boot publishes, then reconciles cron schedules and webhook paths from the
files. If those read the file the registry refused, a definition nobody could
prove would install its schedule and its path while every run executes the row
the database kept. So the triggers of an agent whose version was skipped stay
as they were.
*/
func TestBoot_aVersionSkippedForItsDigest_changesNoTrigger(t *testing.T) {
	pool := testdb.Isolated(t)
	ctx := context.Background()
	dir := t.TempDir()

	// B, on disk: a cron and a path the row the database holds never declared.
	unproven := []byte(strings.Replace(strings.Replace(syncedDefinition, "%s", "legacy", 1),
		"budget:", "triggers:\n  - { type: cron, schedule: \"0 3 * * *\" }\n"+
			"  - { type: webhook, path: /hooks/unproven }\nbudget:", 1))
	if err := os.WriteFile(filepath.Join(dir, "legacy.agent.md"), unproven, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	b, err := spec.Parse("legacy.agent.md", unproven)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, null)`,
		string(b.ID), string(b.Version)); err != nil {
		t.Fatalf("plant the null claim: %v", err)
	}
	// A's schedule, as the installation runs it today.
	if err := trigger.NewPostgresSchedules(pool).Sync(ctx, b.ID, []string{"*/15 * * * *"}, time.Now()); err != nil {
		t.Fatalf("seed A's schedule: %v", err)
	}

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
	if _, err := parts.loadAndPublish(ctx, dir); err != nil {
		t.Fatalf("boot: %v", err)
	}

	var schedules []string
	rows, err := pool.Query(ctx, `select schedule from trigger_schedules where agent_id = 'legacy' order by 1`)
	if err != nil {
		t.Fatalf("read schedules: %v", err)
	}
	for rows.Next() {
		var one string
		if err := rows.Scan(&one); err != nil {
			t.Fatalf("scan: %v", err)
		}
		schedules = append(schedules, one)
	}
	if strings.Join(schedules, ",") != "*/15 * * * *" {
		t.Errorf("schedules = %v, want A's unchanged", schedules)
	}
	var paths int
	if err := pool.QueryRow(ctx,
		`select count(*) from webhook_triggers where agent_id = 'legacy'`).Scan(&paths); err != nil {
		t.Fatalf("read webhooks: %v", err)
	}
	if paths != 0 {
		t.Errorf("%d webhook paths, want none from the skipped definition", paths)
	}
}
