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

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/httpapi"
)

// Who may sign in, and the one-time claim of a fresh installation.

// identity bundles what it takes to authenticate a caller.
type identity struct {
	// oidc is the live registry the sign-in routes read from and the
	// administration area writes to.
	oidc   *auth.OIDC
	auth   *auth.Authenticator
	routes *httpapi.AuthRoutes
	// dir is the same directory the sign-in path reads, kept because the
	// channel hook has to turn a bound account into a person with grants —
	// and it must be the one directory, or a disabled principal would stop
	// being able to sign in and go on approving through a conversation.
	dir *auth.Postgres
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
		auth: auth.NewAuthenticator(dir, secure, nil),
		// Accounts that sign in with a password, beside the provider and
		// never instead of it. Wired unconditionally: an installation with no
		// local account shows no password form, and that is a question the
		// route answers from the data rather than from how it was built.
		routes: httpapi.NewAuthRoutes(oidc, dir, boot, secure).
			WithLocal(auth.NewLocal(pool, dir)),
		pool: pool,
		oidc: oidc,
		dir:  dir,
	}, nil
}

// registerProviders puts the configured identity providers into the live
// registry at start-up.
//
// One that cannot be discovered is logged and skipped rather than fatal: a
// provider whose issuer is down must not stop the console from serving, or the
// only way to fix a broken sign-in would be to fix the thing that broke first.
func registerProviders(ctx context.Context, store *admin.Identity, live *auth.OIDC) {
	providers, err := store.IdentityProviders(ctx)
	if err != nil {
		slog.Error("could not read the identity providers", "err", err)
		return
	}
	for _, p := range providers {
		if !p.Enabled {
			continue
		}
		secret, err := store.IdentitySecret(ctx, p.ID)
		if err != nil {
			slog.Error("identity provider has no readable secret", "provider", p.ID, "err", err)
			continue
		}
		if err := live.Add(ctx, &auth.OIDCProvider{
			ID: p.ID, Display: p.Display, Issuer: p.Issuer, ClientID: p.ClientID,
			ClientSecret: secret, GroupsClaim: p.GroupsClaim, Mappings: p.Mappings,
		}); err != nil {
			slog.Error("identity provider did not answer; nobody can sign in with it",
				"provider", p.ID, "issuer", p.Issuer, "err", err)
			continue
		}
		slog.Info("identity provider configured", "provider", p.ID, "mappings", len(p.Mappings))
	}
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
