package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/autonomy"
	"github.com/fuseone/agents/internal/budget"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/worker"
)

/*
The loops a worker starts beside the one that drains the queue.

Every one is a goroutine with an owner: it stops when the worker's context
does. Each is conditional on what this installation has — a worker on the
in-memory ledger starts none of them, which is why a laptop with no database
is a real mode rather than a broken one.

They are started together, in one place, because a loop added next to the
wiring it happens to need is a loop nobody finds again — which is how the
command that used to hold all of this reached five times its size limit.
*/
func (p *workerParts) startLoops(ctx context.Context, cfg workerFlags, sim *worker.Worker) {
	if simulations(p.store) != nil {
		go runSimulations(ctx, sim)
		slog.Info("simulation pool started", "slots", simulationSlots(cfg.concurrency))
	}
	if p.configPool == nil {
		return
	}

	// Retention. It reads the configured window on every pass, so shortening
	// it takes effect on the next sweep rather than at the next deploy —
	// which is the whole reason it is a setting.
	if p.durable != nil && p.retention != nil {
		go sweepContent(ctx, admin.NewErasures(p.configPool, p.durable, p.retention))
	}

	// Demoting an agent people keep overruling. Promotion is not here on
	// purpose: it is only ever suggested, and a person does it.
	if agreements, ok := p.store.(autonomy.Agreements); ok {
		go watchAutonomy(ctx, autonomy.New(
			agreements, spec.NewState(p.configPool),
			demotions{pool: p.configPool}, slog.Default()))
	}

	// Warning before the ceiling rather than at it. A limit that says nothing
	// until it stops the work is a limit discovered by a run parking
	// mid-afternoon (PRD FO-05).
	if p.budgets != nil {
		if spender, ok := p.store.(budget.Spend); ok {
			go watchBudgets(ctx, budget.NewWatcher(
				p.budgets, spender, admin.NewMarks(p.configPool),
				engine.SystemClock{}, slog.Default()))
		}
	}

	if p.registry != nil {
		// Every worker runs a scheduler. They do not coordinate, because the
		// run's idempotency key is derived from the due moment and the ledger
		// accepts exactly one of them.
		go runScheduler(ctx, trigger.NewScheduler(
			trigger.NewPostgresSchedules(p.configPool), p.opener(),
			engine.SystemClock{}, slog.Default(),
		))

		// Composition between agents (PRD SE-10). A sweep rather than a hook
		// on the finishing run: a worker that died between finishing and
		// publishing would drop the event, and a sweep cannot — the run it
		// opens carries a key derived from the source run, so a second pass
		// reaches the run the first one opened.
		go dispatchEvents(ctx, trigger.NewDispatcher(
			p.registry, p.store, p.opener(), engine.SystemClock{}, slog.Default(),
		))
	}
}

/*
opener is how anything unattended starts a run.

One builder rather than two identical chains: the schedule and the event
dispatch must honour the same pauses, the same stops and the same stages, and
two lists of them is two lists that drift. A stop honoured by the schedule and
not by an event is a stop that quietens half the platform.
*/
func (p *workerParts) opener() *trigger.Opener {
	return trigger.NewOpener(p.store, p.registry, engine.SystemClock{}).
		WithContent(ledger.NewContent(p.configPool)).
		WithPauses(spec.NewState(p.configPool)).
		WithStops(admin.NewStops(p.configPool)).
		WithStages(spec.NewState(p.configPool))
}

// isCancelled reports the ordinary shutdown rather than a failure.
func isCancelled(err error) bool { return errors.Is(err, context.Canceled) }
