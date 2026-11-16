package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A click that arrives on an area's connection decides only inside that area
(NT-019, phase 4 readers).

Whoever owns the app holds its signing secret and can sign a click as any
account bound to it, for any run. So the person behind the account acts with
the grants they hold inside the connection's scope, and no others. The person
here could approve acme/finance's run from the console; through acme/cx's app
they cannot.
*/
func TestSlackInteraction_onAnAreasConnection_aRunOfAnotherArea_isNotDecided(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	parkAt(t, store, "run-fin", domain.Scope{Company: "acme", Area: "finance"})
	hooks, api := areaHooks(t, store)

	rec := press(t, hooks, "cx-slack", "U024", "approve:run-fin:2")

	if !strings.Contains(rec.Body.String(), string(slack.AnswerForbidden)) {
		t.Fatalf("answer = %s, want the decision refused", rec.Body.String())
	}
	steps, _ := api.store.Read(t.Context(), "run-fin", domain.FirstSeq)
	for _, step := range steps {
		if step.Kind == domain.StepApprovalDecided {
			t.Fatal("acme/cx's app decided acme/finance's run")
		}
	}
}

// The positive half: the same person, the same app, a run of the area the app
// belongs to. Decided, so the refusal above is the fence and not a door that
// refuses everything.
func TestSlackInteraction_onAnAreasConnection_aRunOfThatArea_isDecided(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	parkAt(t, store, "run-cx", domain.Scope{Company: "acme", Area: "cx"})
	hooks, api := areaHooks(t, store)

	rec := press(t, hooks, "cx-slack", "U024", "approve:run-cx:2")

	if !strings.Contains(rec.Body.String(), string(slack.AnswerApproved)) {
		t.Fatalf("answer = %s, want the run approved", rec.Body.String())
	}
	steps, _ := api.store.Read(t.Context(), "run-cx", domain.FirstSeq)
	if last := steps[len(steps)-1]; last.Kind != domain.StepApprovalDecided {
		t.Errorf("last step = %s, want the decision sealed", last.Kind)
	}
}

// areaHooks is the door for acme/cx's own app, with one account bound to a
// person who approves in both acme/cx and acme/finance.
func areaHooks(t *testing.T, store *ledger.Memory) (*ChannelHooks, *Server) {
	t.Helper()
	ana := domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser, Display: "Ana",
		Grants: []domain.Grant{
			{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleApprover},
			{Scope: domain.Scope{Company: "acme", Area: "finance"}, Role: domain.RoleApprover},
		},
	}
	api := NewServer(store, "test")
	hooks := NewChannelHooks(api,
		&bindingSpy{
			bound:    map[string]domain.UserID{"U024": "usr_ana"},
			servesAt: domain.Scope{Company: "acme", Area: "cx"},
		},
		&directorySpy{who: ana}, func() time.Time { return time.Now() }, nil)
	return hooks, api
}

func parkAt(t *testing.T, store *ledger.Memory, run string, scope domain.Scope) {
	t.Helper()
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
