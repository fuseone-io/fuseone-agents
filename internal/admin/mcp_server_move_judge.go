package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// The refusals of a move that depend on what is stored and where (NT-019):
// a local server, a target nobody registered, an origin out of the target's
// egress policy, and the published agents it would strand.

// judgeMove answers every refusal that depends on what is stored and where.
func (i *Integrations) judgeMove(ctx context.Context, tx settings.DB, m MCPServerMove, stored settings.Setting) error {
	var server storedServer
	if err := json.Unmarshal(stored.Value, &server); err != nil {
		return fmt.Errorf("admin: decode MCP server %s: %w", m.Name, err)
	}
	if server.Transport != domain.TransportHTTP {
		return ErrMoveLocalServer
	}
	if err := registeredTarget(ctx, tx, m.To); err != nil {
		return err
	}
	kind := settings.ScopeCompany
	if m.To.Area != "" {
		kind = settings.ScopeArea
	}
	egress, err := i.egressAtTx(ctx, tx, kind, m.To)
	if err != nil {
		return err
	}
	if err := withinEgress(egressOrNone(egress), server.URL, domain.ReadMCPCredentials(stored.Secret)); err != nil {
		return err
	}
	if login := server.OAuthLogin; login != nil {
		// Its sign-in endpoints are requests of the server's too.
		if err := originsWithin(egressOrNone(egress), nonEmpty(login.AuthorizeURL, login.TokenURL)); err != nil {
			return err
		}
	}
	if m.Strand {
		return nil
	}
	stranded, err := strandedAgents(ctx, tx, m.Name, m.To)
	if err != nil {
		return err
	}
	if len(stranded) > 0 {
		return &MoveStrandsAgents{Agents: stranded}
	}
	return nil
}

// egressOrNone holds a server outside the installation to its policy, or to
// none when no policy applies: it reaches nothing (NT-019).
func egressOrNone(egress *domain.MCPEgress) *domain.MCPEgress {
	if egress == nil {
		return &domain.MCPEgress{}
	}
	return egress
}

func registeredTarget(ctx context.Context, tx settings.DB, to domain.Scope) error {
	var registered bool
	err := tx.QueryRow(ctx, `
		select case when $2 = ''
			then exists (select 1 from companies where company_id = $1 and archived_at is null)
			else exists (select 1 from scopes a join companies c using (company_id)
			             where a.company_id = $1 and a.area_id = $2 and c.archived_at is null)
		end`, string(to.Company), string(to.Area)).Scan(&registered)
	if err != nil {
		return fmt.Errorf("admin: read the target's registration: %w", err)
	}
	if !registered {
		return ErrMoveTargetUnregistered
	}
	return nil
}

// strandedAgents are the agents outside to whose current version names one
// of the server's tools, retired ones aside: a snapshot, read in the move's
// transaction.
func strandedAgents(ctx context.Context, tx settings.DB, server string, to domain.Scope) ([]StrandedAgent, error) {
	rows, err := tx.Query(ctx, `
		with current as (
			select distinct on (s.agent_id) s.agent_id, s.company_id, s.area_id, s.tools
			from agent_specs s
			left join agent_state st on st.agent_id = s.agent_id
			order by s.agent_id, (st.current_version = s.version_id) desc nulls last, s.published_at desc
		)
		select c.agent_id, c.company_id, c.area_id
		from current c left join agent_state st on st.agent_id = c.agent_id
		where st.retired_at is null
		  and exists (select 1 from unnest(c.tools) t where starts_with(t, $1))
		order by c.agent_id`, server+".")
	if err != nil {
		return nil, fmt.Errorf("admin: read the agents naming %s's tools: %w", server, err)
	}
	defer rows.Close()
	var out []StrandedAgent
	for rows.Next() {
		var agent, company, area string
		if err := rows.Scan(&agent, &company, &area); err != nil {
			return nil, err
		}
		at := domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		if !to.Contains(at) {
			out = append(out, StrandedAgent{Agent: domain.AgentID(agent), Scope: at})
		}
	}
	return out, rows.Err()
}
