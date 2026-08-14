package spec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// State is whether an agent runs.
//
// Separate from the registry because it is a different kind of fact. A
// specification is what somebody wrote and never changes; whether it is
// allowed to start changes on an afternoon when something is wrong, and it
// must not create a version between a run and the text that ran.
type State struct{ pool *pgxpool.Pool }

func NewState(pool *pgxpool.Pool) *State { return &State{pool: pool} }

// Paused reports which agents are stopped.
//
// A map rather than a per-agent query: every screen that lists agents needs
// this for all of them, and asking once is the difference between one query
// and one per row.
func (s *State) Paused(ctx context.Context) (map[domain.AgentID]bool, error) {
	rows, err := s.pool.Query(ctx, `select agent_id, paused from agent_state`)
	if err != nil {
		return nil, fmt.Errorf("spec: read agent state: %w", err)
	}
	defer rows.Close()

	out := map[domain.AgentID]bool{}
	for rows.Next() {
		var agent domain.AgentID
		var paused bool
		if err := rows.Scan(&agent, &paused); err != nil {
			return nil, err
		}
		out[agent] = paused
	}
	return out, rows.Err()
}

// Stages reads how far each agent is trusted, by agent.
//
// Read as a set for the same reason pauses are: the worker asks once per pass
// rather than once per run, and a stage that arrived a moment late is a run
// judged under the trust of a minute ago — which is the safe direction, since
// promotion only ever loosens.
func (s *State) Stages(ctx context.Context) (map[domain.AgentID]domain.Stage, error) {
	rows, err := s.pool.Query(ctx, `select agent_id, stage from agent_state`)
	if err != nil {
		return nil, fmt.Errorf("spec: read stages: %w", err)
	}
	defer rows.Close()

	out := map[domain.AgentID]domain.Stage{}
	for rows.Next() {
		var agent, stage string
		if err := rows.Scan(&agent, &stage); err != nil {
			return nil, fmt.Errorf("spec: scan stage: %w", err)
		}
		out[domain.AgentID(agent)] = domain.StageOf(stage)
	}
	return out, rows.Err()
}

// StageOf reads one agent's stage. An agent nobody has staged is a draft.
func (s *State) StageOf(ctx context.Context, agent domain.AgentID) (domain.Stage, error) {
	var stage string
	err := s.pool.QueryRow(ctx,
		`select stage from agent_state where agent_id = $1`, string(agent)).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StageDraft, nil
	}
	if err != nil {
		return domain.StageDraft, fmt.Errorf("spec: read stage of %s: %w", agent, err)
	}
	return domain.StageOf(stage), nil
}

// SetStage records how far an agent is trusted, and who decided.
func (s *State) SetStage(
	ctx context.Context, agent domain.AgentID, stage domain.Stage, by domain.UserID,
) error {
	if !stage.Valid() {
		return fmt.Errorf("spec: %q is not a stage", stage)
	}
	_, err := s.pool.Exec(ctx, `
		insert into agent_state (agent_id, paused, stage, changed_at, changed_by)
		values ($1, true, $2, now(), $3)
		on conflict (agent_id) do update set
			stage = excluded.stage, changed_at = now(), changed_by = excluded.changed_by`,
		string(agent), string(stage), string(by))
	if err != nil {
		return fmt.Errorf("spec: set stage of %s: %w", agent, err)
	}
	return nil
}

// IsPaused reports one agent's state.
//
// An agent nobody has a row for is paused. A new agent is created paused, and
// an absent row means nobody ever decided otherwise — reading that as running
// would let an agent start because a write failed.
func (s *State) IsPaused(ctx context.Context, agent domain.AgentID) (bool, error) {
	var paused bool
	err := s.pool.QueryRow(ctx,
		`select paused from agent_state where agent_id = $1`, string(agent)).Scan(&paused)
	if err != nil {
		return true, nil //nolint:nilerr // absent means paused, which is the safe reading
	}
	return paused, nil
}

// SetPaused starts or stops an agent.
func (s *State) SetPaused(
	ctx context.Context, agent domain.AgentID, paused bool, by domain.UserID, at time.Time,
) error {
	_, err := s.pool.Exec(ctx, `
		insert into agent_state (agent_id, paused, changed_at, changed_by)
		values ($1, $2, $3, $4)
		on conflict (agent_id) do update set
			paused = excluded.paused,
			changed_at = excluded.changed_at,
			changed_by = excluded.changed_by`,
		string(agent), paused, at.UTC(), string(by))
	if err != nil {
		return fmt.Errorf("spec: set %s paused=%v: %w", agent, paused, err)
	}
	return nil
}

// EnsurePaused records a new agent as paused, without touching one that
// already has a state. Authoring never starts anything, and republishing an
// agent somebody deliberately started must not stop it.
func (s *State) EnsurePaused(ctx context.Context, agent domain.AgentID, by domain.UserID) error {
	_, err := s.pool.Exec(ctx, `
		insert into agent_state (agent_id, paused, changed_by)
		values ($1, true, $2)
		on conflict (agent_id) do nothing`, string(agent), string(by))
	if err != nil {
		return fmt.Errorf("spec: record %s as paused: %w", agent, err)
	}
	return nil
}
