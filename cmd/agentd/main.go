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
	"strings"
	"time"

	"github.com/fuseone/agents/internal/admin"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/drift"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
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
	case "verify":
		return verifyCmd(args[1:])
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
  agentd verify <file>   check a signed ledger export
  agentd version         print the build version
`)
	return errors.New("no command given")
}

// Store is what the server and the seeder both need: the ledger plus listing.
type Store interface {
	httpapi.Store
	httpapi.LastBattery
	drift.Batteries
	drift.LastBatteries
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
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := ledger.Migrate(ctx, pool); err != nil {
		pool.Close()
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

// defaultOwner prefers the pod name, so an expired lease is traceable to the
// machine that dropped it.
func defaultOwner() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "agentd"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// assistants builds the provider registry the authoring assistant reaches
// through. Configured the same way the worker's is, so an authoring call
// cannot drift onto a different credential than a run would use.
//
// The serve process holds no vault, so a provider whose credential is sealed
// cannot be opened here. That is reported when the call is attempted rather
// than at boot: an installation with no assistant configured must still start.
func assistants(ctx context.Context, integrations *admin.Integrations) *model.Registry {
	providers := model.NewRegistry(nil)
	if err := registerConfigured(ctx, providers, integrations); err != nil {
		slog.Warn("could not read configured providers for the authoring assistant", "err", err)
	}
	registerFromEnv(providers)
	if err := refreshConfiguredPrices(ctx, providers, integrations); err != nil {
		slog.Warn("could not read configured prices for the authoring assistant", "err", err)
	}
	go watchConfiguredPrices(ctx, providers, integrations)
	return providers
}
