package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
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
// What a publication says is judged against the configuration as it stands
// under the catalogue's lock — never as the worker read it — so a late
// publication from a worker that missed a change records nothing about the
// server it no longer describes (judge). And every publication, from any
// worker, clears the tools of servers the configuration no longer wants, so
// the catalogue converges at the next publish whatever was lost on the way.
// Rulings are not catalogue rows and are never dropped: a tool that comes
// back finds the decision already made.
func (c *Curator) Publish(ctx context.Context, pub domain.ToolPublication) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockToolCatalogue(ctx, tx); err != nil {
		return err
	}
	configured, err := configuredServers(ctx, tx)
	if err != nil {
		return err
	}
	if err := sweepUnconfigured(ctx, tx, configured); err != nil {
		return err
	}
	held := judge(pub, configured)
	if err := dropUnoffered(ctx, tx, held); err != nil {
		return err
	}
	if err := writeTools(ctx, tx, held); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit the tool catalogue: %w", err)
	}
	return nil
}

func writeTools(ctx context.Context, tx pgx.Tx, held judged) error {
	if len(held.entries) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, e := range held.entries {
		value, err := encodeTool(e, held.unmanaged[e.Server])
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

// dropUnoffered removes, for each server the publication holds, the tools it
// no longer lists.
func dropUnoffered(ctx context.Context, tx pgx.Tx, held judged) error {
	kept := make(map[string][]string, len(held.servers))
	for _, e := range held.entries {
		kept[e.Server] = append(kept[e.Server], string(e.ID))
	}
	for _, server := range held.servers {
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

func encodeTool(e domain.ToolEntry, unmanaged bool) ([]byte, error) {
	value, err := json.Marshal(struct {
		Server      string `json:"server"`
		Description string `json:"description,omitempty"`
		Digest      string `json:"digest,omitempty"`
		// Whether this installation brought it in. Published rather than
		// derived, because the console reads this copy and a screen that
		// showed every discovered tool as taken would be offering the choice
		// while hiding the answer.
		OnSurface bool `json:"onSurface"`
		// A server the configuration does not hold: the sweep leaves it to the
		// worker that started it.
		Unmanaged bool `json:"unmanaged,omitempty"`
	}{e.Server, e.Description, e.Digest, e.OnSurface, unmanaged})
	if err != nil {
		return nil, fmt.Errorf("admin: encode tool %s: %w", e.ID, err)
	}
	return value, nil
}
