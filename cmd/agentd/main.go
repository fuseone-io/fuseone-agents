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
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fuseone/agents/internal/admin"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/tools"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/web"
	"github.com/fuseone/agents/internal/worker"
)

var version = "0.1.0-dev"

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(os.Args[1:]); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "worker":
		return workerCmd(args[1:])
	case "start":
		return startCmd(args[1:])
	case "keygen":
		return keygen()
	case "bootstrap":
		return bootstrapCmd(args[1:])
	case "migrate":
		return migrate(args[1:])
	case "version":
		fmt.Println(version)
		return nil
	default:
		return usage()
	}
}

func usage() error {
	fmt.Fprint(os.Stderr, `agentd — FuseOne Agents

usage:
  agentd serve [flags]   run the API server
  agentd worker [flags]  advance claimed runs continuously
  agentd start --agent   open a run for the worker to pick up
  agentd keygen          print a new master key for sealing credentials
  agentd bootstrap --dsn apply or reissue the first-run setup token
  agentd migrate --dsn   apply pending database migrations
  agentd version         print the build version
`)
	return errors.New("no command given")
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "listen address")
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string; in-memory when empty")
	demo := fs.Bool("demo", false, "seed the ledger with example runs")
	baseURL := fs.String("base-url", envOr("FUSEONE_BASE_URL", "http://127.0.0.1:8080"),
		"the console's public URL; identity providers redirect back to it")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	store, err := openStore(ctx, *dsn)
	if err != nil {
		return err
	}

	identity, err := openIdentity(ctx, *dsn, *baseURL)
	if err != nil {
		return err
	}
	if *demo {
		if err := seedDemo(ctx, store); err != nil {
			return fmt.Errorf("seed demo: %w", err)
		}
		slog.Info("seeded demo ledger")
	}

	api := httpapi.NewServer(store, version)
	if identity != nil {
		// The administration area needs a database: it is where rulings and
		// their trail live. An installation on the in-memory ledger serves
		// runs and answers the admin endpoints empty.
		curator := admin.NewCurator(identity.pool)

		// The vault is optional here and required in the worker. This process
		// reports that a credential exists; it never opens one. Refusing to
		// boot without the key would stop an installation that has not
		// configured a provider yet from starting at all — and configuring one
		// is what the administration area is for.
		v, err := openVault()
		if err != nil {
			slog.Warn("no master key; credentials cannot be stored from the console until one is set",
				"variable", vault.KeyEnv)
		}
		store := settings.NewStore(identity.pool, v)
		integrations := admin.NewIntegrations(identity.pool, store)
		api = api.WithAdministration(curator, curator, integrations).
			WithAgents(spec.NewRegistry(identity.pool)).
			WithCeilings(admin.NewBudgets(identity.pool, store)).
			// The same store the worker writes into. Without it the console
			// can show that an approval is pending but not what it is for,
			// which is the one thing the approver needs.
			WithContent(ledger.NewContent(identity.pool)).
			WithWebhooks(trigger.NewPostgresWebhooks(identity.pool)).
			WithAudit(audit.NewPostgres(identity.pool)).
			WithHealth(admin.NewHealth(identity.pool))
	}

	apiHandler := openapi.HandlerWithOptions(
		openapi.NewStrictHandler(api, nil),
		openapi.StdHTTPServerOptions{
			BaseURL:    "/api/v1",
			BaseRouter: http.NewServeMux(),
			ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
				writeProblem(w, http.StatusBadRequest, "Invalid request", err.Error())
			},
		},
	)

	// The API owns /api/; everything else is the console, which falls back to
	// index.html so browser routing works on a hard refresh. Ordering matters:
	// an unmatched /api path must 404 as JSON, never as the SPA shell.
	root := http.NewServeMux()

	// Authentication is required for the API and only for the API. The sign-in
	// routes below must stay reachable to an anonymous caller — that is how a
	// caller becomes authenticated — and the console's static assets carry
	// nothing worth protecting.
	if identity != nil {
		root.Handle("/api/", apiProblems(identity.auth.Middleware(apiHandler)))
		root.Handle("GET /api/v1/me", identity.auth.Middleware(http.HandlerFunc(httpapi.MeHandler)))
		// Liveness is reachable without a credential. A probe cannot hold one,
		// and a health check that answers 401 reads as a dead pod to every
		// orchestrator — the endpoint reports status and version, nothing a
		// caller could not learn by connecting.
		root.Handle("GET /api/v1/healthz", apiProblems(apiHandler))
		identity.routes.Mount(root)

		// Webhooks are outside the session middleware on purpose: the caller
		// is an ERP or a CRM, not a person with a browser. They are
		// authenticated by a secret an operator generated, and a path with no
		// secret answers exactly like a path that does not exist.
		httpapi.NewHooks(
			trigger.NewPostgresWebhooks(identity.pool),
			trigger.NewOpener(store, spec.NewRegistry(identity.pool), engine.SystemClock{}).
				WithContent(ledger.NewContent(identity.pool)),
			slog.Default(),
		).Mount(root)
	} else {
		slog.Warn("running without authentication; every caller has full access")
		root.Handle("/api/", apiProblems(apiHandler))
		// The console asks who it should let in before it renders anything.
		// Without this it gets the SPA's own index.html back, fails to read it
		// as JSON, and shows an error instead of the console — an installation
		// with nothing to protect would look broken rather than open.
		root.Handle("GET /auth/providers", http.HandlerFunc(httpapi.OpenInstallation))
	}

	root.Handle("/", web.Handler())

	srv := &http.Server{
		Addr:              *addr,
		Handler:           withRequestLog(root),
		ReadHeaderTimeout: 10 * time.Second,
		// No write timeout: the run event stream is long-lived by design.
		IdleTimeout: 120 * time.Second,
	}

	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", *addr, "version", version, "console_embedded", web.Embedded())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-sigCtx.Done():
		slog.Info("shutting down")
	}

	// In-flight requests finish; runs survive regardless because their state
	// is in the ledger, not in this process (PRD DE-15).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// Store is what the server and the seeder both need: the ledger plus listing.
