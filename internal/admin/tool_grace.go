package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/settings"
)

/*
LegacyToolGrace is how long an unmanaged server published by a worker from
before leases is kept with nobody renewing it, counted from the first sweep
that saw it.

Such a worker marks its --mcp servers unmanaged and nothing else, and never
renews a lease it does not know exists. During a rolling upgrade it may still
be serving them while a new worker — without the flag, or whose connection to
the server failed — sweeps. Taking its tools at once would break what it
serves for the rest of the rollout.

Per server from first sight rather than from the migration: an old worker can
publish a server for the first time an hour into the rollout, and its grace
starts then. A day outlasts any rolling upgrade by a wide margin; the price is
that rows already stuck before the upgrade — the ones #223 is about — leave a
day after it rather than at once. A new worker running the server writes the
lease marker over the old rows and holds a lease, and the grace stops
mattering.
*/
const LegacyToolGrace = 24 * time.Hour

// graceBegunBy is the moment an old worker's grace must have begun by to be
// over at now.
func graceBegunBy(now time.Time) time.Time { return now.Add(-LegacyToolGrace) }

// legacyRow is the catalogue row of an unmanaged server written without the
// lease marker: by a worker from before leases.
const legacyRow = `coalesce((s.value->>'unmanaged')::boolean, false)
	and not coalesce((s.value->>'leased')::boolean, false)`

// ungracedLegacy is an old worker's row whose server no sweep has seen yet:
// worth taking the lock for, to start its grace.
const ungracedLegacy = legacyRow + `
	and not exists (select 1 from tool_server_grace g where g.server = s.value->>'server')`

// beginLegacyGrace starts the grace of every old worker's server no sweep
// has seen before. Under the catalogue's lock. A grace already begun is kept:
// seeing the server again does not start it over.
func beginLegacyGrace(ctx context.Context, db settings.DB, now time.Time) error {
	if _, err := db.Exec(ctx, `
		insert into tool_server_grace (server, first_seen)
		select distinct s.value->>'server', $2::timestamptz from settings s
		where s.scope_kind = 'installation' and s.company_id = '' and s.area_id = ''
		  and s.kind = $1 and `+legacyRow+`
		on conflict (server) do nothing`, toolKind, now); err != nil {
		return fmt.Errorf("admin: begin the grace of old workers' servers: %w", err)
	}
	return nil
}

// forgetSettledGrace drops the grace of every server with no old worker's
// row left — swept, or rewritten by a lease-era worker — so an old worker
// publishing it again later gets a grace of its own.
func forgetSettledGrace(ctx context.Context, db settings.DB) error {
	if _, err := db.Exec(ctx, `
		delete from tool_server_grace g
		where not exists (
			select 1 from settings s
			where s.scope_kind = 'installation' and s.company_id = '' and s.area_id = ''
			  and s.kind = $1 and `+legacyRow+` and s.value->>'server' = g.server)`,
		toolKind); err != nil {
		return fmt.Errorf("admin: forget settled graces: %w", err)
	}
	return nil
}
