package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/worker"
)

/*
What a worker is built from, and in what order.

Gathered into one value rather than threaded through half a dozen arguments:
the pools, the sweeps and the tool reconciler all need overlapping subsets of
it, and a function taking eight of them positionally is one nobody can call
correctly a year from now.

Every field is optional in the same sense the command is: a worker pointed at
no database runs on the in-memory ledger with the tool servers its flags name,
and the parts that need configuration are nil. That is what makes a laptop with
no administrator a real mode rather than a broken one.
*/
type workerParts struct {
	store Store
	queue worker.Queue

	// content is what the loop reads and writes bulky payloads through, and
	// durable is the same store when there is a database. Retention erases
	// through durable and only can: the engine's port has no erase and should
	// not (PRD AU-04).
	content engine.ContentStore
	durable *ledger.Content
	catalog *tools.Catalog

	// configPool outlives the call that opens it: the sweeps need it after the
	// configuration is read, and opening a second pool for the same database
	// to avoid saying so would be worse.
	configPool   *pgxpool.Pool
	curator      *admin.Curator
	integrations *admin.Integrations
	registry     *spec.Registry
	budgets      *admin.Budgets
	retention    *admin.Retention
	// settings is what the administration area configures, and the channel
	// sweep reads its conversations from it rather than from a table of its
	// own (NT-005 §5 of the note, and the reason the credential is sealed).
	settings *settings.Store
	health   healthRecorder
}

// openWorkerParts connects everything a worker reads before it runs.
func openWorkerParts(ctx context.Context, dsn string) (*workerParts, error) {
	store, err := openStore(ctx, dsn)
	if err != nil {
		return nil, err
	}
	queue, ok := store.(worker.Queue)
	if !ok {
		return nil, errors.New("this store cannot serve a work queue")
	}
	parts := &workerParts{store: store, queue: queue}

	// Durable when there is a database, because a run that already called a
	// tool cannot be resumed by any other process without its earlier content
	// — including this same worker after a restart (PRD NF-02, DE-15).
	parts.content = engine.NewMemoryContent()
	pool, err := contentPool(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if pool != nil {
		parts.durable = ledger.NewContent(pool)
		parts.content = parts.durable
	}
	parts.catalog = tools.NewCatalog(parts.content)

	if err := parts.openConfiguration(ctx, dsn); err != nil {
		return nil, err
	}
	parts.health = healthOf(parts.configPool)
	return parts, nil
}

/*
openConfiguration reads the administration area.

Everything the worker talks to comes from there rather than from the command
line: which tool servers exist, which model providers are configured, and what
each tool is classified as. The flags remain for a laptop that has no
administrator yet.
*/
func (p *workerParts) openConfiguration(ctx context.Context, dsn string) error {
	if dsn == "" {
		return nil
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect for configuration: %w", err)
	}
	p.configPool = pool

	v, err := openVault()
	if err != nil {
		return err
	}
	store := settings.NewStore(pool, v)
	p.settings = store
	p.retention = admin.NewRetention(pool, store)
	p.curator = admin.NewCurator(pool)
	p.integrations = admin.NewIntegrations(pool, store).ForgettingHealth(admin.NewHealth(pool))
	p.registry = spec.NewRegistry(pool)
	p.budgets = admin.NewBudgets(pool, store)
	return nil
}

// Close releases the configuration pool. The store's own connections belong to
// whoever opened it.
func (p *workerParts) Close() {
	if p.configPool != nil {
		p.configPool.Close()
	}
}

/*
connectTools brings up the tool servers, from the flags and from the console.

The flags first: they are the way to point a laptop at a server before there is
an administrator to configure one, and the reconciler must not disconnect what
it did not connect.
*/
func (p *workerParts) connectTools(ctx context.Context, servers []string) error {
	reconcile := newReconciler(p.catalog, p.integrations, p.health)
	if p.curator != nil {
		reconcile = reconcile.publishingTo(p.curator)
	}

	for _, entry := range servers {
		name, command, _ := strings.Cut(entry, "=")
		if err := connectServer(ctx, p.catalog, domain.MCPServer{
			Name: name, Transport: domain.TransportStdio, Command: command,
		}, ""); err != nil {
			slog.Error("tool server did not answer; its tools are unavailable",
				"server", name, "err", err)
			observe(ctx, p.health, name, false, 0, err.Error())
			continue
		}
		reconcile.hold(name)
		count := p.catalog.CountFrom(name)
		slog.Info("tool server connected", "server", name, "transport", "stdio", "tools", count)
		observe(ctx, p.health, name, true, count, "")
	}

	// Then the configured ones, and again on a timer. Before this, a server
	// added from the console offered nothing until somebody restarted the
	// worker, with nothing on any screen saying so.
	if p.integrations != nil {
		reconcile.reconcile(ctx)
		go reconcile.watch(ctx, toolRefresh)
	}

	if p.curator == nil {
		return nil
	}
	if err := p.curator.Publish(ctx, p.catalog.Entries()); err != nil {
		return err
	}
	if err := syncRulings(ctx, p.catalog, p.curator); err != nil {
		return err
	}
	go refreshRulings(ctx, p.catalog, p.curator)
	return nil
}

/*
gate is the policy set in force, refreshed on a timer.

The Gate itself never queries. It is on the path of every effect, and a
decision must not wait on a database to find out whether something is allowed.
*/
func (p *workerParts) gate(ctx context.Context) (*policy.Enforcer, error) {
	enforcer := policy.NewEnforcer(policySource(p.configPool), slog.Default())
	if p.configPool == nil {
		return enforcer, nil
	}
	if err := enforcer.Refresh(ctx); err != nil {
		return nil, fmt.Errorf("read the policy set: %w", err)
	}
	go enforcer.Watch(ctx, policyRefresh)
	return enforcer, nil
}

// deps is what the loop runs on.
func (p *workerParts) deps(gate engine.Gate) engine.Deps {
	return engine.Deps{
		Ledger:  p.store,
		Gate:    gate,
		Tools:   p.catalog,
		Catalog: p.catalog,
		Content: p.content,
		Clock:   engine.SystemClock{},
	}
}
