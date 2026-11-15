package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

var (
	// ErrMoveNeedsIfMatch is a move with no precondition: it takes a server
	// away from every other scope, so it is made against the revision read.
	ErrMoveNeedsIfMatch = errors.New("admin: moving an MCP server needs If-Match")
	// ErrMoveTargetUnregistered is a target no registered company or area is.
	ErrMoveTargetUnregistered = errors.New("admin: the target is not a registered company or area")
	// ErrMoveNotAtInstallation is a server already stored at a company or
	// area: its owner's rulings sit at its scope, and nothing would carry them.
	ErrMoveNotAtInstallation = errors.New("admin: only a server stored at the installation is moved")
	// ErrMoveLocalServer is a local server, which stays the installation's
	// (migration 0110).
	ErrMoveLocalServer = errors.New("admin: a local MCP server stays the installation's")
)

// StrandedAgent is a published agent outside a move's target that names one
// of the server's tools.
type StrandedAgent struct {
	Agent domain.AgentID
	Scope domain.Scope
}

// MoveStrandsAgents refuses a move that would strand published agents,
// naming them, unless the move says strand (NT-019).
type MoveStrandsAgents struct{ Agents []StrandedAgent }

func (e *MoveStrandsAgents) Error() string {
	return fmt.Sprintf("admin: moving the server strands %d published agents", len(e.Agents))
}

// MCPServerMove moves a server out of the installation (NT-019).
type MCPServerMove struct {
	By domain.UserID
	// Audit is the platform's scope, where the departure is recorded.
	Audit  domain.Scope
	Name   string
	To     domain.Scope
	Expect domain.IfMatch
	// Strand moves the server even when published agents outside To name
	// its tools; their runs are then blocked by the Gate.
	Strand bool
}

/*
MoveMCPServer stores a server of the installation at a company or area, in one
transaction and in the order the PUT takes its locks: the name, the row at
each key, then the egress policies shared. Every refusal comes before the
first write. The secret opens only at the key it was sealed under, so it is
revealed there and sealed again at the target; the person's credentials,
stored at the installation, are pointed at the new row.
*/
func (i *Integrations) MoveMCPServer(ctx context.Context, m MCPServerMove) (domain.Revision, error) {
	if !m.Expect.Present() {
		return "", ErrMoveNeedsIfMatch
	}
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("admin: begin the move: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stored, to, err := i.lockForMove(ctx, tx, m)
	if err != nil {
		return "", err
	}
	if err := i.judgeMove(ctx, tx, m, stored); err != nil {
		return "", err
	}
	moved := stored
	moved.ScopeKind, moved.Scope, moved.UpdatedBy = to, m.To, string(m.By)
	if err := i.settings.DeleteTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, m.Name); err != nil {
		return "", err
	}
	if err := i.settings.PutTx(ctx, tx, moved); err != nil {
		return "", err
	}
	if err := i.repointPersonalCredentials(ctx, tx, m.Name, to, m.To); err != nil {
		return "", err
	}
	if err := recordMove(ctx, tx, m); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("admin: commit the move: %w", err)
	}
	return i.mcpRevision(moved), nil
}

// lockForMove takes the move's locks and reveals the server at the
// installation, answering the precondition against what it reads.
func (i *Integrations) lockForMove(
	ctx context.Context, tx settings.DB, m MCPServerMove,
) (settings.Setting, settings.ScopeKind, error) {
	to := settings.ScopeCompany
	if m.To.Area != "" {
		to = settings.ScopeArea
	}
	if err := LockMCPServerName(ctx, tx, m.Name); err != nil {
		return settings.Setting{}, "", err
	}
	for _, key := range []settings.Setting{
		{ScopeKind: settings.ScopeInstallation, Kind: settings.KindMCPServer, Name: m.Name},
		{ScopeKind: to, Scope: m.To, Kind: settings.KindMCPServer, Name: m.Name},
	} {
		if err := settings.GuardTx(ctx, tx, key); err != nil {
			return settings.Setting{}, "", err
		}
	}
	stored, err := i.settings.RevealTx(ctx, tx, settings.ScopeInstallation, domain.Scope{}, settings.KindMCPServer, m.Name)
	if errors.Is(err, settings.ErrNotFound) {
		return settings.Setting{}, "", i.missingAtInstallation(ctx, tx, m.Name)
	}
	if err != nil {
		return settings.Setting{}, "", err
	}
	if current := i.mcpRevision(stored); !m.Expect.Allows(true, current) {
		return settings.Setting{}, "", &domain.StaleError{Current: current}
	}
	return stored, to, nil
}

func (i *Integrations) missingAtInstallation(ctx context.Context, tx settings.DB, name string) error {
	var elsewhere bool
	if err := tx.QueryRow(ctx,
		`select exists (select 1 from settings where kind = $1 and name = $2)`,
		string(settings.KindMCPServer), name).Scan(&elsewhere); err != nil {
		return fmt.Errorf("admin: find MCP server %s: %w", name, err)
	}
	if elsewhere {
		return ErrMoveNotAtInstallation
	}
	return ErrMCPServerUnknown
}

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

// repointPersonalCredentials names the server's new row in each person's
// credential. They are stored at the installation under their own key, so
// their secrets stay sealed as they are.
func (i *Integrations) repointPersonalCredentials(
	ctx context.Context, tx settings.DB, server string, kind settings.ScopeKind, to domain.Scope,
) error {
	rows, err := i.settings.ListUnderPrefixTx(ctx, tx, settings.KindMCPUserCredential, encodeSettingPart(server)+".")
	if err != nil {
		return err
	}
	for _, row := range rows {
		var stored storedMCPUserCredential
		if err := json.Unmarshal(row.Value, &stored); err != nil {
			return fmt.Errorf("admin: decode personal MCP credential %s: %w", row.Name, err)
		}
		if rowServer(stored, row.Name) != server {
			continue
		}
		stored.ServerAt = serverRow(kind, to)
		value, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		row.Value = value
		if err := i.settings.PutTx(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

func recordMove(ctx context.Context, tx settings.DB, m MCPServerMove) error {
	detail := map[string]string{"to": scopePath(m.To), "from": "installation"}
	for _, at := range []domain.Scope{m.Audit, m.To} {
		if err := Record(ctx, tx, Event{
			Principal: m.By, Scope: at, Action: "mcp_server.moved", Target: m.Name, Detail: detail,
		}); err != nil {
			return err
		}
	}
	return nil
}

func scopePath(s domain.Scope) string {
	return strings.TrimSuffix(string(s.Company)+"/"+string(s.Area), "/")
}
