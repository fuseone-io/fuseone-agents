package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/connect"
	"github.com/fuseone/agents/internal/drift"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/simulate"
)

/*
The corpus on a clock, and the notice when it moves.

Two sweeps rather than one, and deliberately not chained: a battery is a set
of runs the pool advances over minutes, so a loop that opened one and then
read it would be reading a half-fold every time. The reading pass compares
whatever has settled since — which means a worker that died between the two
loses nothing, because both are sweeps and a sweep runs again.

The pass is hourly and the battery is daily. Reading is a query that finds
nothing on most passes; running one costs a real set of model calls, so the
interval that matters is the one on the battery.
*/
const driftPass = time.Hour

// watchDrift runs the corpus on a clock and says so when the world moves
// underneath an agent nobody changed (NT-006 §3).
func watchDrift(ctx context.Context, p *workerParts, store *settings.Store) {
	corpora := regression.NewStore(p.configPool)
	notice := channel.NewNotice(
		channel.NewConfigured(store), channel.NewRouter(connect.New(store)))

	watch := drift.New(corpora, p.store, folded{p.store}, notice, slog.Default())
	rerun := drift.NewRerun(corpora,
		p.store, regression.NewCorpus(corpora, ledger.NewContent(p.configPool)),
		p.opener(), slog.Default()).Every(drift.Default)

	ticker := time.NewTicker(driftPass)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			// Read first. A battery opened this pass has not settled, so
			// what is compared is the one from before — which is the point.
			if moved, err := watch.Sweep(ctx, now); err != nil {
				slog.Error("the drift sweep did not finish", "err", err)
			} else if moved > 0 {
				slog.Warn("agents drifted", "agents", moved)
			}

			if opened, err := rerun.Sweep(ctx, now); err != nil {
				slog.Error("scheduled batteries did not open", "err", err)
			} else if opened > 0 {
				slog.Info("corpora re-run on the clock", "batteries", opened)
			}
		}
	}
}

// folded reads a simulation back as the report it always was. A named type
// rather than a method on the store: folding is a question about the ledger's
// contents, and the ledger has no business knowing what a report is.
type folded struct{ store Store }

func (f folded) Fold(ctx context.Context, simulation string) (simulate.Report, error) {
	return simulate.Gather(ctx, f.store, simulation)
}
