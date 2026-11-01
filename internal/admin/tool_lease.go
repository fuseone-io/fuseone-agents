package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
ToolServerLease is how long a worker's hold on an unmanaged server lasts
without being renewed (#223).

A server started with --mcp has no configuration row, so nothing in the
configuration can say it is gone. What says it is still served is a lease:
every worker running it renews one on each reconcile pass, and when the last
lease on a server runs out, the next pass of any worker takes its tools away.

Workers renew every 30 seconds, so a worker can miss three passes — a slow
pass waiting on a server that times out, a database blip — before its tools
leave. It is also how long a server removed from every command line goes on
being offered.

Leases are written and judged by different processes' clocks. The margin is
minutes; clock skew between replicas of one installation is far below it.
*/
const ToolServerLease = 2 * time.Minute

// ErrNoHolder refuses an unmanaged server published without a holder: its
// tools would be rows nobody is accountable for renewing.
var ErrNoHolder = errors.New("admin: an unmanaged server is published without a holder to lease it")

// WithClock sets the clock leases are written and judged by.
func (c *Curator) WithClock(now func() time.Time) *Curator {
	c.clock = now
	return c
}

/*
HoldToolServers renews the holder's lease on each unmanaged server it runs,
and drops the tools of unmanaged servers nobody holds any more.

Every worker calls it on every pass, holding servers or not: when the last
worker drops a flag, it is a worker holding nothing whose pass notices.

It reports whether any of the holder's leases had run out or did not exist.
The server's tools may then have been swept, and a worker with nothing new
publishes nothing — so it must publish to put them back. A renewal racing a
sweep reports this too: the sweep only drops a server it found with no live
lease, so the lease being renewed was not live.
*/
func (c *Curator) HoldToolServers(ctx context.Context, holder string, servers []string) (bool, error) {
	now := c.clock()
	lapsed, err := renewLeases(ctx, c.pool, toolLease{holder: holder, servers: servers, at: now})
	if err != nil {
		return false, err
	}
	if err := c.sweepUnheld(ctx, now); err != nil {
		return false, err
	}
	return lapsed, nil
}

// toolLease is one holder's claim, at one moment, on the servers it runs.
type toolLease struct {
	holder  string
	servers []string
	at      time.Time
}

// renewLeases extends the holder's lease on every server, and counts the
// ones that were not live before. A lease never moves back: a holder whose
// clock steps backwards keeps the later expiry it already had.
func renewLeases(ctx context.Context, db settings.DB, lease toolLease) (bool, error) {
	if len(lease.servers) == 0 {
		return false, nil
	}
	if lease.holder == "" {
		return false, ErrNoHolder
	}
	var lapsed int
	if err := db.QueryRow(ctx, `
		with prior as (
			select server from tool_server_leases
			where holder = $1 and server = any($2) and expires_at > $3
		), renewed as (
			insert into tool_server_leases (server, holder, expires_at)
			select distinct s, $1, $4::timestamptz from unnest($2::text[]) s
			on conflict (server, holder) do update set
				expires_at = greatest(tool_server_leases.expires_at, excluded.expires_at)
			returning server
		)
		select count(*) from renewed where server not in (select server from prior)`,
		lease.holder, lease.servers, lease.at, lease.at.Add(ToolServerLease)).Scan(&lapsed); err != nil {
		return false, fmt.Errorf("admin: renew the leases of %s: %w", lease.holder, err)
	}
	return lapsed > 0, nil
}

// leaseUnmanaged leases to the publication's holder every unmanaged server
// it speaks for that the configuration does not hold. Run under the
// catalogue's lock, before the sweep, so what the publication records is
// held when it is recorded.
func leaseUnmanaged(ctx context.Context, db settings.DB, pub domain.ToolPublication, at time.Time) error {
	var servers []string
	for _, s := range pub.Servers {
		if s.Unmanaged {
			servers = append(servers, s.Name)
		}
	}
	_, err := renewLeases(ctx, db, toolLease{holder: pub.Holder, servers: servers, at: at})
	return err
}

/*
unheldUnmanaged is the catalogue row of an unmanaged server nobody holds a
live lease on, and that is due to leave. $2 is the moment it is judged at,
$3 the moment an old worker's grace must have begun by to be over.

A row a lease-era worker wrote carries the lease marker, and leaves as soon
as its leases run out. A row without it was written by a worker from before
leases, which may still be serving it and cannot renew (legacyGrace): it
leaves once its server's grace is over.
*/
const unheldUnmanaged = `coalesce((s.value->>'unmanaged')::boolean, false)
	and not exists (
		select 1 from tool_server_leases l
		where l.server = s.value->>'server' and l.expires_at > $2)
	and (coalesce((s.value->>'leased')::boolean, false)
	     or exists (
		select 1 from tool_server_grace g
		where g.server = s.value->>'server' and g.first_seen <= $3))`

// sweepUnheld drops the tools of unmanaged servers nobody holds. It looks
// before it locks: most passes find nothing, and the catalogue's lock is
// installation-wide.
func (c *Curator) sweepUnheld(ctx context.Context, now time.Time) error {
	// A lease that ran out says nothing a missing one would not; without this
	// every restart under a new pod name would leave a row behind for good.
	if _, err := c.pool.Exec(ctx,
		`delete from tool_server_leases where expires_at <= $1`, now); err != nil {
		return fmt.Errorf("admin: forget leases that ran out: %w", err)
	}
	var found bool
	if err := c.pool.QueryRow(ctx, `
		select exists (
			select 1 from settings s
			where s.scope_kind = 'installation' and s.company_id = '' and s.area_id = ''
			  and s.kind = $1 and ((`+unheldUnmanaged+`) or (`+ungracedLegacy+`)))`,
		toolKind, now, graceBegunBy(now)).Scan(&found); err != nil {
		return fmt.Errorf("admin: look for unheld servers: %w", err)
	}
	if !found {
		return nil
	}
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
	if err := sweepCatalogue(ctx, tx, configured, now); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("admin: commit the sweep: %w", err)
	}
	return nil
}
