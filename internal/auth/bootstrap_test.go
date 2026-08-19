package auth_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/ledger"
)

// An installation whose only administrator cannot get in — a lost session, a
// broken identity provider, a person who left the company — has no way back.
// Configuring a provider needs Curator; the only Curator is unreachable; and
// the setup token refuses to mint because an administrator exists. That is a
// permanent lockout of an on-premise installation, recoverable only by
// editing the database by hand.

func bootstrapFor(t *testing.T) (*auth.Bootstrap, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the bootstrap suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`truncate role_grants, principals, sessions, settings, admin_events, scopes, companies cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return auth.NewBootstrap(pool, auth.NewPostgres(pool)), pool
}

func claimFirst(t *testing.T, b *auth.Bootstrap) {
	t.Helper()
	secret, issued, err := b.Issue(t.Context(), time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !issued {
		// Already outstanding from an earlier Issue in the same test.
		if secret, err = b.Reissue(t.Context(), time.Hour); err != nil {
			t.Fatalf("Reissue: %v", err)
		}
	}
	if _, _, err := b.Claim(t.Context(), secret, "Ana", "test", "127.0.0.1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
}

func TestClaim_grantsOneInstallationAdmin(t *testing.T) {
	b, _ := bootstrapFor(t)
	secret, issued, err := b.Issue(t.Context(), time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !issued {
		t.Fatal("fresh bootstrap did not issue a token")
	}

	_, principal, err := b.Claim(t.Context(), secret, "Ana", "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if len(principal.Grants) != 1 {
		t.Fatalf("grants = %+v, want one admin grant", principal.Grants)
	}
	if got := principal.Grants[0]; got.Scope.Company != "*" || got.Scope.Area != "" || got.Role != "admin" {
		t.Errorf("grant = %+v, want installation admin", got)
	}
}

func TestIssue_afterTheInstallationIsClaimed_refuses(t *testing.T) {
	b, _ := bootstrapFor(t)
	claimFirst(t, b)

	// The ordinary path stays shut: a token minted on every restart would sit
	// in every log the installation ever writes.
	if _, _, err := b.Issue(t.Context(), time.Hour); !errors.Is(err, auth.ErrBootstrapClosed) {
		t.Fatalf("Issue after claim = %v, want ErrBootstrapClosed", err)
	}
}

func TestReopen_claimedInstallation_mintsAWorkingToken(t *testing.T) {
	b, _ := bootstrapFor(t)
	claimFirst(t, b)

	secret, err := b.Reopen(t.Context(), time.Hour, "the sole administrator lost their session")
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	if _, _, err := b.Claim(t.Context(), secret, "Bruno", "test", "127.0.0.1"); err != nil {
		t.Fatalf("the reopened token was refused: %v", err)
	}
}

func TestReopen_leavesTheReasonInTheAdministrativeTrail(t *testing.T) {
	// Minting an administrator without a trace is worse than a lockout: the
	// installation could not tell afterwards that it had happened.
	b, pool := bootstrapFor(t)
	claimFirst(t, b)

	if _, err := b.Reopen(t.Context(), time.Hour, "provider outage"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	var reason string
	if err := pool.QueryRow(t.Context(),
		`select detail->>'reason' from admin_events where action = 'bootstrap_reopened'`).Scan(&reason); err != nil {
		t.Fatalf("no record of the reopening: %v", err)
	}
	if reason != "provider outage" {
		t.Errorf("recorded reason = %q, want the one given", reason)
	}
}

func TestClaim_withoutALiveToken_refuses(t *testing.T) {
	b, _ := bootstrapFor(t)
	claimFirst(t, b)

	// Claiming burns the token, so a claimed installation has none. The door
	// is shut by default; only an explicit reopen opens it.
	if _, _, err := b.Claim(t.Context(), "anything", "Bruno", "test", "127.0.0.1"); err == nil {
		t.Fatal("a second administrator was created with no live setup token")
	}
}

func TestClaim_reopenedTokenIsStillSingleUse(t *testing.T) {
	b, _ := bootstrapFor(t)
	claimFirst(t, b)

	secret, err := b.Reopen(t.Context(), time.Hour, "lost session")
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if _, _, err := b.Claim(t.Context(), secret, "Bruno", "test", "127.0.0.1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	if _, _, err := b.Claim(t.Context(), secret, "Carla", "test", "127.0.0.1"); err == nil {
		t.Fatal("the same reopened token created a second administrator")
	}
}

func TestOpen_reportsWhetherSomebodyCanStillClaim(t *testing.T) {
	// What the login screen asks. A claimed installation shows the identity
	// provider; a reopened one has to show the setup form again, or the
	// operator holding a fresh token has nowhere to type it.
	b, _ := bootstrapFor(t)
	if _, _, err := b.Issue(t.Context(), time.Hour); err != nil {
		t.Fatalf("Issue: %v", err)
	}

	open, err := b.Open(t.Context())
	if err != nil || !open {
		t.Fatalf("Open with a token outstanding = %v, %v; want true", open, err)
	}

	claimFirst(t, b)
	if open, err = b.Open(t.Context()); err != nil || open {
		t.Fatalf("Open after claim = %v, %v; want false", open, err)
	}

	if _, err := b.Reopen(t.Context(), time.Hour, "lost session"); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if open, err = b.Open(t.Context()); err != nil || !open {
		t.Fatalf("Open after reopen = %v, %v; want true", open, err)
	}
}
