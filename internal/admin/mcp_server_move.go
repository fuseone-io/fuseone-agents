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
	if err := i.moveLoginSecret(ctx, tx, m.Name, to, m.To, m.By); err != nil {
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
		// Every credential of the server is rewritten at one time: a row
		// stored before the instant was recorded keeps its own (delivery 3).
		stored.ConfiguredAt = stored.configuredOr(row.UpdatedAt)
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