type Store interface {
	httpapi.Store
	engine.Ledger
	Verify(ctx context.Context, runID domain.RunID) error
}

// openStore returns the Postgres ledger when a DSN is configured and the
// in-memory one otherwise. Development gets a zero-setup server; an
// installation gets durability. Nothing above this call knows the difference.
func openStore(ctx context.Context, dsn string) (Store, error) {
	if dsn == "" {
		slog.Warn("no --dsn given; using the in-memory ledger, which is lost on restart")
		return ledger.NewMemory(), nil
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := ledger.Migrate(ctx, pool); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	slog.Info("connected to postgres")
	return ledger.NewPostgres(pool), nil
}

// migrate applies pending migrations and exits, for installations that run
// schema changes as a separate step before rolling the server.
func migrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return errors.New("migrate requires --dsn or DATABASE_URL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	if err := ledger.Migrate(ctx, pool); err != nil {
		return err
	}
	slog.Info("migrations applied")
	return nil
}

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
	if pool, err := contentPool(ctx, *dsn); err != nil {
		return err
	} else if pool != nil {
		content = ledger.NewContent(pool)
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
		curator = admin.NewCurator(pool)
		integrations = admin.NewIntegrations(pool, store)
		registry = spec.NewRegistry(pool)
		budgets = admin.NewBudgets(pool, store)

		configured, err := integrations.MCPServers(ctx)
		if err != nil {
			return fmt.Errorf("read configured MCP servers: %w", err)
		}
		for _, server := range configured {
			if !server.Enabled {
				continue
			}
			servers = append(servers, server.Name+"="+
				strings.Join(append([]string{server.Command}, server.Args...), " "))
		}
	}

	if err := servers.connect(ctx, catalog, healthOf(configPool)); err != nil {
		return err
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
	}, queue, deps, specs, engine.SystemClock{}, slog.Default())

	if budgets != nil {
		if spender, ok := store.(spendReader); ok {
			w = w.WithCeilings(ceilings{Budgets: budgets, spend: spender})
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
				WithContent(ledger.NewContent(configPool)),
			engine.SystemClock{}, slog.Default(),
		)
		go runScheduler(ctx, scheduler)
	}

	slog.Info("worker started", "owner", *owner, "concurrency", *concurrency)
	err = w.Run(ctx)
	if errors.Is(err, context.Canceled) {
		slog.Info("worker stopped")
		return nil
	}
	return err
}

