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
		if err == nil && r.rulings != nil {
			_, err = r.catalog.Sync(ctx, r.rulings, domain.Scope{})
		}
		if err != nil {
			slog.Error("tool server rediscovery failed; its tools are unchanged",
				"server", name, "generation", generation, "err", err)
		}
		r.answer(ctx, name, generation, err == nil)
		changed = changed || err == nil
	}
	return changed
}

func (r *reconciler) answer(ctx context.Context, server string, generation int64, succeeded bool) {
	if r.rediscovery.answered == nil {
		r.rediscovery.answered = map[string]int64{}
	}
	r.rediscovery.answered[server] = generation
	store, ok := r.servers.(rediscoveryStore)
	if !ok || r.holder == "" {
		return
	}
	if err := store.RecordMCPRediscovery(ctx, domain.MCPRediscoveryReceipt{
		Server: server, Worker: r.holder, Generation: generation, Succeeded: succeeded,
	}); err != nil {
		slog.Error("could not record a tool server rediscovery", "server", server, "err", err)
	}
}
