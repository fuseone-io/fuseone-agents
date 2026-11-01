package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

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
// What a publication says is judged against the configuration as it stands
// under the catalogue's lock — never as the worker read it — so a late
// publication from a worker that missed a change records nothing about the
// server it no longer describes (judge). And every publication, from any
// worker, clears the tools of servers the configuration no longer wants, so
// the catalogue converges at the next publish whatever was lost on the way.
// An unmanaged server has no configuration to say it is gone: the
// publication leases it to the publishing worker, and the same sweep clears
// the unmanaged servers nobody holds a lease on any more (HoldToolServers).
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
	now := c.clock()
	if err := leaseUnmanaged(ctx, tx, pub, now); err != nil {
		return err
	}
	configured, err := configuredServers(ctx, tx)
	if err != nil {
		return err
	}
	if err := sweepCatalogue(ctx, tx, configured, now); err != nil {
		return err
	}
	held := judge(pub, configured)
	if err := dropUnoffered(ctx, tx, held); err != nil {
		return err
	}
	if err := writeTools(ctx, tx, held); err != nil {
		return err
	}
	// Again after the write: it may have rewritten an old worker's rows
	// under a lease, settling their grace.
	if err := forgetSettledGrace(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit the tool catalogue: %w", err)
	}
	return nil
}

// writeTools records every held tool in one statement. A tool ID is unique
// across the catalogue — it is the settings row's name — so the last entry
// for an ID is the one kept, and the insert never meets one row twice.
func writeTools(ctx context.Context, tx pgx.Tx, held judged) error {
	if len(held.entries) == 0 {
		return nil
	}
	values := make(map[string]string, len(held.entries))
	for _, e := range held.entries {
		value, err := encodeTool(e, held.unmanaged[e.Server])
		if err != nil {
			return err
		}
		values[string(e.ID)] = string(value)
	}
	names := slices.Sorted(maps.Keys(values))
	encoded := make([]string, len(names))
	for i, name := range names {
		encoded[i] = values[name]
	}
	if _, err := tx.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_at)
		select 'installation', '', '', $1, t.name, t.value::jsonb, true, now()
		from unnest($2::text[], $3::text[]) as t(name, value)
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			value = excluded.value, updated_at = now()
		where settings.value is distinct from excluded.value`,
		toolKind, names, encoded); err != nil {
		return fmt.Errorf("admin: publish tools: %w", err)
	}
	return nil
}

// toolCatalogueLock serialises every write that judges which tools a server
// offers.
const toolCatalogueLock = "tool-catalogue"

func lockToolCatalogue(ctx context.Context, tx settings.DB) error {
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, toolCatalogueLock); err != nil {
		return fmt.Errorf("admin: lock the tool catalogue: %w", err)
	}
	return nil
}

// dropServerTools removes every catalogue row of one server, under the
// catalogue's lock, and reports how many went. Rulings stay.
func dropServerTools(ctx context.Context, tx settings.DB, server string) (int64, error) {
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

/*
dropUnoffered removes, for every server the publication holds, the tools it
no longer lists — one statement, one pass over the catalogue's rows, however
many servers. A tool ID names one row across the catalogue, so "not kept"
needs no pairing with its server.

An anti-join rather than "<> all(kept)": the array form compares every row
with every kept ID, and at 50 servers of 100 tools it took 245 ms where this
takes 10 ms (and the per-server statements it replaces, 64 ms).
*/
func dropUnoffered(ctx context.Context, tx pgx.Tx, held judged) error {
	if len(held.servers) == 0 {
		return nil
	}
	kept := make([]string, 0, len(held.entries))
	for _, e := range held.entries {
		kept = append(kept, string(e.ID))
	}
	if _, err := tx.Exec(ctx, `
		delete from settings s
		where s.scope_kind = 'installation' and s.company_id = '' and s.area_id = ''
		  and s.kind = $1 and s.value->>'server' = any($2)
		  and not exists (select 1 from unnest($3::text[]) k(name) where k.name = s.name)`,
		toolKind, held.servers, kept); err != nil {
		return fmt.Errorf("admin: drop the tools no longer offered: %w", err)
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
		// workers holding a lease on it.
		Unmanaged bool `json:"unmanaged,omitempty"`
		// Written under a lease. A row marked unmanaged without it comes from
		// a worker from before leases, which gets a grace (LegacyToolGrace).
		Leased bool `json:"leased,omitempty"`
	}{e.Server, e.Description, e.Digest, e.OnSurface, unmanaged, unmanaged})
	if err != nil {
		return nil, fmt.Errorf("admin: encode tool %s: %w", e.ID, err)
	}
	return value, nil
}
