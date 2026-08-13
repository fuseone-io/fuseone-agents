package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
Reading the catalogue: which agents exist, at which version, for whom.

“Current” is a stored choice and not the newest row, because an author who
publishes an edit and then withdraws it must leave the installation running
what it ran before, not the withdrawn version forever.
*/
// List returns published agents, newest version of each unless every version
// is asked for.
func (r *Registry) List(ctx context.Context, scope domain.Scope, allVersions bool) ([]domain.AgentSummary, error) {
	// The current version is the one somebody chose, and only the newest by
	// publication when nobody has. Ordering by the choice first is what makes
	// a rollback take effect and what stops a withdrawn version staying
	// current for ever (PRD DE-08).
	//
	// DISTINCT ON is why the default is one row per agent: picking in SQL
	// beats reading every version and discarding most of them.
	selection := `
		select distinct on (s.agent_id)
		       s.agent_id, s.version_id, s.company_id, s.area_id, s.name,
		       s.provider, s.model, s.effort, s.tools, s.budget, s.triggers,
		       s.published_by, s.published_at, true
		from agent_specs s
		left join agent_state st on st.agent_id = s.agent_id %s
		order by s.agent_id, (st.current_version = s.version_id) desc nulls last,
		         s.published_at desc`
	if allVersions {
		selection = `
			select s.agent_id, s.version_id, s.company_id, s.area_id, s.name,
			       s.provider, s.model, s.effort, s.tools, s.budget, s.triggers,
			       s.published_by, s.published_at,
			       coalesce(st.current_version = s.version_id,
			                s.published_at = max(s.published_at) over (partition by s.agent_id))
			from agent_specs s
			left join agent_state st on st.agent_id = s.agent_id %s
			order by s.agent_id, s.published_at desc`
	}

	where, args := scopeSQL(scope)
	rows, err := r.pool.Query(ctx, fmt.Sprintf(selection, where), args...)
	if err != nil {
		return nil, fmt.Errorf("spec: list agents: %w", err)
	}
	defer rows.Close()

	var out []domain.AgentSummary
	for rows.Next() {
		summary, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, rows.Err()
}

// Versions returns every published version of one agent, newest first.
//
// The whole history rather than the latest: publishing a new version never
// touches a run already in flight, so the older ones stay the only correct
// explanation of the runs pinned to them.
func (r *Registry) Versions(ctx context.Context, agent domain.AgentID) ([]domain.AgentSummary, error) {
	// The current one first, then the rest newest-first. Every caller reads
	// the first row as the answer — the opener pins a run to it — so ordering
	// by publication alone made a withdrawn version the one every new run got
	// (PRD DE-08).
	rows, err := r.pool.Query(ctx, `
		select s.agent_id, s.version_id, s.company_id, s.area_id, s.name,
		       s.provider, s.model, s.effort, s.tools, s.budget, s.triggers,
		       s.published_by, s.published_at, false
		from agent_specs s
		left join agent_state st on st.agent_id = s.agent_id
		where s.agent_id = $1
		order by (st.current_version = s.version_id) desc nulls last,
		         s.published_at desc`, string(agent))
	if err != nil {
		return nil, fmt.Errorf("spec: versions of %s: %w", agent, err)
	}
	defer rows.Close()

	var out []domain.AgentSummary
	for rows.Next() {
		summary, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The current one is first, so the first row is what a reader gets by
	// default and what a run pins to.
	for i := range out {
		out[i].Latest = i == 0
	}
	return out, nil
}

// Instructions returns one version's body and where it came from.
//
// Separate from the listing because the text is only ever wanted one version
// at a time, and a page of twenty agents carrying twenty bodies of prose is a
// page nobody can load twice.
func (r *Registry) Instructions(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (text, source string, err error) {
	err = r.pool.QueryRow(ctx, `
		select instructions, source from agent_specs
		where agent_id = $1 and version_id = $2`,
		string(agent), string(version)).Scan(&text, &source)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("%w: %s@%s", ErrNotPublished, agent, version)
	}
	if err != nil {
		return "", "", fmt.Errorf("spec: read instructions %s@%s: %w", agent, version, err)
	}
	return text, source, nil
}

/*
MakeCurrent names the version an agent runs.

Which version is current is state beside the specification rather than in it,
like whether the agent is paused: a published version is what somebody wrote
and never changes, while the choice of which one runs changes on an afternoon.
Deriving it from the publication timestamp made a rollback impossible to
express and left a withdrawn version current for ever.

The row is created if the agent has none, paused, because an agent nobody has
decided about does not start (PRD DE-07).
*/
func (r *Registry) MakeCurrent(ctx context.Context, agent domain.AgentID, version domain.VersionID) error {
	_, err := r.pool.Exec(ctx, `
		insert into agent_state (agent_id, paused, current_version, changed_at)
		values ($1, true, $2, now())
		on conflict (agent_id) do update set
			current_version = excluded.current_version, changed_at = now()`,
		string(agent), string(version))
	if err != nil {
		return fmt.Errorf("spec: make %s@%s current: %w", agent, version, err)
	}
	return nil
}

func scanAgent(row interface{ Scan(...any) error }) (domain.AgentSummary, error) {
	var (
		s                domain.AgentSummary
		agent, version   string
		company, area    string
		by               string
		tools            []string
		budget, triggers []byte
	)
	if err := row.Scan(&agent, &version, &company, &area, &s.Name,
		&s.Provider, &s.Model, &s.Effort, &tools, &budget, &triggers,
		&by, &s.PublishedAt, &s.Latest); err != nil {
		return domain.AgentSummary{}, err
	}

	s.ID = domain.AgentID(agent)
	s.VersionID = domain.VersionID(version)
	s.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	s.PublishedBy = domain.UserID(by)
	for _, t := range tools {
		s.Tools = append(s.Tools, domain.ToolID(t))
	}
	if err := json.Unmarshal(budget, &s.Budget); err != nil {
		return domain.AgentSummary{}, fmt.Errorf("spec: decode budget for %s: %w", agent, err)
	}
	if err := json.Unmarshal(triggers, &s.Triggers); err != nil {
		return domain.AgentSummary{}, fmt.Errorf("spec: decode triggers for %s: %w", agent, err)
	}
	return s, nil
}

func triggersOf(s Spec) []domain.AgentTrigger {
	out := make([]domain.AgentTrigger, 0, len(s.Triggers))
	for _, t := range s.Triggers {
		out = append(out, domain.AgentTrigger{Type: t.Type, Schedule: t.Schedule, Path: t.Path, Event: t.Event})
	}
	return out
}

func scopeSQL(scope domain.Scope) (string, []any) {
	var (
		clauses []string
		args    []any
	)
	if scope.Company != "" {
		args = append(args, string(scope.Company))
		clauses = append(clauses, fmt.Sprintf("s.company_id = $%d", len(args)))
	}
	if scope.Area != "" {
		args = append(args, string(scope.Area))
		clauses = append(clauses, fmt.Sprintf("s.area_id = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "where " + strings.Join(clauses, " and "), args
}