// policyRefresh bounds how long an authored policy takes to reach a running
// worker. Short enough that turning a rule on is a live control rather than a
// deploy; long enough that a pool of workers is not a load generator.
const policyRefresh = 30 * time.Second

// policySource is where the set comes from, or a source of nothing when this
// worker has no database. An installation running on the in-memory ledger
// decides under the built-in ladder, which is the safe default rather than an
// absence of rules.
func policySource(pool *pgxpool.Pool) policy.Source {
	if pool == nil {
		return emptyPolicies{}
	}
	return policy.NewStore(pool)
}

type emptyPolicies struct{}

func (emptyPolicies) Active(context.Context) (policy.Set, error) {
	return policy.Set{Hash: "builtin", Policies: nil}, nil
}

// rulingRefresh bounds how long a classification change takes to reach a
// running worker. Short enough that demoting a tool is a live control rather
// than a deploy, long enough that a pool of workers is not a load generator.
const rulingRefresh = 30 * time.Second

func syncRulings(ctx context.Context, catalog *tools.Catalog, curator *admin.Curator) error {
	applied, err := catalog.Sync(ctx, curator, domain.Scope{})
	if err != nil {
		// Starting with every tool silently demoted to read-only would look
		// like a working worker while every write agent quietly stalls.
		return fmt.Errorf("apply tool classifications: %w", err)
	}
	slog.Info("tool classifications applied", "count", applied)
	return nil
}

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

// syncSchedules reconciles the trigger table with what the newest published
// version of each agent declares.
func syncSchedules(ctx context.Context, pool *pgxpool.Pool, specDir *string) error {
	loaded := spec.NewStore()
	if _, err := loaded.LoadDir(ctx, os.DirFS("."), *specDir); err != nil {
		return fmt.Errorf("sync schedules: load %s: %w", *specDir, err)
	}

	schedules := trigger.NewPostgresSchedules(pool)
	now := time.Now()
	for _, agent := range loaded.Agents() {
		versions := loaded.Versions(agent)
		published, err := loaded.Get(agent, versions[len(versions)-1])
		if err != nil {
			return fmt.Errorf("sync schedules: %w", err)
		}
		if err := schedules.Sync(ctx, agent, cronSchedulesOf(published), now); err != nil {
			return fmt.Errorf("sync schedules for %s: %w", agent, err)
		}
	}
	return nil
}

// syncWebhooks reconciles the declared paths with what each agent's newest
// version says. Secrets are untouched: publishing a new version must not break
// every sender configured against a path, because editing a prompt is not a
// security event.
func syncWebhooks(ctx context.Context, pool *pgxpool.Pool, specDir *string) error {
	loaded := spec.NewStore()
	if _, err := loaded.LoadDir(ctx, os.DirFS("."), *specDir); err != nil {
		return fmt.Errorf("sync webhooks: load %s: %w", *specDir, err)
	}

	hooks := trigger.NewPostgresWebhooks(pool)
	for _, agent := range loaded.Agents() {
		versions := loaded.Versions(agent)
		published, err := loaded.Get(agent, versions[len(versions)-1])
		if err != nil {
			return fmt.Errorf("sync webhooks: %w", err)
		}
		scope := domain.Scope{Area: domain.AreaID(published.Area)}
		if err := hooks.Sync(ctx, agent, scope, webhookPathsOf(published)); err != nil {
			// A path two agents both declare is a configuration error, and one
			// of them keeps it. Loud but not fatal: refusing to start would
			// take the whole installation down over one file, and the path
			// that already works keeps working.
			if errors.Is(err, trigger.ErrPathTaken) {
				slog.Error("webhook path already belongs to another agent; this one will not fire",
					"agent", agent, "err", err)
				continue
			}
			return fmt.Errorf("sync webhooks for %s: %w", agent, err)
		}
	}
	return nil
}

