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
	// afterJudging runs between the graph's judgement and the version's
	// write. Nil outside tests; set by export_test.go to hold a publication
	// there while another one races it.
	afterJudging func()
	// schedules and webhooks move with every version this publisher makes current, in
	// its transaction (#416). Nil reconciles nothing.
	schedules ScheduleSync
	webhooks  WebhookSync
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

	// Guard is judged under the agent's row lock, before anything else about
	// the agent (NT-017 phase 1).
	Guard Guard

	// Within, when set, runs in the publication's transaction once the
	// version is current and before the commit: under the event graph's lock
	// and the agent's row, so what must move with the version moves in the
	// same commit (NT-018, contract 4). An error undoes the publication.
	Within func(ctx context.Context, tx pgx.Tx, version domain.VersionID) error
}

// Published is what a publication found on the way: the event cycles the
// version closes, judged against the graph it is written into (#227).
type Published struct {
	EventCycles domain.EventCycleSearch
	// Scopes is where each agent named in EventCycles lives, so the answer
	// can hide an agent its reader may not read. The cycles are judged across
	// every scope; who is told which names is the caller's to decide.
	Scopes map[domain.AgentID]domain.Scope
	// Paused is whether the agent is paused, as the publication's transaction left it.
	Paused bool
}

/*
Publish writes the version and makes it current, in one transaction, and
judges the event cycles it closes in the same one.

The judgement is taken under the event graph's lock, which every write that
can change a current version takes too (lockEventGraph). Read outside it, two
publications closing a cycle only together could each judge the graph from
before the other and both publish unwarned. Under it, the second waits for
the first to commit and judges the graph the first left. The lock comes
first, before the agent's own row, so every writer takes the two in one order.
*/
func (p *Publisher) Publish(ctx context.Context, pub Publication) (Published, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Published{}, fmt.Errorf("spec: publish %s: begin: %w", pub.Spec.ID, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockEventGraph(ctx, tx); err != nil {
		return Published{}, fmt.Errorf("spec: publish %s: %w", pub.Spec.ID, err)
	}
	// Before anything else about the agent, the claim included: a collision
	// answered to a caller who may not reach the agent would say it exists.
	if _, err := guardAgent(ctx, tx, pub.Spec.ID, pub.Guard); err != nil {
		return Published{}, err
	}
	// Before anything is judged: a judgement of these bytes means nothing if
	// the id they would be published under already names others.
	if err := claimVersion(ctx, tx, pub.Spec); err != nil {
		return Published{}, err
	}
	if err := refuseStage(ctx, tx, pub.Spec.ID, pub.RefuseStages); err != nil {
		return Published{}, err
	}
	judged, err := p.judge(ctx, tx, inCompany(pub.Spec, pub.Company))
	if err != nil {
		return Published{}, err
	}
	if judged.Paused, err = p.writeOn(ctx, tx, pub); err != nil {
		return Published{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Published{}, fmt.Errorf("spec: publish %s: commit: %w", pub.Spec.ID, err)
	}
	return judged, nil
}

// judge reads the graph inside the publication's transaction and finds the
// cycles the version closes in it, replacing its agent's current version.
func (p *Publisher) judge(ctx context.Context, tx pgx.Tx, s Spec) (Published, error) {
	wiring, err := p.registry.within(tx).wiring(ctx)
	if err != nil {
		return Published{}, fmt.Errorf("spec: publish %s: %w", s.ID, err)
	}
	judged := JudgedIn(WiringWith(wiring, s), s.ID)
	if p.afterJudging != nil {
		p.afterJudging()
	}
	return judged, nil
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

func (p *Publisher) SetPaused(
	ctx context.Context, agent domain.AgentID, paused bool, by domain.UserID,
) error {
	return p.state.SetPaused(ctx, agent, paused, by, p.clock.Now())
}

func (p *Publisher) IsPaused(ctx context.Context, agent domain.AgentID) (bool, error) {
	return p.state.IsPaused(ctx, agent)
}

// JudgedIn finds the cycles through the agent and where each agent in them
// lives.
func JudgedIn(w domain.EventWiring, agent domain.AgentID) Published {
	out := Published{EventCycles: domain.EventCyclesThrough(w, agent), Scopes: map[domain.AgentID]domain.Scope{}}
	for _, cycle := range out.EventCycles.Cycles {
		for _, a := range cycle {
			out.Scopes[a] = w[a].Scope
		}
	}
	return out
}

// inCompany is the version as it will be written: a file names no company,
// and the publication supplies the one it is published into.
func inCompany(s Spec, company domain.CompanyID) Spec {
	if s.Company == "" {
		s.Company = company
	}
	return s
}

// writeOn writes the version, makes it current, runs the publication's own condition and
// moves what the version declares, all in tx. It answers whether the agent is paused there.
func (p *Publisher) writeOn(ctx context.Context, tx pgx.Tx, pub Publication) (bool, error) {
	if err := publishOn(ctx, tx, pub.Spec, pub.By, pub.Company); err != nil {
		return false, err
	}
	if err := makeCurrentOn(ctx, tx, pub.Spec.ID, pub.Spec.Version); err != nil {
		return false, err
	}
	if pub.Within != nil {
		if err := pub.Within(ctx, tx, pub.Spec.Version); err != nil {
			return false, err
		}
	}
	if err := p.reconcile(ctx, tx, inCompany(pub.Spec, pub.Company)); err != nil {
		return false, err
	}
	return pausedIn(ctx, tx, pub.Spec.ID)
}
