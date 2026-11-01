package trigger_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/vault"
)

func ordinary(paths ...string) []domain.WebhookTrigger {
	out := []domain.WebhookTrigger{}
	for _, p := range paths {
		out = append(out, domain.WebhookTrigger{Path: p})
	}
	return out
}

func fromGitHub(path string, events ...string) []domain.WebhookTrigger {
	return []domain.WebhookTrigger{{Path: path, From: domain.WebhookFromGitHub, Events: events}}
}

func sealing(t *testing.T) *vault.Vault {
	t.Helper()
	v, err := vault.New(bytes.Repeat([]byte{7}, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	return v
}

func githubHooks(t *testing.T) *trigger.PostgresWebhooks {
	t.Helper()
	return webhooksFor(t).WithVault(sealing(t))
}

func TestGitHubWebhook_declared_carriesItsEventsAndIsClosedUntilRotated(t *testing.T) {
	hooks := githubHooks(t)
	if err := hooks.Sync(t.Context(), "reviewer", cx, fromGitHub("gh/reviewer", "pull_request.opened")); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	hook, err := hooks.Find(t.Context(), "gh/reviewer")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if hook.From != domain.WebhookFromGitHub || !reflect.DeepEqual(hook.Events, []string{"pull_request.opened"}) {
		t.Errorf("hook = %+v, want it from github with its event", hook)
	}
	if hook.Armed {
		t.Error("a GitHub hook nobody rotated reports itself armed")
	}
	if _, err := hooks.SigningSecret(t.Context(), "gh/reviewer"); !errors.Is(err, trigger.ErrNotArmed) {
		t.Errorf("SigningSecret before rotating = %v, want ErrNotArmed", err)
	}
}

// GitHub signs with the secret, so the door has to read it back: what Rotate
// handed the operator once is what SigningSecret opens.
func TestGitHubWebhook_rotated_signsWithTheSecretItHandedOut(t *testing.T) {
	hooks := githubHooks(t)
	if err := hooks.Sync(t.Context(), "reviewer", cx, fromGitHub("gh/reviewer", "pull_request")); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	secret, err := rotateAs(t, hooks, "reviewer", "gh/reviewer")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	got, err := hooks.SigningSecret(t.Context(), "gh/reviewer")
	if err != nil || string(got) != secret {
		t.Fatalf("SigningSecret = %q (%v), want the rotated secret", got, err)
	}
	if hook, _ := hooks.Find(t.Context(), "gh/reviewer"); !hook.Armed {
		t.Error("a rotated GitHub hook reports itself closed")
	}
}

// Without a vault a GitHub secret could only be stored in the clear, so it is
// not stored at all and the hook stays closed.
func TestGitHubWebhook_rotatedWithNoVault_isRefusedAndStaysClosed(t *testing.T) {
	hooks := webhooksFor(t)
	if err := hooks.Sync(t.Context(), "reviewer", cx, fromGitHub("gh/reviewer", "pull_request")); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if _, err := rotateAs(t, hooks, "reviewer", "gh/reviewer"); !errors.Is(err, trigger.ErrNoVault) {
		t.Fatalf("Rotate with no vault = %v, want ErrNoVault", err)
	}
	if hook, _ := hooks.Find(t.Context(), "gh/reviewer"); hook.Armed {
		t.Error("a GitHub hook rotated with no vault reports itself armed")
	}
}

// The GitHub secret signs; it is never a secret somebody sends. Accepted in
// X-FuseOne-Secret it would let whoever holds it skip the signature.
func TestGitHubWebhook_secret_isNeverAcceptedAsASentSecret(t *testing.T) {
	hooks := githubHooks(t)
	if err := hooks.Sync(t.Context(), "reviewer", cx, fromGitHub("gh/reviewer", "pull_request")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	secret, err := rotateAs(t, hooks, "reviewer", "gh/reviewer")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	if ok, _ := hooks.Verify(t.Context(), "gh/reviewer", secret); ok {
		t.Error("a GitHub hook's secret was accepted as a sent secret")
	}
}

/*
A path whose source changes loses its secret.

The holder of an ordinary hook's secret sends it; the holder of a GitHub one
signs with it. Kept across the change, a secret handed to one kind of sender
would quietly start working the other way.
*/
func TestSync_aPathChangingItsSource_closesIt(t *testing.T) {
	cases := map[string]struct{ before, after []domain.WebhookTrigger }{
		"ordinary to github": {ordinary("hooks/a"), fromGitHub("hooks/a", "push")},
		"github to ordinary": {fromGitHub("hooks/a", "push"), ordinary("hooks/a")},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			hooks := githubHooks(t)
			if err := hooks.Sync(t.Context(), "triage", cx, c.before); err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if _, err := rotateAs(t, hooks, "triage", "hooks/a"); err != nil {
				t.Fatalf("Rotate: %v", err)
			}

			if err := hooks.Sync(t.Context(), "triage", cx, c.after); err != nil {
				t.Fatalf("Sync after: %v", err)
			}
			if hook, _ := hooks.Find(t.Context(), "hooks/a"); hook.Armed {
				t.Error("the path kept its secret across a change of source")
			}
		})
	}
}

// Republished with the same source, the secret stays and the events follow
// the newest version.
func TestSync_aGitHubPathRepublished_keepsItsSecretAndTakesTheNewEvents(t *testing.T) {
	hooks := githubHooks(t)
	if err := hooks.Sync(t.Context(), "reviewer", cx, fromGitHub("gh/reviewer", "pull_request")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	secret, _ := rotateAs(t, hooks, "reviewer", "gh/reviewer")

	if err := hooks.Sync(t.Context(), "reviewer", cx, fromGitHub("gh/reviewer", "pull_request.labeled")); err != nil {
		t.Fatalf("Sync again: %v", err)
	}
	hook, _ := hooks.Find(t.Context(), "gh/reviewer")
	if !reflect.DeepEqual(hook.Events, []string{"pull_request.labeled"}) {
		t.Errorf("events = %v, want the republished ones", hook.Events)
	}
	if got, err := hooks.SigningSecret(t.Context(), "gh/reviewer"); err != nil || string(got) != secret {
		t.Errorf("SigningSecret after republishing = %q (%v), want the same secret", got, err)
	}
}

// A sealed secret is bound to its path: copied onto another row by somebody
// with database access, it does not open there.
func TestGitHubWebhook_sealedSecretCopiedToAnotherPath_doesNotOpen(t *testing.T) {
	pool := poolOf(t)
	hooks := githubHooks(t)
	both := append(fromGitHub("gh/a", "push"), fromGitHub("gh/b", "push")...)
	if err := hooks.Sync(t.Context(), "reviewer", cx, both); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := rotateAs(t, hooks, "reviewer", "gh/a"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		update webhook_triggers b set secret_hash = a.secret_hash,
			secret_sealed = a.secret_sealed, secret_nonce = a.secret_nonce
		from webhook_triggers a where a.path = 'gh/a' and b.path = 'gh/b'`); err != nil {
		t.Fatalf("copy: %v", err)
	}

	if got, err := hooks.SigningSecret(t.Context(), "gh/b"); err == nil {
		t.Errorf("a secret sealed for gh/a opened on gh/b: %q", got)
	}
}

/*
A rotation arms only the hook of the agent that asked for it.

The console checks the path belongs to the agent and then rotates. If the path
changed hands in between — the agent withdrew it and another claimed it — a
rotation by path alone armed the new owner's hook and handed its key to
somebody authorised for the old one.
*/
func TestRotate_aPathThatChangedHands_armsNothingAndHandsOutNoKey(t *testing.T) {
	hooks := githubHooks(t)
	if err := hooks.Sync(t.Context(), "triage", cx, ordinary("crm/ticket")); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// What the console saw: crm/ticket belongs to triage. Then it changes hands.
	if err := hooks.Sync(t.Context(), "triage", cx, nil); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if err := hooks.Sync(t.Context(), "billing", cx, ordinary("crm/ticket")); err != nil {
		t.Fatalf("claim: %v", err)
	}

	_, err := hooks.Rotate(t.Context(), trigger.Rotation{
		Path: "crm/ticket", Agent: "triage", By: "usr_ana", At: time.Now(),
	})
	if !errors.Is(err, trigger.ErrNoHook) {
		t.Fatalf("Rotate for the previous owner = %v, want ErrNoHook", err)
	}
	if hook, _ := hooks.Find(t.Context(), "crm/ticket"); hook.Armed {
		t.Error("the new owner's hook was armed by a rotation it never asked for")
	}
}

// rotateAs rotates a path on behalf of the agent the caller was authorised for.
func rotateAs(t *testing.T, hooks *trigger.PostgresWebhooks, agent domain.AgentID, path string) (string, error) {
	t.Helper()
	return hooks.Rotate(t.Context(), trigger.Rotation{Path: path, Agent: agent, By: "usr_ana", At: time.Now()})
}

func poolOf(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}
