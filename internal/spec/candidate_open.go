package spec

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

var _ trigger.Candidates = (*CandidateReader)(nil)

/*
Candidate resolves a seal of agent for the run an experiment opens on it (NT-018 phase 2):
the version it holds, the scope it was sealed in, and the pause the agent has. Decision 3:
only a recorded pause refuses a candidate, so an agent with no state, one never published,
is not paused here. A seal of another agent, one nobody wrote, and an id that is not a seal
id are not found, which the opener answers as a version the agent never published.
*/
func (r *CandidateReader) Candidate(
	ctx context.Context, agent domain.AgentID, seal string,
) (trigger.Candidate, bool, error) {
	if !sealIDShape.MatchString(seal) {
		return trigger.Candidate{}, false, nil
	}
	out := trigger.Candidate{Agent: agent}
	var version, company, area string
	err := r.pool.QueryRow(ctx, `
		select s.version_id, s.company_id, s.area_id, coalesce(st.paused, false)
		from agent_seals s
		left join agent_state st on st.agent_id = s.agent_id
		where s.seal_id = $1::uuid and s.agent_id = $2`, seal, string(agent)).
		Scan(&version, &company, &area, &out.Paused)
	if errors.Is(err, pgx.ErrNoRows) {
		return trigger.Candidate{}, false, nil
	}
	if err != nil {
		return trigger.Candidate{}, false, fmt.Errorf("spec: read seal %s: %w", seal, err)
	}
	out.Version = domain.VersionID(version)
	out.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	return out, true, nil
}
