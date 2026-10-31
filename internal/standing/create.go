package standing

import (
	"context"
	"fmt"
)

/*
Create writes a grant, unless the agent's current version forbids standing
approvals (#192).

The current version is the one somebody chose, and the newest published one
when nobody has — the same reading the registry's listing gives, so the agent
the console shows as forbidding is the agent refused here. An agent nobody has
published is not refused: a grant may name an agent about to be published, and
the claim reads the version the run pinned in any case.

One statement rather than a read and then a write, so a publish landing between
them cannot slip a grant past a version that already forbade it. A publish that
lands after the insert is the claim's to refuse: the engine never asks the store
for a run whose version forbids grants, whenever the grant was made.
*/
func (s *Store) Create(ctx context.Context, g Grant) error {
	if err := Validate(g, g.CreatedAt); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		insert into standing_approvals
			(grant_id, tool, agent_id, company_id, area_id, daily_cap,
			 reason, status, created_by, created_at, expires_at)
		select $1, $2, $3, $4, $5, $6::int, $7, 'active', $8,
		       $9::timestamptz, $10::timestamptz
		where not exists (
			select 1 from (
				select s.standing_approvals
				from agent_specs s
				left join agent_state st on st.agent_id = s.agent_id
				where s.agent_id = $3
				order by (st.current_version = s.version_id) desc nulls last,
				         s.published_at desc
				limit 1
			) chosen where chosen.standing_approvals = 'forbidden'
		)`,
		g.ID, string(g.Tool), string(g.Agent),
		string(g.Scope.Company), string(g.Scope.Area),
		g.DailyCap, g.Reason, string(g.CreatedBy), g.CreatedAt, g.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("standing: create: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrStandingForbidden, g.Agent)
	}
	return nil
}
