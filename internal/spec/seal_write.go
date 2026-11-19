package spec

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
What sealing a draft needs from the registry, in the seal's own transaction (NT-018
phase 2, release 2b).

The caller holds the event graph shared (ShareEventGraph): every writer of agent_specs
holds it exclusive, so the claim's search for a row nobody claimed stays race-free, as it
is under a publication, while seals do not wait for each other.
*/

// ClaimForSeal claims s's version id for its bytes, as a publication does. A collision is
// ErrVersionCollision and a version published before digests were kept is
// ErrDigestUnknown.
func ClaimForSeal(ctx context.Context, tx pgx.Tx, s Spec) error {
	return claimVersion(ctx, tx, s)
}

// PublishedIn reports whether agent already published version. Once its claim holds,
// those are the same bytes.
func PublishedIn(ctx context.Context, tx pgx.Tx, agent domain.AgentID, version domain.VersionID) (bool, error) {
	var found bool
	if err := tx.QueryRow(ctx, `
		select exists (select 1 from agent_specs where agent_id = $1 and version_id = $2)`,
		string(agent), string(version)).Scan(&found); err != nil {
		return false, fmt.Errorf("spec: look for %s@%s: %w", agent, version, err)
	}
	return found, nil
}

// JudgeIn is the event cycles s would close, judged against the graph as tx reads it. It
// is informational: activation judges again under the lock.
func JudgeIn(ctx context.Context, tx pgx.Tx, s Spec) (Published, error) {
	wiring, err := (&Registry{q: tx}).wiring(ctx)
	if err != nil {
		return Published{}, fmt.Errorf("spec: judge %s: %w", s.ID, err)
	}
	return JudgedIn(WiringWith(wiring, s), s.ID), nil
}
