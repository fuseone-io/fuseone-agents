package main

import (
	"context"
	"log/slog"
	"maps"
	"slices"

	"github.com/fuseone/agents/internal/domain"
)

/*
toolLeases is where a worker says it still runs the servers its flags started
(#223).

A --mcp server has no configuration row, so nothing in the configuration can
say it is gone. What says it is still served is the lease each worker running
it renews on every pass; when the last one runs out, any worker's pass takes
its tools away. That is why a worker holding no flag server renews too: when
the last worker drops a flag, it is a worker holding nothing whose pass
notices.
*/
type toolLeases interface {
	HoldToolServers(ctx context.Context, holder string, servers []string) (bool, error)
}

// holdingAs names this worker in the leases it holds, and where they are
// kept.
func (r *reconciler) holdingAs(holder string, leases toolLeases) *reconciler {
	r.holder = holder
	r.leases = leases
	return r
}

// renewLeases renews this worker's leases on the servers its flags started,
// and reports whether it must publish: a lease that had run out means the
// server's tools may have been swept, and only a publication puts them back.
func (r *reconciler) renewLeases(ctx context.Context) bool {
	if r.leases == nil {
		return false
	}
	lapsed, err := r.leases.HoldToolServers(ctx, r.holder, r.flagServers())
	if err != nil {
		slog.Error("could not renew the leases on the command-line tool servers", "err", err)
		return false
	}
	return lapsed
}

// flagServers is the servers this worker serves because its flags started
// them: held, and with a live session. A session that died is not served,
// and leasing it would keep its tools on offer for as long as the worker ran.
func (r *reconciler) flagServers() []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(r.connected)) {
		if r.connected[name] == flagMark && r.catalog.Connected(name) {
			out = append(out, name)
		}
	}
	return out
}

/*
reviveFlagServers starts again every server a flag names whose session is not
live — it died, or never came up — and reports whether one did.

The flag still names it, so it is reconnected rather than dropped, as a
configured server whose session died is. Until a reconnect takes, the server
is not leased, and once its lease runs out its tools leave; a reconnect that
takes is worth a publication, which leases it again.
*/
func (r *reconciler) reviveFlagServers(ctx context.Context) bool {
	revived := false
	for _, name := range slices.Sorted(maps.Keys(r.flags)) {
		if r.catalog.Connected(name) {
			continue
		}
		if r.connectFlag(ctx, r.flags[name]) {
			revived = true
		}
	}
	return revived
}

// connectFlag connects one flag server, from what the flag says: no
// credentials, which a flag cannot carry.
func (r *reconciler) connectFlag(ctx context.Context, server domain.MCPServer) bool {
	// What a dead session left behind; RemoveServer of nothing is nothing.
	_ = r.catalog.RemoveServer(server.Name)
	if err := r.connectTo(ctx, r.catalog, server, domain.MCPCredentials{},
		nil, nil, credentialPolicy{}, r.egress); err != nil {
		slog.Error("tool server did not answer; its tools are unavailable",
			"server", server.Name, "err", err)
		observe(ctx, r.health, server.Name, false, 0, err.Error())
		return false
	}
	if r.rulings != nil {
		if _, err := r.catalog.Sync(ctx, r.rulings, domain.Scope{}); err != nil {
			slog.Error("could not apply tool classifications after discovery",
				"server", server.Name, "err", err)
		}
	}
	count := r.catalog.CountFrom(server.Name)
	slog.Info("tool server connected", "server", server.Name, "transport", "stdio", "tools", count)
	observe(ctx, r.health, server.Name, true, count, "")
	return true
}
