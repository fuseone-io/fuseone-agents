package admin_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

/*
A browser sign-in in flight: begun when a person asks to connect, consumed
once when their browser comes back. The state is the request's only name and
is never stored as itself; the PKCE verifier is sealed. Consuming takes the
row whatever the answer, so a request is tried once, and decides under the
row's lock, so two returns of one browser cannot both use it.
*/

var flowNow = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func flowFor(state string, principal domain.UserID) admin.MCPOAuthFlow {
	return admin.MCPOAuthFlow{
		State: state, Verifier: "verifier-" + state, Principal: principal,
		Server: signin, ServerScope: domain.Scope{Company: domain.Installation},
		ClientID: "agents", TokenAuthMethod: domain.TokenAuthNone,
		TokenURL: "https://idp.example.com/token", Issuer: "https://idp.example.com",
		RedirectURI: "https://agents.example.com/auth/mcp/callback",
		ExpiresAt:   flowNow.Add(10 * time.Minute),
	}
}

func withFlows(t *testing.T) moveWorld {
	t.Helper()
	w := withSigninWorld(t)
	clean := func() { _, _ = w.pool.Exec(context.Background(), `delete from settings where kind = 'mcp_oauth_flow'`) }
	clean()
	t.Cleanup(clean)
	return w
}

func TestMCPOAuthFlow_isConsumedOnce_byThePersonWhoBeganIt(t *testing.T) {
	w := withFlows(t)
	ctx := context.Background()
	if err := w.integrations.BeginMCPOAuthFlow(ctx, flowFor("st-1", "usr_ana"), flowNow); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	got, err := w.integrations.ConsumeMCPOAuthFlow(ctx, "st-1", "usr_ana", flowNow.Add(time.Minute))
	if err != nil || got.Verifier != "verifier-st-1" || got.Server != signin || got.TokenURL != "https://idp.example.com/token" {
		t.Fatalf("Consume = %+v, %v; want the flow and its verifier", got, err)
	}
	if _, err := w.integrations.ConsumeMCPOAuthFlow(ctx, "st-1", "usr_ana", flowNow.Add(time.Minute)); !errors.Is(err, admin.ErrMCPOAuthFlowUnknown) {
		t.Errorf("a second return: err = %v, want ErrMCPOAuthFlowUnknown", err)
	}
}

// Another person's browser returning with the state is refused, and the
// request is spent: its owner starts again rather than racing a stranger.
func TestMCPOAuthFlow_anotherPersonsReturn_isRefusedAndSpendsIt(t *testing.T) {
	w := withFlows(t)
	ctx := context.Background()
	if err := w.integrations.BeginMCPOAuthFlow(ctx, flowFor("st-2", "usr_ana"), flowNow); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := w.integrations.ConsumeMCPOAuthFlow(ctx, "st-2", "usr_bruno", flowNow); !errors.Is(err, admin.ErrMCPOAuthFlowNotYours) {
		t.Errorf("err = %v, want ErrMCPOAuthFlowNotYours", err)
	}
	if _, err := w.integrations.ConsumeMCPOAuthFlow(ctx, "st-2", "usr_ana", flowNow); !errors.Is(err, admin.ErrMCPOAuthFlowUnknown) {
		t.Errorf("the owner after: err = %v, want ErrMCPOAuthFlowUnknown", err)
	}
}

func TestMCPOAuthFlow_returnedTooLate_isExpired(t *testing.T) {
	w := withFlows(t)
	ctx := context.Background()
	if err := w.integrations.BeginMCPOAuthFlow(ctx, flowFor("st-3", "usr_ana"), flowNow); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if _, err := w.integrations.ConsumeMCPOAuthFlow(ctx, "st-3", "usr_ana", flowNow.Add(11*time.Minute)); !errors.Is(err, admin.ErrMCPOAuthFlowExpired) {
		t.Errorf("err = %v, want ErrMCPOAuthFlowExpired", err)
	}
}

func TestMCPOAuthFlow_holdsNeitherTheStateNorTheVerifierInTheOpen(t *testing.T) {
	w := withFlows(t)
	ctx := context.Background()
	if err := w.integrations.BeginMCPOAuthFlow(ctx, flowFor("state-canary", "usr_ana"), flowNow); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	var name, value string
	if err := w.pool.QueryRow(ctx, `select name, value::text from settings where kind = 'mcp_oauth_flow'`).Scan(&name, &value); err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, open := range []string{name, value} {
		if strings.Contains(open, "state-canary") || strings.Contains(open, "verifier-") {
			t.Errorf("found in the open: %s", open)
		}
	}
}

// A request nobody came back for does not stay: the next one begun clears it.
func TestBeginMCPOAuthFlow_clearsTheExpired(t *testing.T) {
	w := withFlows(t)
	ctx := context.Background()
	if err := w.integrations.BeginMCPOAuthFlow(ctx, flowFor("old", "usr_ana"), flowNow); err != nil {
		t.Fatalf("Begin old: %v", err)
	}
	if err := w.integrations.BeginMCPOAuthFlow(ctx, flowFor("new", "usr_bruno"), flowNow.Add(time.Hour)); err != nil {
		t.Fatalf("Begin new: %v", err)
	}
	var count int
	if err := w.pool.QueryRow(ctx, `select count(*) from settings where kind = 'mcp_oauth_flow'`).Scan(&count); err != nil || count != 1 {
		t.Errorf("flows = %d, %v; want the new one alone", count, err)
	}
}

// Decided under the row's lock: a return that reads the request while another
// holds it waits, and then finds it gone, rather than both using it.
func TestConsumeMCPOAuthFlow_waitsForAReturnHoldingIt_andFindsItSpent(t *testing.T) {
	w := withFlows(t)
	ctx := context.Background()
	if err := w.integrations.BeginMCPOAuthFlow(ctx, flowFor("st-race", "usr_ana"), flowNow); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		delete from settings where kind = 'mcp_oauth_flow'`); err != nil {
		t.Fatalf("hold and spend the request: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := w.integrations.ConsumeMCPOAuthFlow(ctx, "st-race", "usr_ana", flowNow)
		done <- err
	}()
	if !waitForRowWait(t, w.pool) {
		t.Fatal("the return never waited for the one holding the request")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; !errors.Is(err, admin.ErrMCPOAuthFlowUnknown) {
		t.Errorf("err = %v, want ErrMCPOAuthFlowUnknown: the request was spent meanwhile", err)
	}
}
