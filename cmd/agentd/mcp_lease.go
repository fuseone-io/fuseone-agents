package main

import (
	"context"
	"log/slog"
	"maps"
	"slices"
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

// flagServers is the servers this worker holds because its flags started
// them.
func (r *reconciler) flagServers() []string {
	var out []string
	for _, name := range slices.Sorted(maps.Keys(r.connected)) {
		if r.connected[name] == flagMark {
			out = append(out, name)
		}
	}
	return out
}
