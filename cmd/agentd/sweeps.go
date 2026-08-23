// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/finops"

	"github.com/fuseone/agents/internal/budget"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/worker"
)

// The background loops a worker owns.
//
// Each is a goroutine with an owner: it stops when the worker's context does.
// They are gathered here because they share a shape — a ticker, a pass, a
// logged failure that does not stop the worker — and because a loop added
// beside the wiring it belongs to is a loop nobody finds again.

// runSimulations owns the simulation pool: it stops when the worker's context
// does, like the scheduler beside it.
func runSimulations(ctx context.Context, w *worker.Worker) {
	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("simulation pool stopped", "err", err)
	}
}

// spendSweep is how often planning calls are projected for the cost aggregate.
//
// A minute, not seconds. This exists so a cost screen never folds the chain,
// and a figure that is a minute behind is still a figure somebody can act on —
// nobody decides on model spend in the second after a step. Reading more often
// would put load on the ledger for freshness nothing asks for.
const spendSweep = time.Minute

// spendBatch bounds one pass. Small enough that a busy installation catching
// up does not hold its transaction long, large enough to drain a backlog in
// minutes rather than hours.
const spendBatch = 500

func sweepSpend(ctx context.Context, spend *finops.Spend) {
	sweep(ctx, spendSweep, "planning calls projected", func() (int, error) {
		return spend.Project(ctx, spendBatch)
	})
}

// retentionSweep is how often content past its window is erased.
//
// Daily, because retention is measured in years and a sweep that ran every
// minute would be a delete statement against the whole content table every
// minute for no benefit. The first one runs at start-up so an installation
// that was down past its window does not wait a day to honour it.
const retentionSweep = 24 * time.Hour

// sweepContent erases what is past its retention window, for ever, until its
// context is cancelled.
// budgetSweep is how often spend is compared to the ceilings. Minutes rather
// than seconds: a monthly budget does not move fast, and every pass is a query
// per configured scope.
const budgetSweep = 5 * time.Minute

func watchBudgets(ctx context.Context, watcher *budget.Watcher) {
	ticker := time.NewTicker(budgetSweep)
	defer ticker.Stop()

	for {
		if _, err := watcher.Sweep(ctx); err != nil {
			// Logged and retried. A failed sweep means a warning arrives late,
			// which is worth a line in the log and not worth stopping over.
			slog.Error("budget sweep failed", "err", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// eventSweep is how often finished runs are checked for events to publish.
// Short, because a person waiting for the second agent to start is waiting for
// this; every pass is one indexed query when nothing emits.
const eventSweep = 15 * time.Second

func dispatchEvents(ctx context.Context, dispatcher *trigger.Dispatcher) {
	ticker := time.NewTicker(eventSweep)
	defer ticker.Stop()

	for {
		if opened, err := dispatcher.Sweep(ctx, trigger.Window); err != nil {
			slog.Error("event dispatch failed", "err", err)
		} else if opened > 0 {
			slog.Info("events opened runs", "runs", opened)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func sweepContent(ctx context.Context, erasures *admin.Erasures) {
	ticker := time.NewTicker(retentionSweep)
	defer ticker.Stop()

	for {
		if erased, err := erasures.Sweep(ctx); err != nil {
			// Logged and retried tomorrow. A failed sweep keeps data longer
			// than promised, which is a problem; stopping the worker over it
			// would be a bigger one.
			slog.Error("retention sweep failed", "err", err)
		} else if erased > 0 {
			slog.Info("content erased past its retention window", "objects", erased)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// policyRefresh bounds how long an authored policy takes to reach a running
// worker. Short enough that turning a rule on is a live control rather than a
// deploy; long enough that a pool of workers is not a load generator.
const policyRefresh = 30 * time.Second

// rulingRefresh bounds how long a classification change takes to reach a
// running worker. Short enough that demoting a tool is a live control rather
// than a deploy, long enough that a pool of workers is not a load generator.
const rulingRefresh = 30 * time.Second

// schedulerTick is how often the scheduler looks for due moments. A minute,
// because the finest schedule this platform accepts is a minute — anything
// finer is a queue, and this is not one.
const schedulerTick = time.Minute

// runScheduler ticks until the worker's context ends. Owned by ctx, like every
// other goroutine here: nothing outlives the process that started it.
func runScheduler(ctx context.Context, scheduler *trigger.Scheduler) {
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := scheduler.Tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("scheduler tick failed", "err", err)
			}
		}
	}
}

// refreshRulings keeps a running worker current. Owned by ctx: it stops when
// the worker does.
func refreshRulings(ctx context.Context, catalog *tools.Catalog, curator *admin.Curator) {
	ticker := time.NewTicker(rulingRefresh)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := catalog.Sync(ctx, curator, domain.Scope{}); err != nil && ctx.Err() == nil {
				// A failed refresh leaves the last good rulings in place and
				// says so, rather than widening or narrowing access silently.
				slog.Warn("could not refresh tool classifications", "err", err)
			}
		}
	}
}
