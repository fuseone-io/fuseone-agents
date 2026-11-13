package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

var (
	// ErrNoOwnerScope is a tool whose server no stored row holds: one nobody
	// published, or one a --mcp flag connected. There is no owner's scope to
	// store a ruling at; the platform's level still takes one.
	ErrNoOwnerScope = errors.New("admin: the tool's server is not stored at any scope")
	// ErrRulingUnknown is a withdrawal of a ruling nobody recorded.
	ErrRulingUnknown = errors.New("admin: no such ruling")
)

// ToolOwner is the server a published tool belongs to, and the exact row that
// stores it: where the owner's ruling on the tool is stored and recorded, and
// the row the door authorized (NT-019).
type ToolOwner struct {
	Server string
	At     MCPServerAt
}

// OwnerOf reads the tool's server from the published catalogue, then where
// that server's row is stored. One read, so a door authorizes the owner it
// will then write for.
func (c *Curator) OwnerOf(ctx context.Context, tool domain.ToolID) (ToolOwner, error) {
	var server, kind, company, area string
	err := c.pool.QueryRow(ctx, `
		select s.name, s.scope_kind, s.company_id, s.area_id
		from settings t
		join settings s on s.kind = $2 and s.name = t.value->>'server'
		where t.kind = $1 and t.name = $3`,
		toolKind, string(settings.KindMCPServer), string(tool),
	).Scan(&server, &kind, &company, &area)
	if errors.Is(err, pgx.ErrNoRows) {
		return ToolOwner{}, fmt.Errorf("%w: %s", ErrNoOwnerScope, tool)
	}
	if err != nil {
		return ToolOwner{}, fmt.Errorf("admin: find the owner of %s: %w", tool, err)
	}
	stored := domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	return ToolOwner{Server: server, At: MCPServerAt{
		Kind: settings.ScopeKind(kind), Stored: stored,
		Audit: mcpServerRuntimeScope(settings.ScopeKind(kind), stored),
	}}, nil
}

/*
Withdraw removes the platform's ruling on a tool, the ceiling (NT-019).

Only the platform's row: the owner's, when there is one, is then the ruling in
force, and with none the tool never executes. Recorded in the same
transaction, with what was withdrawn.
*/
func (c *Curator) Withdraw(ctx context.Context, by domain.UserID, scope domain.Scope, tool domain.ToolID) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var withdrawn []byte
	err = tx.QueryRow(ctx, `
		delete from settings
		where scope_kind = 'installation' and company_id = '' and area_id = ''
		  and kind = $1 and name = $2
		returning value`, classificationKind, string(tool)).Scan(&withdrawn)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrRulingUnknown, tool)
	}
	if err != nil {
		return fmt.Errorf("admin: withdraw the ruling on %s: %w", tool, err)
	}
	if err := Record(ctx, tx, Event{
		Principal: by, Scope: scope, Action: "tool.classification_withdrawn",
		Target: string(tool), Detail: json.RawMessage(withdrawn),
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit the withdrawal: %w", err)
	}
	return nil
}
