package spec

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
ShareEventGraph takes the event graph's lock shared, in tx (NT-018 1b).

For a write that reads which version is current and must not be judged
against one a publication is changing: a draft judging the agent's current
scope, or refusing an identifier that already has a published agent. It
waits for a publication in flight and holds new ones off until tx ends,
while such writes do not wait for each other. Taken before any row lock, as
the publication takes the exclusive one, so every writer keeps one order.
*/
func ShareEventGraph(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock_shared($1)`, eventGraphLock); err != nil {
		return fmt.Errorf("spec: share the event graph: %w", err)
	}
	return nil
}

// CurrentIn is the agent's current version, whole, read inside tx: the one
// somebody made current, else the newest published. False when the agent has
// no version.
func CurrentIn(ctx context.Context, tx pgx.Tx, agent domain.AgentID) (Spec, bool, error) {
	version, _, found, err := currentVersionIn(ctx, tx, agent)
	if err != nil || !found {
		return Spec{}, false, err
	}
	s, err := (&Registry{q: tx}).Get(ctx, agent, version)
	if err != nil {
		return Spec{}, false, err
	}
	return s, true, nil
}

// currentVersionIn chooses the current version as every reader does
// (Registry.Versions), and the scope it was published in.
func currentVersionIn(
	ctx context.Context, q pgx.Tx, agent domain.AgentID,
) (domain.VersionID, domain.Scope, bool, error) {
	var version, company, area string
	err := q.QueryRow(ctx, `
		select s.version_id, s.company_id, s.area_id
		from agent_specs s
		left join agent_state st on st.agent_id = s.agent_id
		where s.agent_id = $1
		order by (st.current_version = s.version_id) desc nulls last, s.published_at desc
		limit 1`, string(agent)).Scan(&version, &company, &area)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.Scope{}, false, nil
	}
	if err != nil {
		return "", domain.Scope{}, false, fmt.Errorf("spec: %s: read the current version: %w", agent, err)
	}
	scope := domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	return domain.VersionID(version), scope, true, nil
}