// webhookPathsOf picks the paths out of a specification's triggers.
func webhookPathsOf(s spec.Spec) []string {
	out := []string{}
	for _, t := range s.Triggers {
		if t.Type == "webhook" && t.Path != "" {
			out = append(out, strings.TrimPrefix(t.Path, "/"))
		}
	}
	return out
}

// cronSchedulesOf picks the schedules out of a specification's triggers.
func cronSchedulesOf(s spec.Spec) []string {
	out := []string{}
	for _, t := range s.Triggers {
		if t.Type == "cron" && t.Schedule != "" {
			out = append(out, t.Schedule)
		}
	}
	return out
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

// contentPool opens the connection the durable claim check needs, or reports
// that there is none.
func contentPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		slog.Warn("no --dsn given; step content is held in memory and a restart loses runs in flight")
		return nil, nil
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect for step content: %w", err)
	}
	return pool, nil
}

// spendReader is the part of a ledger the ceilings need.
type spendReader interface {
	SpentSince(ctx context.Context, scope domain.Scope, since time.Time) (domain.Consumption, error)
}

// ceilings joins what somebody configured with what has been spent.
//
// The two live apart on purpose — a ceiling is administration, spend is the
// ledger — and this is the one place they meet, which is the worker deciding
// how much room a run has left.
type ceilings struct {
	*admin.Budgets
	spend spendReader
}

func (c ceilings) SpentSince(ctx context.Context, scope domain.Scope, since time.Time) (domain.Consumption, error) {
	return c.spend.SpentSince(ctx, scope, since)
}

// mcpServers collects --mcp flags.
//
// Provisional on purpose. The installation's real source of MCP servers is the
// settings store, written from the administration area; until that exists, a
// flag is how an operator points the worker at one. Both feed the same
// AddServer call, so the flag is a front end and not a second mechanism.
type mcpServers []string

func (m *mcpServers) String() string { return strings.Join(*m, ",") }

func (m *mcpServers) Set(v string) error {
	name, command, ok := strings.Cut(v, "=")
	if !ok || name == "" || command == "" {
		return fmt.Errorf("want name=command, got %q", v)
	}
	*m = append(*m, v)
	return nil
}

