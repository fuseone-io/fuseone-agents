package ledger_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/testdb"
)

// A candidate is sealed bytes outside agent_specs (NT-018 phase 2). Release 2a
// only teaches every reader the shape: nothing writes a seal yet, so these
// tests write the rows the way release 2b will.

func claimVersion(t *testing.T, pool *pgxpool.Pool, agent, version string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		insert into agent_version_digests (agent_id, version_id, digest)
		values ($1, $2, repeat('a', 64)) on conflict do nothing`, agent, version); err != nil {
		t.Fatalf("claim %s/%s: %v", agent, version, err)
	}
}

func insertSeal(ctx context.Context, pool *pgxpool.Pool, agent, version string) (string, error) {
	var seal string
	err := pool.QueryRow(ctx, `
		insert into agent_seals
		  (seal_id, agent_id, version_id, company_id, area_id, content_ref,
		   draft_id, draft_revision, sealed_by)
		values (gen_random_uuid(), $1, $2, 'acme', 'cx', 'ref-1',
		        gen_random_uuid(), 'rev-1', 'usr_ana')
		returning seal_id::text`, agent, version).Scan(&seal)
	return seal, err
}

func TestAgentSeals_aSealNamesAClaimedVersion(t *testing.T) {
	pool := testdb.Pool(t)
	agent := "seal-" + t.Name()
	if _, err := insertSeal(t.Context(), pool, agent, "unclaimed"); err == nil {
		t.Fatal("a seal of a version nobody claimed was accepted")
	}
	claimVersion(t, pool, agent, "v1")
	if _, err := insertSeal(t.Context(), pool, agent, "v1"); err != nil {
		t.Fatalf("seal of a claimed version: %v", err)
	}
}

func TestAgentSeals_sealsAndTheirSourcesAreInsertOnly(t *testing.T) {
	pool := testdb.Pool(t)
	agent := "seal-" + t.Name()
	claimVersion(t, pool, agent, "v1")
	seal, err := insertSeal(t.Context(), pool, agent, "v1")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		insert into agent_seal_sources (seal_id, part, trail_seq)
		values ($1::uuid, 'tools/crm.lookup', 3), ($1::uuid, 'block/b1', null)`, seal); err != nil {
		t.Fatalf("seal sources, one unknown: %v", err)
	}
	for _, change := range []string{
		`update agent_seals set content_ref = 'other' where seal_id = $1::uuid`,
		`delete from agent_seals where seal_id = $1::uuid`,
		`update agent_seal_sources set trail_seq = 4 where seal_id = $1::uuid`,
		`delete from agent_seal_sources where seal_id = $1::uuid`,
	} {
		if _, err := pool.Exec(t.Context(), change, seal); err == nil {
			t.Errorf("%q succeeded, want seals to be permanent", change)
		}
	}
}
