package httpapi

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
Replaying needs two things from two different stores: the policy set a decision
was made under, and the capability pack of the version that ran.

Joined here rather than at the call site because they are one question — "what
was in force when this happened" — and a handler that took two arguments would
invite one of them being wired and the other forgotten.
*/
type replayPorts struct {
	snapshots func(ctx context.Context, hash string) ([]domain.Policy, error)
	packs     func(ctx context.Context, agent domain.AgentID, version domain.VersionID) (gate.Pack, error)
}

// NewReplays joins the two stores a faithful replay reads.
func NewReplays(
	snapshots func(ctx context.Context, hash string) ([]domain.Policy, error),
	packs func(ctx context.Context, agent domain.AgentID, version domain.VersionID) (gate.Pack, error),
) Replays {
	return replayPorts{snapshots: snapshots, packs: packs}
}

func (r replayPorts) Snapshot(ctx context.Context, hash string) ([]domain.Policy, error) {
	return r.snapshots(ctx, hash)
}

func (r replayPorts) Pack(ctx context.Context, agent domain.AgentID, version domain.VersionID) (gate.Pack, error) {
	return r.packs(ctx, agent, version)
}
