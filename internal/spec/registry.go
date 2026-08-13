package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNotPublished means no such agent version is on record.
var ErrNotPublished = errors.New("spec: no such published version")

// Registry is where published specifications live.
//
// The Store holds what a process loaded; this holds what the installation has
// published. The distinction matters because a run is pinned to a version: the
// file on disk can change, and the version that ran must not.
type Registry struct {
	pool *pgxpool.Pool
}

func NewRegistry(pool *pgxpool.Pool) *Registry {
	return &Registry{pool: pool}
}

// Publish records a version, or does nothing if it is already there.
//
// Insert-or-nothing rather than upsert: the version is the digest of the
// content, so the same version is the same text. A conflict means somebody
// published what was already published, which is not an error and must not
// overwrite the original's authorship or date.
func (r *Registry) Publish(ctx context.Context, s Spec, by domain.UserID, company domain.CompanyID) error {
	budget, err := json.Marshal(s.Budget)
	if err != nil {
		return fmt.Errorf("spec: encode budget: %w", err)
	}
	triggers, err := json.Marshal(triggersOf(s))
	if err != nil {
		return fmt.Errorf("spec: encode triggers: %w", err)
	}

	tools := make([]string, 0, len(s.Tools))
	for _, t := range s.Tools {
		tools = append(tools, string(t))
	}
	// Never nil: a nil slice reaches Postgres as null, and the column says not
	// null because "declares no events" is the empty list rather than unknown.
	emits := s.Emits
	if emits == nil {
		emits = []string{}
	}

	_, err = r.pool.Exec(ctx, `
		insert into agent_specs (
			agent_id, version_id, company_id, area_id, name,
			provider, model, effort, tools, budget, triggers,
			instructions, source, published_by, emits
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		on conflict (agent_id, version_id) do nothing`,
		string(s.ID), string(s.Version), string(company), string(s.Area), s.Name,
		s.Provider, s.Model, s.Effort, tools, budget, triggers,
		s.Instructions, s.Source, string(by), emits)
	if err != nil {
		return fmt.Errorf("spec: publish %s@%s: %w", s.ID, s.Version, err)
	}
	return nil
}

// Get returns one published version, exactly as it was published.
func (r *Registry) Get(ctx context.Context, agent domain.AgentID, version domain.VersionID) (Spec, error) {
	var (
		s        Spec
		tools    []string
		budget   []byte
		triggers []byte
		company  string
	)
	err := r.pool.QueryRow(ctx, `
		select agent_id, version_id, company_id, area_id, name,
		       provider, model, effort, tools, budget, triggers, instructions,
		       source, emits
		from agent_specs where agent_id = $1 and version_id = $2`,
		string(agent), string(version),
	).Scan(&s.ID, &s.Version, &company, &s.Area, &s.Name,
		&s.Provider, &s.Model, &s.Effort, &tools, &budget, &triggers,
		&s.Instructions, &s.Source, &s.Emits)

	if errors.Is(err, pgx.ErrNoRows) {
		return Spec{}, fmt.Errorf("%w: %s@%s", ErrNotPublished, agent, version)
	}
	if err != nil {
		return Spec{}, fmt.Errorf("spec: read %s@%s: %w", agent, version, err)
	}

	for _, t := range tools {
		s.Tools = append(s.Tools, domain.ToolID(t))
	}
	if err := json.Unmarshal(budget, &s.Budget); err != nil {
		return Spec{}, fmt.Errorf("spec: decode budget: %w", err)
	}
	var stored []domain.AgentTrigger
	if err := json.Unmarshal(triggers, &stored); err != nil {
		return Spec{}, fmt.Errorf("spec: decode triggers: %w", err)
	}
	for _, t := range stored {
		s.Triggers = append(s.Triggers, Trigger{Type: t.Type, Schedule: t.Schedule, Path: t.Path, Event: t.Event})
	}
	return s, nil
}
