package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
)

/*
Rediscovery on request (#287): every worker asks a server again what it offers,
once, on the session it already holds.

The administration keeps a generation per server; each worker remembers the
generation it last answered and owes one rediscovery while it is behind. A
worker that connects after the request answers it with that connection's own
discovery, so one that starts later is not left behind and is not asked twice.
A failure is an answer too: the session and the tools stay as they were, the
receipt says it failed, and nothing is retried until somebody asks again.
*/

// rediscoveryStore is what a worker needs from the administration to answer
// requests, declared here by the consumer. Optional: a worker whose store has
// no such requests simply never rediscovers on request.
type rediscoveryStore interface {
	MCPRediscoveries(ctx context.Context) (map[string]int64, error)
	RecordMCPRediscovery(ctx context.Context, r domain.MCPRediscoveryReceipt) error
}

// rediscoveries is the reconciler's view of the requests: what was asked, as
// read at the start of this pass, and what this worker has answered.
type rediscoveries struct {
	requested map[string]int64
	answered  map[string]int64
	// unrecorded is each answer the store has not kept yet. The answer stands
	// and is not repeated; only its record is retried, on every pass.
	unrecorded map[string]domain.MCPRediscoveryReceipt
}

// readRediscoveries takes this pass's requests. A failed read asks nothing of
// this pass, and the next one reads again.
func (r *reconciler) readRediscoveries(ctx context.Context) {
	store, ok := r.servers.(rediscoveryStore)
	if !ok {
		return
	}
	requested, err := store.MCPRediscoveries(ctx)
	if err != nil {
		slog.Error("could not read tool server rediscovery requests", "err", err)
		requested = nil
	}
	r.rediscovery.requested = requested
}

// answeredByConnecting records that a fresh connection is this worker's answer
// to whatever had been asked of the server when the pass began.
func (r *reconciler) answeredByConnecting(ctx context.Context, server string) {
	r.answer(ctx, server, r.rediscovery.requested[server], true)
}

// rediscover answers every request this worker is behind on, for the servers
// it holds, and reports whether any found something to publish.
func (r *reconciler) rediscover(ctx context.Context, wanted map[string]domain.MCPServer) bool {
	r.recordUnrecorded(ctx)
	changed := false
	for name, generation := range r.rediscovery.requested {
		server, held := wanted[name]
		if !held || !r.catalog.Connected(name) || r.rediscovery.answered[name] >= generation {
			continue
		}
		err := r.catalog.Rediscover(ctx, name, server.Surface)
		if errors.Is(err, tools.ErrSessionReplaced) {
			// The replacement's own discovery answers the request.
			continue
		}
		if err != nil {
			slog.Error("tool server rediscovery failed; its tools are unchanged",
				"server", name, "generation", generation, "err", err)
		} else {
			r.applyRulingsAfterRediscovery(ctx, name)
		}
		r.answer(ctx, name, generation, err == nil)
		changed = changed || err == nil
	}
	return changed
}

/*
applyRulingsAfterRediscovery is the second half of a rediscovery that already
happened, so it cannot undo the first. By now the catalogue holds what the
server offers: an unchanged tool kept its ruling, and a new or changed one is
unclassified, which the Gate refuses. A failure to read the rulings leaves that
safe state for the periodic refresh to settle, and the rediscovery stands.
*/
func (r *reconciler) applyRulingsAfterRediscovery(ctx context.Context, server string) {
	if r.rulings == nil {
		return
	}
	if _, err := r.catalog.Sync(ctx, r.rulings, domain.Scope{}); err != nil {
		slog.Warn("tool server rediscovered, but its rulings could not be read yet; new tools stay unclassified",
			"server", server, "err", err)
	}
}

func (r *reconciler) answer(ctx context.Context, server string, generation int64, succeeded bool) {
	if r.rediscovery.answered == nil {
		r.rediscovery.answered = map[string]int64{}
	}
	r.rediscovery.answered[server] = generation
	if r.holder == "" {
		return
	}
	if r.rediscovery.unrecorded == nil {
		r.rediscovery.unrecorded = map[string]domain.MCPRediscoveryReceipt{}
	}
	// A newer answer replaces one still waiting: the store keeps only the newest.
	r.rediscovery.unrecorded[server] = domain.MCPRediscoveryReceipt{
		Server: server, Worker: r.holder, Generation: generation, Succeeded: succeeded,
	}
	r.recordUnrecorded(ctx)
}

// recordUnrecorded hands the store every answer it has not kept yet, and keeps
// the ones it refused for the next pass.
func (r *reconciler) recordUnrecorded(ctx context.Context) {
	store, ok := r.servers.(rediscoveryStore)
	if !ok {
		return
	}
	for server, receipt := range r.rediscovery.unrecorded {
		if err := store.RecordMCPRediscovery(ctx, receipt); err != nil {
			slog.Error("could not record a tool server rediscovery; will try again",
				"server", server, "err", err)
			continue
		}
		delete(r.rediscovery.unrecorded, server)
	}
}
