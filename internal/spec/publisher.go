package spec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
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
	pool     *pgxpool.Pool
	registry *Registry
	state    *State
	clock    Clock
}

// Clock is when a pause was decided.
type Clock interface{ Now() time.Time }

func NewPublisher(pool *pgxpool.Pool, clock Clock) *Publisher {
	return &Publisher{pool: pool, registry: NewRegistry(pool), state: NewState(pool), clock: clock}
}

// ErrStageRefused is a publication refused because the agent stands at a
// stage the publisher asked to refuse. Nothing was written.
var ErrStageRefused = errors.New("spec: the agent stands at a stage this publication refuses")

// Publication is one version to publish, and the condition it is published
// under.
type Publication struct {
	Spec    Spec
	By      domain.UserID
	Company domain.CompanyID

	// RefuseStages refuses the publication while the agent stands at any of
	// these. Empty refuses nothing, which is how the console publishes.
	//
	// A caller that read the stage first and then published would leave a
	// window between the two in which a promotion lands, and an autonomous
	// agent runs whatever version is current. So the stage is judged here,
	// under the agent's row lock, in the transaction that writes the version.
	RefuseStages []domain.Stage
}

// Publish writes the version and makes it current, in one transaction.
func (p *Publisher) Publish(ctx context.Context, pub Publication) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("spec: publish %s: begin: %w", pub.Spec.ID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := refuseStage(ctx, tx, pub.Spec.ID, pub.RefuseStages); err != nil {
		return err
	}
	if err := publishOn(ctx, tx, pub.Spec, pub.By, pub.Company); err != nil {
		return err
	}
	if err := makeCurrentOn(ctx, tx, pub.Spec.ID, pub.Spec.Version); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("spec: publish %s: commit: %w", pub.Spec.ID, err)
	}
	return nil
}

/*
refuseStage locks the agent's state row, then judges its stage.

Lock first, judge second. The row is created if the agent has none, paused and
at draft as every new agent is, so the lock always has a row to land on: a
read of an absent row locks nothing, and a promotion inserting it could commit
in between. `select … for update` then waits for any transaction holding the
row — SetStage among them — and, under read committed, returns the row as that
transaction committed it, not the snapshot from before the wait. A promotion
that arrives after the lock waits for this transaction instead, and happens to
an agent whose current version is already the reviewed one.
*/
func refuseStage(ctx context.Context, tx pgx.Tx, agent domain.AgentID, refused []domain.Stage) error {
	if len(refused) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		insert into agent_state (agent_id, paused, changed_at) values ($1, true, now())
		on conflict (agent_id) do nothing`, string(agent)); err != nil {
		return fmt.Errorf("spec: publish %s: ensure state: %w", agent, err)
	}
	var stage string
	if err := tx.QueryRow(ctx,
		`select stage from agent_state where agent_id = $1 for update`, string(agent)).Scan(&stage); err != nil {
		return fmt.Errorf("spec: publish %s: lock state: %w", agent, err)
	}
	if slices.Contains(refused, domain.StageOf(stage)) {
		return fmt.Errorf("%w: %s is %s", ErrStageRefused, agent, stage)
	}
	return nil
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
