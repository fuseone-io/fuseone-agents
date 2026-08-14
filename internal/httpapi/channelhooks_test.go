package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Deciding an approval from a conversation.

The property worth the whole stage: a decision taken in Slack is the decision
the console takes. Same permission check against the run's own scope, same
refusal when a later step superseded the ask, same step sealed into the same
chain. What follows tests that by proving the refusals are the console's, not a
weaker set written beside them.
*/

const signingSecret = "8f742231b10e8888abcd99yyyzzz85a5"

func TestSlackInteraction_boundApprover_decidesTheRun(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	park(t, store, "run-1")

	hooks, api := hooksFor(t, store, map[string]domain.UserID{"U024": "usr_ana"},
		principal("usr_ana", domain.RoleApprover, "acme", "cx"))

	rec := press(t, hooks, "acme-slack", "U024", "approve:run-1:2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), string(slack.AnswerApproved)) {
		t.Fatalf("answer = %s", rec.Body.String())
	}

	// The decision is in the ledger, not merely reported in a reply.
	steps, _ := api.store.Read(t.Context(), "run-1", domain.FirstSeq)
	if last := steps[len(steps)-1]; last.Kind != domain.StepApprovalDecided {
		t.Errorf("last step = %s, want the decision sealed", last.Kind)
	}
}

// The heart of it. The account is bound to somebody real who has no business
// approving in this area, and the refusal is the console's own — the channel
// path does not get a laxer one.
func TestSlackInteraction_boundToSomebodyWithoutApproval_isRefusedLikeTheConsole(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	park(t, store, "run-1")

	hooks, api := hooksFor(t, store, map[string]domain.UserID{"U024": "usr_bob"},
		principal("usr_bob", domain.RoleAuthor, "acme", "cx"))

	rec := press(t, hooks, "acme-slack", "U024", "approve:run-1:2")
	if !strings.Contains(rec.Body.String(), string(slack.AnswerForbidden)) {
		t.Fatalf("answer = %s, want the same refusal the console gives", rec.Body.String())
	}

	steps, _ := api.store.Read(t.Context(), "run-1", domain.FirstSeq)
	for _, step := range steps {
		if step.Kind == domain.StepApprovalDecided {
			t.Fatal("a decision was sealed for somebody who cannot approve")
		}
	}
}

func TestSlackInteraction_accountNobodyBound_decidesNothing(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	park(t, store, "run-1")

	hooks, _ := hooksFor(t, store, map[string]domain.UserID{}, domain.Principal{})

	rec := press(t, hooks, "acme-slack", "U999", "approve:run-1:2")
	if !strings.Contains(rec.Body.String(), "not linked") {
		t.Fatalf("answer = %s, want it to say the account is not linked", rec.Body.String())
	}
}

// A message keeps its buttons for ever. Answering a step the run has moved past
// is the stale-tab problem with a longer fuse, and the console's own conflict
// check is what stops it.
func TestSlackInteraction_answersAStepTheRunHasMovedPast_isRefused(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	park(t, store, "run-1")

	hooks, _ := hooksFor(t, store, map[string]domain.UserID{"U024": "usr_ana"},
		principal("usr_ana", domain.RoleApprover, "acme", "cx"))

	rec := press(t, hooks, "acme-slack", "U024", "approve:run-1:97")
	if !strings.Contains(rec.Body.String(), string(slack.AnswerDecided)) {
		t.Fatalf("answer = %s, want the conflict the console reports", rec.Body.String())
	}
}

func TestSlackInteraction_unsigned_neverReachesTheDecision(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	park(t, store, "run-1")

	hooks, api := hooksFor(t, store, map[string]domain.UserID{"U024": "usr_ana"},
		principal("usr_ana", domain.RoleApprover, "acme", "cx"))

	body := payload("U024", "approve:run-1:2")
	req := httptest.NewRequest("POST", "/hooks/channel/acme-slack/slack", strings.NewReader(body))
	req.SetPathValue("channel", "acme-slack")
	rec := httptest.NewRecorder()
	hooks.slackInteraction(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want an unsigned request refused", rec.Code)
	}
	steps, _ := api.store.Read(t.Context(), "run-1", domain.FirstSeq)
	for _, step := range steps {
		if step.Kind == domain.StepApprovalDecided {
			t.Fatal("an unsigned request decided a run")
		}
	}
}

// --- the harness ------------------------------------------------------------

func hooksFor(
	t *testing.T, store *ledger.Memory,
	bound map[string]domain.UserID, who domain.Principal,
) (*ChannelHooks, *Server) {
	t.Helper()
	api := NewServer(store, "test")
	hooks := NewChannelHooks(api,
		&bindingSpy{bound: bound}, &directorySpy{who: who},
		func() time.Time { return time.Now() }, nil)
	return hooks, api
}

func press(t *testing.T, hooks *ChannelHooks, name, user, action string) *httptest.ResponseRecorder {
	t.Helper()
	body := payload(user, action)
	at := time.Now()

	req := httptest.NewRequest("POST", "/hooks/channel/"+name+"/slack", strings.NewReader(body))
	req.SetPathValue("channel", name)
	req.Header.Set("X-Slack-Request-Timestamp", fmt.Sprint(at.Unix()))
	req.Header.Set("X-Slack-Signature", signBody(at, body))

	rec := httptest.NewRecorder()
	hooks.slackInteraction(rec, req)
	return rec
}

func payload(user, action string) string {
	doc := fmt.Sprintf(
		`{"user":{"id":%q},"channel":{"name":"ops"},"actions":[{"value":%q}]}`, user, action)
	return "payload=" + url.QueryEscape(doc)
}

func signBody(at time.Time, body string) string {
	mac := hmac.New(sha256.New, []byte(signingSecret))
	fmt.Fprintf(mac, "v0:%d:%s", at.Unix(), body)
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func park(t *testing.T, store *ledger.Memory, run string) {
	t.Helper()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	for _, step := range []domain.Step{
		{RunID: domain.RunID(run), Kind: domain.StepRunStarted, At: time.Now(),
			Scope: scope, AgentID: "triage", VersionID: "v1"},
		{RunID: domain.RunID(run), Kind: domain.StepApprovalRequested, At: time.Now(),
			Scope: scope, AgentID: "triage", VersionID: "v1",
			Payload: []byte(`{"tool":"crm.reply","rule":"write"}`)},
	} {
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("park: %v", err)
		}
	}
}

func principal(id string, role domain.Role, company, area string) domain.Principal {
	return domain.Principal{
		ID: domain.UserID(id), Kind: domain.PrincipalUser, Display: "Ana",
		Grants: []domain.Grant{{
			Scope: domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
			Role:  role,
		}},
	}
}

type bindingSpy struct{ bound map[string]domain.UserID }

func (b *bindingSpy) PrincipalFor(_ context.Context, _, account string) (domain.UserID, bool) {
	id, ok := b.bound[account]
	return id, ok
}

func (b *bindingSpy) Secrets(context.Context, string) (channel.Credentials, bool) {
	return channel.Credentials{Token: "xoxb-1", Signing: signingSecret}, true
}

type directorySpy struct{ who domain.Principal }

func (d *directorySpy) PrincipalByID(
	_ context.Context, id domain.UserID,
) (domain.Principal, error) {
	if d.who.ID != id {
		return domain.Principal{}, fmt.Errorf("no such principal")
	}
	return d.who, nil
}