// connect starts each server and imports its tools.
//
// Every tool arrives classified read-only whatever the server says about
// itself. Promoting one is the Curator's separate act.
//
// A server that will not answer does not stop the worker. It used to: the
// first failure returned, and one broken integration meant nothing on the
// installation ran — including every agent that never touches it. Now the
// failure is recorded and the worker starts; agents needing that server get a
// clean capability refusal, which is a diagnosable outcome, and the console
// says which server is down and why.
func (m mcpServers) connect(ctx context.Context, catalog *tools.Catalog, health healthRecorder) error {
	for _, entry := range m {
		name, command, _ := strings.Cut(entry, "=")

		fields := strings.Fields(command)
		cmd := exec.CommandContext(ctx, fields[0], fields[1:]...)
		cmd.Stderr = os.Stderr

		client := mcp.NewClient(&mcp.Implementation{Name: "fuseone-agents", Version: version}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
		if err != nil {
			slog.Error("mcp server did not answer; its tools are unavailable",
				"server", name, "command", fields[0], "err", err)
			observe(ctx, health, name, false, 0, err.Error())
			continue
		}
		if err := catalog.AddServer(ctx, name, session); err != nil {
			slog.Error("mcp server answered but its tools could not be imported",
				"server", name, "err", err)
			observe(ctx, health, name, false, 0, err.Error())
			continue
		}
		slog.Info("mcp server connected", "server", name, "command", fields[0])
		observe(ctx, health, name, true, catalog.CountFrom(name), "")
	}
	return nil
}

// healthOf returns where observations are written, or nil when this worker has
// no database to write them to.
func healthOf(pool *pgxpool.Pool) healthRecorder {
	if pool == nil {
		return nil
	}
	return admin.NewHealth(pool)
}

// healthRecorder is what remembers an observation. Optional: a worker running
// without a database still connects, it just has nowhere to write down what it
// saw.
type healthRecorder interface {
	Record(ctx context.Context, obs domain.IntegrationHealth) error
}

// observe records an attempt, and never fails the caller over it. Losing the
// note is worse than nothing; failing the connection because the note could not
// be written would be absurd.
func observe(ctx context.Context, health healthRecorder, name string, ok bool, tools int, detail string) {
	if health == nil {
		return
	}
	if err := health.Record(ctx, domain.IntegrationHealth{
		Name: name, Reachable: ok, ToolCount: tools, Detail: detail,
		ObservedAt: time.Now(), ObservedBy: hostname(),
	}); err != nil {
		slog.Warn("could not record what an integration answered", "server", name, "err", err)
	}
}

// hostname names which worker made an observation. Several connect to the same
// servers and can disagree — one pod on a network that reaches it, one not.
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

// publishSpecs records every definition on disk as a published version.
//
// The worker is where definitions are read today; the Studio will write them
// directly later (PRD DE-07). Either way the registry is what the rest of the
// installation reads, so the two never disagree about what is published.
func publishSpecs(ctx context.Context, registry *spec.Registry, dir string) (int, error) {
	store := spec.NewStore()
	if _, err := store.LoadDir(ctx, os.DirFS("."), dir); err != nil {
		return 0, fmt.Errorf("load agent definitions from %s: %w", dir, err)
	}

	published := 0
	for _, agent := range store.Agents() {
		for _, version := range store.Versions(agent) {
			s, err := store.Get(agent, version)
			if err != nil {
				return published, err
			}
			// The company is the installation's single one until phase 2
			// (PRD §3.1); the area comes from the definition itself.
			if err := registry.Publish(ctx, s, "worker", auth.BootstrapScope.Company); err != nil {
				return published, err
			}
			published++
		}
	}
	return published, nil
}

// loadSpecs publishes the agent definitions on disk and wires the resolver to
// the configured model providers and tool catalogue.
//
// Providers come from the installation's configuration; credentials come from
// the environment rather than the definition, so an agent file is safe to
// commit to a repository.
func loadSpecs(ctx context.Context, dir string, catalog *tools.Catalog, integrations *admin.Integrations) (worker.Specs, error) {
	store := spec.NewStore()
	loaded, err := store.LoadDir(ctx, os.DirFS("."), dir)
	if err != nil {
		return nil, fmt.Errorf("load agent definitions from %s: %w", dir, err)
	}
	slog.Info("loaded agent definitions", "count", loaded, "dir", dir)

	providers := model.NewRegistry(nil)
	if err := registerConfigured(ctx, providers, integrations); err != nil {
		return nil, err
	}
	registerFromEnv(providers)

	if len(providers.Names()) == 0 {
		slog.Warn("no model provider configured; add one in the administration area")
	}

	return spec.NewResolver(store, providers, catalog), nil
}

// registerConfigured takes providers from the administration area, credential
// and all. This is where a key leaves the vault, and the only place it does.
func registerConfigured(ctx context.Context, registry *model.Registry, integrations *admin.Integrations) error {
	if integrations == nil {
		return nil
	}

	configured, err := integrations.Providers(ctx)
	if err != nil {
		return fmt.Errorf("read configured providers: %w", err)
	}

	for _, p := range configured {
		if !p.Enabled {
			continue
		}
		provider := model.Provider{
			Name: p.Name, Kind: model.Kind(p.Kind), BaseURL: p.BaseURL,
		}
		// A preset fills in the quirks — which optional fields the endpoint
		// tolerates, whether it reports cached tokens — that a base URL alone
		// cannot express.
		if preset, ok := model.Preset(p.Name); ok {
			preset.BaseURL, preset.Kind = p.BaseURL, provider.Kind
			provider = preset
		}
		if p.HasKey {
			key, err := integrations.Credential(ctx, p.Name)
			if err != nil {
				return fmt.Errorf("open credential for %s: %w", p.Name, err)
			}
			provider.APIKey = key
		}
		if err := registry.Register(provider); err != nil {
			return err
		}
		slog.Info("provider configured", "provider", p.Name, "source", "administration")
	}
	return nil
}

// registerFromEnv keeps the environment working for an installation that has
// no administrator yet, and for local development. A provider already
// configured in the administration area wins: configuration somebody can audit
// outranks configuration nobody can see.
func registerFromEnv(registry *model.Registry) {
	existing := make(map[string]struct{}, len(registry.Names()))
	for _, name := range registry.Names() {
		existing[name] = struct{}{}
	}

	for _, name := range model.PresetNames() {
		key := os.Getenv(envKeyFor(name))
		if key == "" {
			continue
		}
		if _, taken := existing[name]; taken {
			slog.Info("ignoring environment credential; the administration area configures this provider",
				"provider", name)
			continue
		}
		p, _ := model.Preset(name)
		p.APIKey = key
		if base := os.Getenv(envBaseFor(name)); base != "" {
			p.BaseURL = base
		}
		if err := registry.Register(p); err != nil {
			slog.Warn("could not register provider from environment", "provider", name, "err", err)
			continue
		}
		slog.Info("provider configured", "provider", name, "source", "environment")
	}
}

// keygen prints a master key.
//
// It goes to stdout and nowhere else: the key is never stored by the platform,
// because a platform that can read its own credentials at rest offers an
// attacker with database access nothing to break.
func keygen() error {
	key, err := vault.GenerateKey()
	if err != nil {
		return err
	}
	fmt.Println(key)
	fmt.Fprintf(os.Stderr,
		"\nSet this as %s wherever agentd runs. Losing it means every stored\n"+
			"credential has to be entered again; leaking it means they are all readable.\n",
		vault.KeyEnv)
	return nil
}

// openVault reads the master key. Configuration with a credential in it is
// unreadable without one, so a worker that needs providers needs this.
func openVault() (*vault.Vault, error) {
	encoded := os.Getenv(vault.KeyEnv)
	if encoded == "" {
		return nil, fmt.Errorf("the administration area seals credentials; set %s (agentd version prints how)", vault.KeyEnv)
	}
	// The key id travels with the ciphertext so a future rotation can tell
	// which key sealed a given row.
	v, err := vault.FromBase64(encoded, "primary")
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", vault.KeyEnv, err)
	}
	return v, nil
}

