package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
)

/*
Signing in with the administrator's own password.

The path that exists so an installation cannot lock itself out, which means
the two things it must get right are opposites: it has to work when nothing
else does, and it must not become a way in for somebody guessing.
*/

func TestSignIn_theRightPassword_issuesASession(t *testing.T) {
	local, id := localWithPassword(t, "uma senha bem comprida")

	who, err := local.Verify(context.Background(), "ana", "uma senha bem comprida")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if who != id {
		t.Errorf("principal = %q, want %q", who, id)
	}
}

func TestSignIn_theWrongPassword_isRefusedAndCounted(t *testing.T) {
	local, id := localWithPassword(t, "uma senha bem comprida")
	ctx := context.Background()

	if _, err := local.Verify(ctx, "ana", "outra senha comprida"); err == nil {
		t.Fatal("the wrong password was accepted")
	}

	failed, _, err := local.Attempts(ctx, id)
	if err != nil {
		t.Fatalf("Attempts: %v", err)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want the attempt counted", failed)
	}
}

/*
A console reachable from a browser is a password oracle unless something
stops the guessing. Locked after a handful of tries, and for minutes rather
than for ever: an administrator who mistyped their own password must not need
database access to get back in, or the lockout becomes the outage.
*/
func TestSignIn_repeatedlyWrong_locksTheAccountForAWhile(t *testing.T) {
	local, id := localWithPassword(t, "uma senha bem comprida")
	ctx := context.Background()

	for range auth.MaxSignInAttempts {
		_, _ = local.Verify(ctx, "ana", "chute")
	}

	// Even the right one, now. A lock that the correct password opens is not
	// a lock: guessing it is exactly what the attacker is trying to do.
	_, err := local.Verify(ctx, "ana", "uma senha bem comprida")
	if !errors.Is(err, auth.ErrLockedOut) {
		t.Fatalf("err = %v, want it locked", err)
	}

	_, until, err := local.Attempts(ctx, id)
	if err != nil {
		t.Fatalf("Attempts: %v", err)
	}
	if until.IsZero() || until.After(time.Now().Add(auth.LockoutFor+time.Minute)) {
		t.Errorf("locked until %s, want a bounded window", until)
	}
}

func TestSignIn_afterSucceeding_theCountIsForgotten(t *testing.T) {
	local, id := localWithPassword(t, "uma senha bem comprida")
	ctx := context.Background()

	_, _ = local.Verify(ctx, "ana", "chute")
	if _, err := local.Verify(ctx, "ana", "uma senha bem comprida"); err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	// Otherwise a week of ordinary typos adds up to a lockout on a morning
	// nobody did anything wrong.
	failed, _, err := local.Attempts(ctx, id)
	if err != nil {
		t.Fatalf("Attempts: %v", err)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want it reset", failed)
	}
}

// A principal that arrived through a provider has no password, and asking
// about one must not be a way to find out that the account exists.
func TestSignIn_anAccountWithNoPassword_isRefusedTheSameWay(t *testing.T) {
	local, _ := localWithPassword(t, "uma senha bem comprida")

	_, absent := local.Verify(context.Background(), "ninguem", "uma senha bem comprida")
	if absent == nil {
		t.Fatal("an account that does not exist signed in")
	}
	if !errors.Is(absent, auth.ErrBadCredential) {
		t.Errorf("err = %v, want the same refusal a wrong password gives", absent)
	}
}

// --- harness ----------------------------------------------------------------

// localWithPassword claims an installation and gives its administrator a
// password, which is the state every installation reaches after setup.
func localWithPassword(t *testing.T, password string) (*auth.Local, string) {
	t.Helper()
	boot, pool := bootstrapFor(t)

	secret, _, err := boot.Issue(t.Context(), time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	_, principal, err := boot.Claim(t.Context(), secret, "Ana", "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	local := auth.NewLocal(pool, auth.NewPostgres(pool))
	if err := local.SetUsername(t.Context(), string(principal.ID), "ana"); err != nil {
		t.Fatalf("SetUsername: %v", err)
	}
	if err := local.SetPassword(t.Context(), string(principal.ID), password); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	return local, string(principal.ID)
}
