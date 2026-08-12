package spec

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// Publisher is authoring and operations behind one door, for callers that need
// both.
//
// The two stay separate underneath — a version is written by the registry and
// a pause by the state — because they are different kinds of fact. This only
// spares every caller from holding two objects that always travel together.
type Publisher struct {
	registry *Registry
	state    *State
	clock    Clock
}

// Clock is when a pause was decided.
type Clock interface{ Now() time.Time }

func NewPublisher(pool *pgxpool.Pool, clock Clock) *Publisher {
	return &Publisher{registry: NewRegistry(pool), state: NewState(pool), clock: clock}
}

func (p *Publisher) Publish(
	ctx context.Context, s Spec, by domain.UserID, company domain.CompanyID,
) error {
	return p.registry.Publish(ctx, s, by, company)
}

func (p *Publisher) EnsurePaused(ctx context.Context, agent domain.AgentID, by domain.UserID) error {
	return p.state.EnsurePaused(ctx, agent, by)
}

func (p *Publisher) SetPaused(
	ctx context.Context, agent domain.AgentID, paused bool, by domain.UserID,
) error {
	return p.state.SetPaused(ctx, agent, paused, by, p.clock.Now())
}

func (p *Publisher) IsPaused(ctx context.Context, agent domain.AgentID) (bool, error) {
	return p.state.IsPaused(ctx, agent)
}
