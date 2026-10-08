package main

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/standing"
)

// standingPort adapts the standing store to the engine's port, translating
// the store's grant into the three fields a decided step needs.
type standingPort struct{ store *standing.Store }

func (p standingPort) Claim(
	ctx context.Context, tool domain.ToolID, agent domain.AgentID,
	scope domain.Scope, run domain.RunID, at time.Time,
) (engine.StandingGrant, bool, error) {
	grant, covers, err := p.store.Claim(ctx, tool, agent, scope, run, at)
	if err != nil || !covers {
		return engine.StandingGrant{}, false, err
	}
	return engine.StandingGrant{
		ID: grant.ID, By: grant.CreatedBy, Reason: grant.Reason,
	}, true, nil
}
