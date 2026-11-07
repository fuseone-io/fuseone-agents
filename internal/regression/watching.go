package regression

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// ErasedCases lists every correction whose occurrence is gone, for the notice
// that tells an author a case has stopped being checked.
func (s *Store) ErasedCases(ctx context.Context) ([]domain.ErasedCase, error) {
	rows, err := s.pool.Query(ctx, `
		select c.agent_id, c.case_id, c.company_id, c.area_id
		from regression_cases c
		left join run_content rc on rc.ref = c.input_ref
		left join run_content ex on ex.ref = c.expectations_ref
		where rc.erased_at is not null or ex.erased_at is not null
		order by c.agent_id, c.case_id`)
	if err != nil {
		return nil, fmt.Errorf("regression: list erased cases: %w", err)
	}
	defer rows.Close()

	var out []domain.ErasedCase
	for rows.Next() {
		var agent, kase, company, area string
		if err := rows.Scan(&agent, &kase, &company, &area); err != nil {
			return nil, fmt.Errorf("regression: scan erased case: %w", err)
		}
		out = append(out, domain.ErasedCase{
			Agent: domain.AgentID(agent), Case: kase,
			Scope: domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
		})
	}
	return out, rows.Err()
}

/*
Watching lists the agents that have a corpus at all.

The clock only spends money where there is something to check. An agent with
no corrections would produce a battery whose report says nothing, at the price
of a real set of model calls per pass.

The scope comes from the cases, which is where it already is: a correction is
made in an area and the notice about it has to reach that area's conversation,
not the installation's.
*/
func (s *Store) Watching(ctx context.Context) ([]domain.WatchedCorpus, error) {
	rows, err := s.pool.Query(ctx, `
		select c.agent_id, coalesce(st.current_version, ''),
		       min(c.company_id), min(c.area_id)
		from regression_cases c
		left join agent_state st on st.agent_id = c.agent_id
		group by c.agent_id, st.current_version
		order by c.agent_id`)
	if err != nil {
		return nil, fmt.Errorf("regression: list watched corpora: %w", err)
	}
	defer rows.Close()

	var out []domain.WatchedCorpus
	for rows.Next() {
		var agent, version, company, area string
		if err := rows.Scan(&agent, &version, &company, &area); err != nil {
			return nil, fmt.Errorf("regression: scan watched corpus: %w", err)
		}
		// An agent with no current version has never been published, so
		// there is nothing to have drifted.
		if version == "" {
			continue
		}
		out = append(out, domain.WatchedCorpus{
			Agent:   domain.AgentID(agent),
			Version: domain.VersionID(version),
			Scope: domain.Scope{
				Company: domain.CompanyID(company), Area: domain.AreaID(area),
			},
		})
	}
	return out, rows.Err()
}