// envKeyFor names the variable holding a provider's credential, e.g.
// ANTHROPIC_API_KEY, DEEPSEEK_API_KEY.
func envKeyFor(provider string) string {
	return strings.ToUpper(provider) + "_API_KEY"
}

// envBaseFor overrides a provider's endpoint — required for the self-hosted
// presets, which ship without one.
func envBaseFor(provider string) string {
	return strings.ToUpper(provider) + "_BASE_URL"
}

// defaultOwner prefers the pod name, so an expired lease is traceable to the
// machine that dropped it.
func defaultOwner() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "agentd"
}

// identity bundles what it takes to authenticate a caller.
type identity struct {
	auth   *auth.Authenticator
	routes *httpapi.AuthRoutes
	// pool is shared with the administration area: rulings, their trail and
	// the session store are one database, and opening a second connection
	// pool to it would only make that less obvious.
	pool *pgxpool.Pool
}

// openIdentity wires authentication, or reports that there is none.
//
// The in-memory ledger has nowhere to keep sessions, so a development server
// started without a database runs open and says so loudly. An installation
// always has Postgres, so it always has authentication.
func openIdentity(ctx context.Context, dsn, baseURL string) (*identity, error) {
	if dsn == "" {
		return nil, nil
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect for identity: %w", err)
	}

	dir := auth.NewPostgres(pool)
	boot := auth.NewBootstrap(pool, dir)

	// A fresh installation cannot configure an identity provider, because
	// doing so needs a permission only an identity provider can grant. The
	// setup token breaks that deadlock exactly once.
	if secret, issued, err := boot.Issue(ctx, 24*time.Hour); err != nil {
		if !errors.Is(err, auth.ErrBootstrapClosed) {
			return nil, err
		}
	} else if issued {
		slog.Warn("SETUP REQUIRED — claim this installation within 24 hours",
			"token", secret, "url", strings.TrimSuffix(baseURL, "/")+"/setup")
	} else {
		slog.Warn("setup is pending; a token was already issued. " +
			"Run `agentd bootstrap --dsn ... --reissue` if it was lost")
	}

	secure := strings.HasPrefix(baseURL, "https://")
	oidc := auth.NewOIDC(baseURL, secure)

	return &identity{
		auth:   auth.NewAuthenticator(dir, secure, nil),
		routes: httpapi.NewAuthRoutes(oidc, dir, boot, secure),
		pool:   pool,
	}, nil
}

