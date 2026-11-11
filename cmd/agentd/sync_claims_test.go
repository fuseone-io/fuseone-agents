package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
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
