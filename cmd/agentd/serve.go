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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/connect"
	"github.com/fuseone/agents/internal/settings"
	"github.com/fuseone/agents/internal/vault"

	"github.com/fuseone/agents/internal/admin"

	"github.com/fuseone/agents/internal/audit"
	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/scope"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/web"
)

// The serve command: the HTTP API and the console, and everything they read.

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
	// Held outside the block that builds it, because the channel hook is
	// mounted with the routes rather than with the administration area.
	var channels *admin.Channels
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
		switch {
		case errors.Is(err, vault.ErrNoKey):
			slog.Warn("no master key; credentials cannot be stored from the console until one is set",
				"variable", vault.KeyEnv)
		case err != nil:
			// Set and wrong is not the same as unset. Warning here let a
			// console serve happily beside workers crash-looping on the same
			// value, which is a configuration mistake dressed as a broken
			// image.
			return fmt.Errorf("%s is set but unusable: %w", vault.KeyEnv, err)
		}
		store := settings.NewStore(identity.pool, v)
		// Forgetting health on removal, because this is the process that serves
		// the delete: without it a removed server stays on the screen as one
		// nobody configured, which cannot be edited or removed.
		integrations := admin.NewIntegrations(identity.pool, store).
			ForgettingHealth(admin.NewHealth(identity.pool))
		// Where runs report, and a way to prove the bot was invited without
		// waiting for one to park (NT-005 stage 1).
		drivers := connect.New(store)
		channels = admin.NewChannels(identity.pool, store)
		api = api.WithChannels(channels, channel.NewRouter(drivers)).
			WithChannelListing(drivers).
			WithAdministration(curator, curator, integrations).
			WithAgents(spec.NewRegistry(identity.pool)).
			WithCeilings(admin.NewBudgets(identity.pool, store)).
			// The same store the worker writes into. Without it the console
			// can show that an approval is pending but not what it is for,
			// which is the one thing the approver needs.
			WithContent(ledger.NewContent(identity.pool)).
			// The same store, filing the case sets a simulation is run
			// against. A set is real customer records and belongs under the
			// installation's retention like every other bulky payload (AU-04).
			WithCases(ledger.NewContent(identity.pool)).
			// The corrections a future version is checked against (FU-12).
			WithRegressions(regression.NewStore(identity.pool)).
			// How long content is kept, and erasing it on request. Its own
			// permission: the one act here nobody can undo.
			// The key exports are signed with, and its public half.
			WithSigning(admin.NewSigning(identity.pool, store)).
			WithRetention(
				admin.NewRetention(identity.pool, store),
				admin.NewErasures(identity.pool, ledger.NewContent(identity.pool),
					admin.NewRetention(identity.pool, store)),
			).
			WithWebhooks(trigger.NewPostgresWebhooks(identity.pool)).
			WithAudit(audit.NewPostgres(identity.pool)).
			WithHealth(admin.NewHealth(identity.pool)).
			WithPolicies(policy.NewStore(identity.pool)).
			WithAreas(scope.NewStore(identity.pool)).
			WithRates(integrations).
			WithAuthoring(authoring.NewStore(identity.pool, store)).
			WithAssistants(assistants(ctx, integrations), authoring.NewStore(identity.pool, store)).
			WithPauses(spec.NewState(identity.pool)).
			WithStops(admin.NewStops(identity.pool)).
			WithMarks(admin.NewMarks(identity.pool)).
			WithComposition(spec.NewRegistry(identity.pool)).
			WithReplays(httpapi.NewReplays(
				policy.NewStore(identity.pool).Policies,
				spec.NewRegistry(identity.pool).Pack,
			)).
			WithStages(spec.NewState(identity.pool)).
			WithPromotions(spec.NewState(identity.pool)).
			WithPublisher(spec.NewPublisher(identity.pool, engine.SystemClock{}))

		// Who may sign in, and what signing in grants. Saving registers the
		// provider straight away, so a configuration never needs a restart to
		// take effect — and start-up loads what is already stored.
		identityStore := admin.NewIdentity(identity.pool, store)
		api = api.WithIdentity(identityStore, identity.oidc).
			WithPeople(auth.NewPostgres(identity.pool))
		registerProviders(ctx, identityStore, identity.oidc)
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
		// Both probes are reachable without a credential. A probe cannot hold
		// one, and a health check that answers 401 reads as a dead pod to
		// every orchestrator — they report status and version, nothing a
		// caller could not learn by connecting.
		root.Handle("GET /api/v1/healthz", apiProblems(apiHandler))
		root.Handle("GET /api/v1/readyz", apiProblems(apiHandler))
		identity.routes.Mount(root)

		// What a conversation says back (NT-005 stage 2). Outside /api/v1 and
		// outside the session middleware on purpose: it is a vendor's webhook,
		// it carries no cookie, and what authenticates it is a signature this
		// installation checks rather than anything the API knows about.
		if channels != nil {
			httpapi.NewChannelHooks(api, channels, identity.dir, time.Now, slog.Default()).
				Mount(root)
		}

		// Webhooks are outside the session middleware on purpose: the caller
		// is an ERP or a CRM, not a person with a browser. They are
		// authenticated by a secret an operator generated, and a path with no
		// secret answers exactly like a path that does not exist.
		httpapi.NewHooks(
			trigger.NewPostgresWebhooks(identity.pool),
			trigger.NewOpener(store, spec.NewRegistry(identity.pool), engine.SystemClock{}).
				WithContent(ledger.NewContent(identity.pool)).
				WithPauses(spec.NewState(identity.pool)).
				// A webhook is a way a run starts, so the switches reach it
				// too. One honoured by the console and not by an inbound hook
				// is a stop that quietens the half nobody is watching.
				WithStops(admin.NewStops(identity.pool)).
				WithStages(spec.NewState(identity.pool)),
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