// bootstrapCmd reissues the first-run token for an operator who lost it.
//
// It requires database access, which is a reasonable stand-in for authority on
// an installation that does not have any yet.
func bootstrapCmd(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL connection string")
	reissue := fs.Bool("reissue", false, "replace the existing setup token")
	reopen := fs.String("reopen", "",
		"reopen a claimed installation so another administrator can be created; the value is the reason, and it is recorded")
	supplied := fs.String("token", "", "use this token instead of a generated one, for provisioning")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return errors.New("bootstrap requires --dsn or DATABASE_URL")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	boot := auth.NewBootstrap(pool, auth.NewPostgres(pool))

	// Reopening is the way back into an installation whose only administrator
	// can no longer reach it: a lost session, a departed colleague, an
	// identity provider that broke. Configuring a provider needs Curator and
	// the only Curator is unreachable, so without this the installation is
	// lost for good — on-premise, with nobody to call.
	if *reopen != "" {
		secret, err := boot.Reopen(ctx, 24*time.Hour, *reopen)
		if err != nil {
			return err
		}
		slog.Warn("installation reopened; the setup screen accepts this token once",
			"reason", *reopen)
		fmt.Println(secret)
		return nil
	}

	pending, err := boot.Pending(ctx)
	if err != nil {
		return err
	}
	if !pending {
		return fmt.Errorf("%w — pass --reopen with a reason to let another administrator be created",
			auth.ErrBootstrapClosed)
	}
	if !*reissue {
		fmt.Println("setup is still pending; pass --reissue to mint a replacement token")
		return nil
	}

	// A supplied token exists for provisioning: a chart or a script that has
	// to know the value before the process starts. It is still single use and
	// the endpoint still closes for good once claimed, so the exposure is the
	// window before somebody claims — which is exactly the window a generated
	// token printed to a log has too.
	if *supplied != "" {
		if len(*supplied) < 24 {
			slog.Warn("the supplied setup token is short enough to guess", "length", len(*supplied))
		}
		if err := boot.Adopt(ctx, *supplied, 24*time.Hour); err != nil {
			return err
		}
		fmt.Println(*supplied)
		return nil
	}

	secret, err := boot.Reissue(ctx, 24*time.Hour)
	if err != nil {
		return err
	}
	fmt.Println(secret)
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// apiProblems keeps every response under /api/ in the contract's error shape.
//
// The generated router answers an unrouted path with net/http's plain-text
// 404, which a client parsing application/problem+json cannot read. Anything
// the contract does not describe still has to look like the contract.
func apiProblems(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &problemRecorder{ResponseWriter: w, path: r.URL.Path}
		next.ServeHTTP(rec, r)
	})
}

type problemRecorder struct {
	http.ResponseWriter
	path      string
	swallowed bool
}

func (p *problemRecorder) WriteHeader(code int) {
	// net/http's NotFound sets text/plain before calling WriteHeader, so the
	// test is "did something other than the contract answer this", not
	// "is the content type unset".
	if code == http.StatusNotFound && !strings.Contains(p.Header().Get("Content-Type"), "json") {
		p.swallowed = true
		writeProblem(p.ResponseWriter, http.StatusNotFound, "Unknown endpoint",
			"No operation is defined for "+p.path)
		return
	}
	p.ResponseWriter.WriteHeader(code)
}

func (p *problemRecorder) Write(b []byte) (int, error) {
	if p.swallowed {
		// The problem body is already written; drop the router's plain text.
		return len(b), nil
	}
	return p.ResponseWriter.Write(b)
}

func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method, "path", r.URL.Path,
			"status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"title":%q,"status":%d,"detail":%q}`, title, status, detail)
}
