package trigger_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

func webhooksFor(t *testing.T) *trigger.PostgresWebhooks {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the webhook suite")
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := ledger.Migrate(t.Context(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `truncate webhook_triggers`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return trigger.NewPostgresWebhooks(pool)
}

var cx = domain.Scope{Company: "acme", Area: "cx"}

func TestWebhook_declaredButNeverRotated_isClosed(t *testing.T) {
	hooks := webhooksFor(t)

	if err := hooks.Sync(t.Context(), "triage", cx, []string{"crm/ticket"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// A path that answered anything until somebody configured it would be an
	// open door with an agent behind it.
	if _, err := hooks.Verify(t.Context(), "crm/ticket", "guess"); !errors.Is(err, trigger.ErrNotArmed) {
		t.Fatalf("Verify on an unarmed hook = %v, want ErrNotArmed", err)
	}
	hook, err := hooks.Find(t.Context(), "crm/ticket")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if hook.Armed {
		t.Error("a hook nobody rotated reports itself armed")
	}
}

func TestWebhook_afterRotating_acceptsOnlyThatSecret(t *testing.T) {
	hooks := webhooksFor(t)
	if err := hooks.Sync(t.Context(), "triage", cx, []string{"crm/ticket"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	secret, err := hooks.Rotate(t.Context(), "crm/ticket", "usr_ana", time.Now())
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	ok, err := hooks.Verify(t.Context(), "crm/ticket", secret)
	if err != nil || !ok {
		t.Fatalf("Verify with the new secret = %v (%v), want true", ok, err)
	}
	if ok, _ := hooks.Verify(t.Context(), "crm/ticket", secret+"x"); ok {
		t.Error("a wrong secret was accepted")
	}
}

func TestWebhook_rotating_retiresThePreviousSecret(t *testing.T) {
	hooks := webhooksFor(t)
	if err := hooks.Sync(t.Context(), "triage", cx, []string{"crm/ticket"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	first, _ := hooks.Rotate(t.Context(), "crm/ticket", "usr_ana", time.Now())
	second, _ := hooks.Rotate(t.Context(), "crm/ticket", "usr_ana", time.Now())

	// Rotating is what somebody does after a secret leaks. If the old one
	// still worked it would not be a rotation.
	if ok, _ := hooks.Verify(t.Context(), "crm/ticket", first); ok {
		t.Error("the retired secret still works")
	}
	if ok, _ := hooks.Verify(t.Context(), "crm/ticket", second); !ok {
		t.Error("the new secret does not work")
	}
}

func TestWebhook_republishing_keepsTheSecret(t *testing.T) {
	hooks := webhooksFor(t)
	if err := hooks.Sync(t.Context(), "triage", cx, []string{"crm/ticket"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	secret, _ := hooks.Rotate(t.Context(), "crm/ticket", "usr_ana", time.Now())

	// Publishing a new version must not break every sender configured against
	// this path. Editing a prompt is not a security event.
	if err := hooks.Sync(t.Context(), "triage", cx, []string{"crm/ticket"}); err != nil {
		t.Fatalf("Sync again: %v", err)
	}
	if ok, _ := hooks.Verify(t.Context(), "crm/ticket", secret); !ok {
		t.Error("republishing invalidated the secret")
	}
}

func TestWebhook_pathNoLongerDeclared_stopsAnswering(t *testing.T) {
	hooks := webhooksFor(t)
	if err := hooks.Sync(t.Context(), "triage", cx, []string{"crm/ticket"}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := hooks.Rotate(t.Context(), "crm/ticket", "usr_ana", time.Now()); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	if err := hooks.Sync(t.Context(), "triage", cx, nil); err != nil {
		t.Fatalf("Sync without paths: %v", err)
	}
	if _, err := hooks.Find(t.Context(), "crm/ticket"); !errors.Is(err, trigger.ErrNoHook) {
		t.Errorf("Find after withdrawal = %v, want ErrNoHook", err)
	}
}

func TestRotate_pathNobodyDeclared_isRefused(t *testing.T) {
	hooks := webhooksFor(t)

	if _, err := hooks.Rotate(t.Context(), "made/up", "usr_ana", time.Now()); !errors.Is(err, trigger.ErrNoHook) {
		t.Errorf("Rotate on an undeclared path = %v, want ErrNoHook", err)
	}
}
