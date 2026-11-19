package spec_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// The seal reader takes the row and the claim's digest from the database as release 2b
// will write them, NULL claims and all.
func TestSealRows_readsTheRowAndItsClaim(t *testing.T) {
	pool := openSpecPool(t)
	ctx := context.Background()
	agent := "seal-rows-" + t.Name()
	const digest = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	if _, err := pool.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest)
		values ($1, 'v1', $2), ($1, 'v2', null)`, agent, digest); err != nil {
		t.Fatalf("claims: %v", err)
	}
	insert := func(seal, version string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			insert into agent_seals (seal_id, agent_id, version_id, company_id, area_id,
			  content_ref, draft_id, draft_revision, sealed_by)
			values ($1::uuid, $2, $3, 'acme', 'cx', 'authoring://seal/0/abc',
			  gen_random_uuid(), 'rev-1', 'usr_ana')`, seal, agent, version); err != nil {
			t.Fatalf("seal %s: %v", version, err)
		}
	}
	const withDigest, withoutDigest = "2d9e5a74-0c65-4d77-9c5a-2f8c5e7a1a33", "3eaf6b85-1d76-4e88-ad6b-3a9d6f8b2b44"
	insert(withDigest, "v1")
	insert(withoutDigest, "v2")
	rows := spec.NewSealRows(pool)

	got, err := rows.Seal(ctx, withDigest)
	want := spec.SealRow{
		Agent: domain.AgentID(agent), Version: "v1", Scope: domain.Scope{Company: "acme", Area: "cx"},
		ContentRef: "authoring://seal/0/abc", Digest: digest,
	}
	if err != nil || got != want {
		t.Fatalf("Seal = %+v (%v), want %+v", got, err, want)
	}
	if got, err := rows.Seal(ctx, withoutDigest); err != nil || got.Digest != "" {
		t.Fatalf("a NULL claim read as %+v (%v), want an empty digest", got, err)
	}
	if _, err := rows.Seal(ctx, "4fb07c96-2e87-4f99-be7c-4bae7f9c3c55"); !errors.Is(err, spec.ErrSealNotFound) {
		t.Fatalf("unknown seal: err = %v, want ErrSealNotFound", err)
	}
}
