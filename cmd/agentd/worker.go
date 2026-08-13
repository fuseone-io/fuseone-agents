// Command agentd is the FuseOne Agents server.
//
// One binary, one Postgres, nothing else required (PRD DE-01). Subcommands
// select the role a process plays inside the installation.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/simulate"

	"github.com/fuseone/agents/internal/admin"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/autonomy"
	"github.com/fuseone/agents/internal/budget"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/worker"
)

// The worker command: the pools that drain the queue, and what they are built from.

// workerCmd runs a pool that advances runs until it is signalled to stop.
//
// It is a separate process from the API on purpose: the two scale on different
// axes, and a pool that needs isolation — its own network policy, its own
// resource limits — becomes its own deployment without touching the server.
func workerCmd(args []string) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	owner := fs.String("owner", defaultOwner(), "identifies this process in a lease")
	concurrency := fs.Int("concurrency", 4, "runs advanced at once")
	lease := fs.Duration("lease", 2*time.Minute, "must outlast the slowest single turn")
	specDir := fs.String("specs", "agents", "directory of *.agent.md definitions")
	var servers mcpServers
	fs.Var(&servers, "mcp", "an MCP server as name=command [args]; repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openStore(ctx, *dsn)
	if err != nil {
		return err
	}
	queue, ok := store.(worker.Queue)
	if !ok {
		return errors.New("this store cannot serve a work queue")
	}

	// Everything except the planner, which is resolved per run from the
	// agent's own definition.
	// Durable when there is a database, because a run that already called a
	// tool cannot be resumed by any other process without its earlier content
	// — including this same worker after a restart (PRD NF-02, DE-15).
	var content engine.ContentStore = engine.NewMemoryContent()
	// durable outlives the block: retention erases through it, and it is the
	// only handle that can — the engine's port has no erase and should not.
	var durable *ledger.Content
	if pool, err := contentPool(ctx, *dsn); err != nil {
		return err
	} else if pool != nil {
		durable = ledger.NewContent(pool)
		content = durable
	}
	catalog := tools.NewCatalog(content)

	// Everything the worker talks to comes from the administration area rather
	// than from this command line: which tool servers exist, which model
	// providers are configured, and what each tool is classified as. The flags
	// remain for a laptop that has no administrator yet.
	var (
		curator      *admin.Curator
		integrations *admin.Integrations
		registry     *spec.Registry
		budgets      *admin.Budgets
		retention    *admin.Retention
		// configPool outlives the block that opens it: the scheduler needs it
		// after the configuration is read, and opening a second pool for the
		// same database to avoid saying so would be worse.
		configPool *pgxpool.Pool
	)
	if *dsn != "" {
		pool, err := pgxpool.New(ctx, *dsn)
		if err != nil {
			return fmt.Errorf("connect for configuration: %w", err)
		}
		defer pool.Close()
		configPool = pool

		v, err := openVault()
		if err != nil {
			return err
		}
		store := settings.NewStore(pool, v)
		retention = admin.NewRetention(pool, store)
		curator = admin.NewCurator(pool)
		integrations = admin.NewIntegrations(pool, store).ForgettingHealth(admin.NewHealth(pool))
		registry = spec.NewRegistry(pool)
		budgets = admin.NewBudgets(pool, store)

	}

	// The flags first: they are the way to point a laptop at a server before
	// there is an administrator to configure one, and the reconciler must not
	// disconnect what it did not connect.
	health := healthOf(configPool)
	reconcile := newReconciler(catalog, integrations, health)
	if curator != nil {
		reconcile = reconcile.publishingTo(curator)
	}
	for _, entry := range servers {
		name, command, _ := strings.Cut(entry, "=")
		if err := connectServer(ctx, catalog, domain.MCPServer{
			Name: name, Transport: domain.TransportStdio, Command: command,
		}, ""); err != nil {
			slog.Error("tool server did not answer; its tools are unavailable",
				"server", name, "err", err)
			observe(ctx, health, name, false, 0, err.Error())
			continue
		}
		reconcile.hold(name)
		count := catalog.CountFrom(name)
		slog.Info("tool server connected", "server", name, "transport", "stdio", "tools", count)
		observe(ctx, health, name, true, count, "")
	}

	// Then the configured ones, and again on a timer. Before this, a server
	// added from the console offered nothing until somebody restarted the
	// worker, with nothing on any screen saying so.
	if integrations != nil {
		reconcile.reconcile(ctx)
		go reconcile.watch(ctx, toolRefresh)
	}

	if curator != nil {
		if err := curator.Publish(ctx, catalog.Entries()); err != nil {
			return err
		}
		if err := syncRulings(ctx, catalog, curator); err != nil {
			return err
		}
		go refreshRulings(ctx, catalog, curator)
	}

	// The set in force, refreshed on a timer. The Gate itself never queries:
	// it is on the path of every effect, and a decision must not wait on a
	// database to find out whether something is allowed.
	enforcer := policy.NewEnforcer(policySource(configPool), slog.Default())
	if configPool != nil {
		if err := enforcer.Refresh(ctx); err != nil {
			return fmt.Errorf("read the policy set: %w", err)
		}
		go enforcer.Watch(ctx, policyRefresh)
	}

	deps := engine.Deps{
		Ledger:  store,
		Gate:    enforcer,
		Tools:   catalog,
		Catalog: catalog,
		Content: content,
		Clock:   engine.SystemClock{},
	}

	specs, err := loadSpecs(ctx, *specDir, catalog, integrations)
	if err != nil {
		return err
	}

	// What this worker loaded becomes what the installation has published, so
	// the API can answer "which agents exist" without reading somebody's disk.
	// Publishing the same version twice is a no-op: the version is the digest
	// of the content.
	if registry != nil {
		published, err := publishSpecs(ctx, registry, *specDir)
		if err != nil {
			return err
		}
		slog.Info("agent versions published", "count", published, "dir", *specDir)

		// A definition arriving from disk is authoring, and authoring never
		// starts anything. An agent already started stays started: this only
		// records the ones nobody has decided about.
		if err := pauseNewAgents(ctx, configPool, specDir); err != nil {
			return err
		}

		// What each version declares becomes what the scheduler watches. A
		// schedule withdrawn by a new version stops firing; one that is still
		// declared keeps the moment it was already waiting for.
		if err := syncSchedules(ctx, configPool, specDir); err != nil {
			return err
		}
		if err := syncWebhooks(ctx, configPool, specDir); err != nil {
			return err
		}
	}

	w := worker.New(worker.Config{
		Owner: *owner, Concurrency: *concurrency, Lease: *lease,
	}, queue, deps, specs, engine.SystemClock{}, slog.Default()).
		// What takes each tool back, for a run somebody abandons. The same
		// catalogue that says what a tool does: the Curator rules on both in
		// one act, because they are one judgement (PRD SE-08).
		WithUndos(catalog)

	// The other half of the queue, drained by a pool built with the dry tool
	// layer. A separate pool rather than a branch inside this one: the branch
	// would be a single mistaken condition away from executing a dry run's
	// proposals against production, and the two claims cannot reach each
	// other's runs.
	//
	// Smaller, because a simulation is somebody waiting at a screen for a set
	// of a few dozen, not the installation's steady load — and because the
	// budget it spends is real.
	dry := simulations(store)
	sim := worker.New(worker.Config{
		Owner: *owner + "-sim", Concurrency: simulationSlots(*concurrency), Lease: *lease,
	}, dry, simulate.Deps(deps), specs, engine.SystemClock{}, slog.Default()).
		WithUndos(catalog)

	if configPool != nil {
		// How far each agent is trusted. Without it the pool treats every
		// agent as a draft, which escalates every effect — the safe reading
		// of a missing wire, and a very loud one.
		stages := spec.NewState(configPool)
		w = w.WithStages(stages)
		sim = sim.WithStages(stages)
	}

	if budgets != nil {
		if spender, ok := store.(spendReader); ok {
			w = w.WithCeilings(ceilings{Budgets: budgets, spend: spender})
			// The same ceilings. A simulation is dry at the tool layer and
			// nowhere else: every planning call is billed by the provider, and
			// a set of fifty that ignored the scope's budget would be the
			// cheapest way to exhaust it.
			sim = sim.WithCeilings(ceilings{Budgets: budgets, spend: spender})
		}
	}

	if dry != nil {
		go runSimulations(ctx, sim)
		slog.Info("simulation pool started", "slots", simulationSlots(*concurrency))
	}

	// Retention, on a timer with an owner. It reads the configured window on
	// every pass, so shortening it takes effect on the next sweep rather than
	// at the next deploy — which is the whole reason it is a setting.
	if configPool != nil && durable != nil && retention != nil {
		go sweepContent(ctx, admin.NewErasures(configPool, durable, retention))
	}

	// Demoting an agent people keep overruling. Promotion is not here on
	// purpose: it is only ever suggested, and a person does it.
	if configPool != nil {
		if agreements, ok := store.(autonomy.Agreements); ok {
			go watchAutonomy(ctx, autonomy.New(
				agreements, spec.NewState(configPool),
				demotions{pool: configPool}, slog.Default()))
		}
	}

	// Warning before the ceiling rather than at it. A limit that says nothing
	// until it stops the work is a limit discovered by a run parking
	// mid-afternoon (PRD FO-05).
	if configPool != nil && budgets != nil {
		if spender, ok := store.(budget.Spend); ok {
			go watchBudgets(ctx, budget.NewWatcher(
				budgets, spender, admin.NewMarks(configPool),
				engine.SystemClock{}, slog.Default()))
		}
	}

	// The scheduler is a goroutine with an owner: it stops when the worker's
	// context does. Every worker runs one — they do not coordinate, because
	// the run's idempotency key is derived from the due moment and the ledger
	// accepts exactly one of them.
	if configPool != nil && registry != nil {
		scheduler := trigger.NewScheduler(
			trigger.NewPostgresSchedules(configPool),
			trigger.NewOpener(store, registry, engine.SystemClock{}).
				WithContent(ledger.NewContent(configPool)).
				WithPauses(spec.NewState(configPool)).
				// The schedule is the one that fires unattended, so it is the
				// one a stop most needs to reach.
				WithStops(admin.NewStops(configPool)).
				WithStages(spec.NewState(configPool)),
			engine.SystemClock{}, slog.Default(),
		)
		go runScheduler(ctx, scheduler)

		// Composition between agents (PRD SE-10). A sweep rather than a hook
		// on the finishing run: a worker that died between finishing and
		// publishing would drop the event, and a sweep cannot — the run it
		// opens carries a key derived from the source run, so a second pass
		// reaches the run the first one opened.
		go dispatchEvents(ctx, trigger.NewDispatcher(
			registry, store,
			trigger.NewOpener(store, registry, engine.SystemClock{}).
				WithContent(ledger.NewContent(configPool)).
				WithPauses(spec.NewState(configPool)).
				WithStops(admin.NewStops(configPool)).
				WithStages(spec.NewState(configPool)),
			engine.SystemClock{}, slog.Default(),
		))
	}

	slog.Info("worker started", "owner", *owner, "concurrency", *concurrency)
	err = w.Run(ctx)
	if errors.Is(err, context.Canceled) {
		slog.Info("worker stopped")
		return nil
	}
	return err
}

// simulations returns the half of the queue holding simulated runs, or nil
// when the store cannot serve one.
func simulations(store Store) worker.Queue {
	type simulatable interface{ Simulations() ledger.SimulationQueue }
	if s, ok := store.(simulatable); ok {
		return s.Simulations()
	}
	return nil
}

// simulationSlots keeps the simulation pool a fraction of the main one, and
// never zero: an installation running a single worker still has to be able to
// simulate, or an agent could never leave Draft.
func simulationSlots(concurrency int) int {
	if slots := concurrency / 2; slots > 0 {
		return slots
	}
	return 1
}
