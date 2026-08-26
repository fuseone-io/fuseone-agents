package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/simulate"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/worker"
)

// workerFlags is how this process is told what it is.
//
// A struct rather than a handful of pointers threaded onward: the pools read
// four of them and the sweeps read two, and passing them positionally is how
// the lease ends up where the concurrency belongs.
type workerFlags struct {
	dsn         string
	owner       string
	concurrency int
	lease       time.Duration
	specDir     string
	servers     mcpServers
	// baseURL is where a notification sends somebody to act. A message about
	// a run waiting for approval that does not link to it is a message that
	// makes the reader go looking.
	baseURL string
	// metricsAddr is a worker-only listener for Prometheus. Empty disables it.
	metricsAddr string
}

func readWorkerFlags(args []string) (workerFlags, error) {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	var cfg workerFlags
	fs.StringVar(&cfg.dsn, "dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	fs.StringVar(&cfg.baseURL, "base-url", os.Getenv("FUSEONE_BASE_URL"),
		"where the console answers, for the links a notification carries")
	fs.StringVar(&cfg.metricsAddr, "metrics-addr", envOr("FUSEONE_WORKER_METRICS_ADDR", ":9090"),
		"address for worker Prometheus metrics; empty disables it")
	fs.StringVar(&cfg.owner, "owner", defaultOwner(), "identifies this process in a lease")
	fs.IntVar(&cfg.concurrency, "concurrency", 4, "runs advanced at once")
	fs.DurationVar(&cfg.lease, "lease", 2*time.Minute, "must outlast the slowest single turn")
	fs.StringVar(&cfg.specDir, "specs", "agents", "directory of *.agent.md definitions")
	fs.Var(&cfg.servers, "mcp", "an MCP server as name=command [args]; repeatable")
	if err := fs.Parse(args); err != nil {
		return workerFlags{}, err
	}
	return cfg, nil
}

/*
workerCmd drains the queue until it is signalled to stop.

The order below is the order the parts depend on each other, and it is the
whole content of this function: connect what the worker reads, bring up the
tool servers, load the definitions, build the pools, start the loops, run.
*/
func workerCmd(args []string) error {
	cfg, err := readWorkerFlags(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	parts, err := openWorkerParts(ctx, cfg.dsn)
	if err != nil {
		return err
	}
	defer parts.Close()

	metrics := worker.NewMetricsRegistry()
	if err := startWorkerMetrics(ctx, cfg.metricsAddr, metrics); err != nil {
		return err
	}
	parts.catalog.WithMetrics(metrics)

	if err := parts.connectTools(ctx, cfg.servers, metrics); err != nil {
		return err
	}

	gate, err := parts.gate(ctx)
	if err != nil {
		return err
	}

	specs, err := parts.loadAndPublish(ctx, cfg.specDir)
	if err != nil {
		return err
	}

	w, sim := parts.pools(cfg, parts.deps(gate, metrics), specs, metrics)
	parts.startLoops(ctx, cfg, sim, metrics)

	slog.Info("worker started", "owner", cfg.owner, "concurrency", cfg.concurrency)
	if err := w.Run(ctx); err != nil && !isCancelled(err) {
		return err
	}
	slog.Info("worker stopped")
	return nil
}

/*
loadAndPublish reads the definitions and records what this worker holds.

What a worker loaded becomes what the installation has published, so the API
can answer "which agents exist" without reading somebody's disk. Publishing the
same version twice is a no-op: the version is the digest of the content.
*/
func (p *workerParts) loadAndPublish(ctx context.Context, dir string) (worker.Specs, error) {
	specs, err := loadSpecs(ctx, dir, p.toolSchemas(), p.integrations, p.registry)
	if err != nil {
		return nil, err
	}
	if p.registry == nil {
		return specs, nil
	}

	published, err := publishSpecs(ctx, p.registry, dir)
	if err != nil {
		return nil, err
	}
	slog.Info("agent versions published", "count", published, "dir", dir)

	// An agent nobody has decided about is recorded as paused. Loading a file
	// never starts anything.
	if err := pauseNewAgents(ctx, p.configPool, &dir); err != nil {
		return nil, err
	}
	if err := syncSchedules(ctx, p.configPool, &dir); err != nil {
		return nil, err
	}
	if err := syncWebhooks(ctx, p.configPool, &dir); err != nil {
		return nil, err
	}
	return specs, nil
}

/*
pools builds the two halves of the queue.

Two pools rather than one with a branch inside it: the branch would be a single
mistaken condition away from executing a dry run's proposals against
production, and two claims cannot reach each other's runs.

The simulation pool is smaller, because a simulation is somebody waiting at a
screen for a set of a few dozen rather than the installation's steady load —
and because the budget it spends is real.
*/
func (p *workerParts) pools(
	cfg workerFlags, deps engine.Deps, specs worker.Specs, metrics *worker.MetricsRegistry,
) (*worker.Worker, *worker.Worker) {
	// What takes each tool back, for a run somebody abandons. The same
	// catalogue that says what a tool does: the Curator rules on both in one
	// act, because they are one judgement (PRD SE-08).
	w := worker.New(worker.Config{
		Owner: cfg.owner, Concurrency: cfg.concurrency, Lease: cfg.lease,
	}, p.queue, deps, specs, engine.SystemClock{}, slog.Default()).
		WithUndos(p.catalog).
		WithMetrics(metrics.Pool("runs", cfg.concurrency))

	sim := worker.New(worker.Config{
		Owner:       cfg.owner + "-sim",
		Concurrency: simulationSlots(cfg.concurrency),
		Lease:       cfg.lease,
	}, simulations(p.store), simulate.Deps(deps), specs, engine.SystemClock{}, slog.Default()).
		WithUndos(p.catalog).
		WithMetrics(metrics.Pool("simulations", simulationSlots(cfg.concurrency)))

	if p.configPool != nil {
		// How far each agent is trusted. Without it the pool treats every
		// agent as a draft, which escalates every effect — the safe reading of
		// a missing wire, and a very loud one.
		stages := spec.NewState(p.configPool)
		w, sim = w.WithStages(stages), sim.WithStages(stages)
	}
	if p.budgets != nil {
		if spender, ok := p.store.(spendReader); ok {
			limits := ceilings{Budgets: p.budgets, spend: spender}
			// The same ceilings on both. A simulation is dry at the tool layer
			// and nowhere else: every planning call is billed by the provider,
			// and a set of fifty that ignored the scope's budget would be the
			// cheapest way to exhaust it.
			w, sim = w.WithCeilings(limits), sim.WithCeilings(limits)
		}
	}
	return w, sim
}
