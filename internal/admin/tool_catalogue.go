package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

// toolKind is the settings row a discovered tool is published as.
const toolKind = "tool"

// Publish records what a server offers, so the catalogue is installation state
// rather than something each process rediscovers.
//
// Discovery and ruling are kept apart on purpose: publishing never changes an
// effect. A server that calls itself a write tool tomorrow still arrives
// unclassified, and the ruling that promotes it stays the separate, attributed
// act it has to be.
//
// What publishing does carry is the digest of the definition it found, because
// that is what a recorded ruling is matched against below. Without it this
// copy of the catalogue would apply rulings by name while the in-process one
// applied them by definition, and the two would disagree about the row a
// Curator is looking at.
//
// A server the publication names is judged whole: what it no longer offers is
// dropped, and only those catalogue rows — a ruling is kept by tool name, so a
// tool that comes back finds the decision already made. A server it does not
// name is left alone; see domain.ToolPublication.
func (c *Curator) Publish(ctx context.Context, pub domain.ToolPublication) error {
	if len(pub.Entries) == 0 && len(pub.Servers) == 0 && len(pub.Withdrawn) == 0 {
		return nil
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Two workers publishing the same server would each keep its own list and
	// drop the other's: serialised, the later discovery is the one that holds.
	if err := lockToolCatalogue(ctx, tx); err != nil {
		return err
	}
	if err := dropUnoffered(ctx, tx, pub); err != nil {
		return err
	}
	if err := dropWithdrawn(ctx, tx, pub.Withdrawn); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, e := range pub.Entries {
		value, err := encodeTool(e)
		if err != nil {
			return err
		}
		batch.Queue(`
			insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_at)
			values ('installation', '', '', $1, $2, $3, true, now())
			on conflict (scope_kind, company_id, area_id, kind, name) do update set
				value = excluded.value, updated_at = now()`,
			toolKind, string(e.ID), value)
	}

	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("admin: publish tools: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit the tool catalogue: %w", err)
	}
	return nil
}

// toolCatalogueLock serialises every write that judges which tools a server
// offers.
const toolCatalogueLock = "tool-catalogue"

func lockToolCatalogue(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, toolCatalogueLock); err != nil {
		return fmt.Errorf("admin: lock the tool catalogue: %w", err)
	}
	return nil
}

// dropServerTools removes every catalogue row of one server, under the
// catalogue's lock, and reports how many went. Rulings stay.
func dropServerTools(ctx context.Context, tx pgx.Tx, server string) (int64, error) {
	if err := lockToolCatalogue(ctx, tx); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		delete from settings
		where scope_kind = 'installation' and company_id = '' and area_id = ''
		  and kind = $1 and value->>'server' = $2`, toolKind, server)
	if err != nil {
		return 0, fmt.Errorf("admin: drop the tools of %s: %w", server, err)
	}
	return tag.RowsAffected(), nil
}

// dropWithdrawn removes the tools of servers a worker found the configuration
// no longer wants — judged now, under the lock, not when the worker read the
// configuration: a server registered or switched on again in between is
// wanted, and keeps its tools.
func dropWithdrawn(ctx context.Context, tx pgx.Tx, servers []string) error {
	for _, server := range servers {
		if _, err := tx.Exec(ctx, `
			delete from settings
			where scope_kind = 'installation' and company_id = '' and area_id = ''
			  and kind = $1 and value->>'server' = $2
			  and not exists (
			      select 1 from settings
			      where scope_kind = 'installation' and company_id = '' and area_id = ''
			        and kind = $3 and name = $2 and enabled)`,
			toolKind, server, string(settings.KindMCPServer)); err != nil {
			return fmt.Errorf("admin: drop the tools of withdrawn %s: %w", server, err)
		}
	}
	return nil
}

// dropUnoffered removes, for each server the publication names, the tools it
// no longer lists.
func dropUnoffered(ctx context.Context, tx pgx.Tx, pub domain.ToolPublication) error {
	kept := make(map[string][]string, len(pub.Servers))
	for _, e := range pub.Entries {
		kept[e.Server] = append(kept[e.Server], string(e.ID))
	}
	for _, server := range pub.Servers {
		if _, err := tx.Exec(ctx, `
			delete from settings
			where scope_kind = 'installation' and company_id = '' and area_id = ''
			  and kind = $1 and value->>'server' = $2 and not (name = any($3))`,
			toolKind, server, append([]string{}, kept[server]...)); err != nil {
			return fmt.Errorf("admin: drop the tools %s no longer offers: %w", server, err)
		}
	}
	return nil
}

func encodeTool(e domain.ToolEntry) ([]byte, error) {
	value, err := json.Marshal(struct {
		Server      string `json:"server"`
		Description string `json:"description,omitempty"`
		Digest      string `json:"digest,omitempty"`
		// Whether this installation brought it in. Published rather than
		// derived, because the console reads this copy and a screen that
		// showed every discovered tool as taken would be offering the choice
		// while hiding the answer.
		OnSurface bool `json:"onSurface"`
	}{e.Server, e.Description, e.Digest, e.OnSurface})
	if err != nil {
		return nil, fmt.Errorf("admin: encode tool %s: %w", e.ID, err)
	}
	return value, nil
}
