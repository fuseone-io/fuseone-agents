package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/trigger"
	"github.com/fuseone/agents/internal/vault"
)

/*
A GitHub hook stored with a hash and no sealed secret refuses every delivery.

A legacy row, or one edited by hand, looks like this. A hash cannot check a
signature, so the real store, behind the real door, has to answer exactly as
it answers a wrong signature — not fall back to the hash, not open a run.
*/
func TestGitHubHook_aRowWithAHashButNoSealedSecret_refusesLikeAnyUnprovenDelivery(t *testing.T) {
	pool := testdb.Pool(t)
	if _, err := pool.Exec(t.Context(), `truncate webhook_triggers`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	v, err := vault.New(bytes.Repeat([]byte{9}, 32), "test")
	if err != nil {
		t.Fatalf("vault: %v", err)
	}
	hooks := trigger.NewPostgresWebhooks(pool).WithVault(v)
	declared := []domain.WebhookTrigger{{Path: "gh/legacy", From: domain.WebhookFromGitHub, Events: []string{"pull_request"}}}
	if err := hooks.Sync(t.Context(), "triage", domain.Scope{Company: "acme", Area: "cx"}, declared); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	secret, hash, _ := trigger.NewSecret()
	if _, err := pool.Exec(t.Context(),
		`update webhook_triggers set secret_hash = $1 where path = 'gh/legacy'`, hash); err != nil {
		t.Fatalf("legacy row: %v", err)
	}
	store := ledger.NewMemory()
	mux := http.NewServeMux()
	NewHooks(hooks, trigger.NewOpener(store, echoRegistry{}, hookClock{}),
		slog.New(slog.NewTextHandler(io.Discard, nil))).Mount(mux)

	// Signed with the secret whose hash is stored, and with no key at all:
	// the second is what a door that opened an absent sealed secret as empty
	// would accept.
	for _, key := range []string{secret, ""} {
		rec := ghPost(t, mux, ghDelivery{path: "gh/legacy", event: "pull_request", delivery: "d-" + key,
			signature: ghSign(key, prOpened), body: prOpened})
		if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "The secret is missing or wrong.") {
			t.Errorf("signed with %q: status = %d (%s), want the uniform rejection", key, rec.Code, rec.Body)
		}
	}
	if runs, _ := store.Runs(context.Background()); len(runs) != 0 {
		t.Errorf("runs = %v, want none", runs)
	}
}
