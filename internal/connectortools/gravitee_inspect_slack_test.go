package connectortools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

// A requester nobody bound is a Slack account, and the email that account's
// own profile shows is what the application's owner is compared with.
func TestGraviteeInspector_aSlackRequesterIsComparedByTheirSlackEmail(t *testing.T) {
	tickets, ctxTicket := inspectedTicketBy(t, domain.SlackRequester("U09"))
	accounts := &fakeAccountEmails{email: "dev@example.com"}
	inspector := NewGraviteeInspector(
		&fakeGraviteeAccess{access: inspectionAccess()}, &fakeGraviteeRemote{observation: safeObservation()},
		fakeRequesterEmails{err: errors.New("no such person")}, engine.NewMemoryContent(), tickets,
	).WithAccountEmails(accounts)
	inspector.now = func() time.Time { return graviteeNow }

	if _, err := inspector.Inspect(t.Context(), "apim", inspectionCall(ctxTicket),
		inspectedWithinPolicy()); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if accounts.asked != "slack/U09" {
		t.Fatalf("asked %q, want the account on the ticket's own connection", accounts.asked)
	}
}

func TestGraviteeInspector_aSlackRequesterWithoutTheOwnersEmail_isRefused(t *testing.T) {
	for name, accounts := range map[string]*fakeAccountEmails{
		"not wired":      nil,
		"another email":  {email: "someone@example.com"},
		"not shared":     {email: ""},
		"profile denied": {err: errors.New("slack: refused: missing_scope")},
	} {
		t.Run(name, func(t *testing.T) {
			tickets, ctxTicket := inspectedTicketBy(t, domain.SlackRequester("U09"))
			remote := &fakeGraviteeRemote{observation: safeObservation()}
			// The person lookup knows the owner's email, and must not be asked:
			// a Slack account is nobody's person.
			inspector := NewGraviteeInspector(
				&fakeGraviteeAccess{access: inspectionAccess()}, remote,
				fakeRequesterEmails{email: "dev@example.com"}, engine.NewMemoryContent(), tickets)
			if accounts != nil {
				inspector.WithAccountEmails(accounts)
			}
			inspector.now = func() time.Time { return graviteeNow }

			_, err := inspector.Inspect(t.Context(), "apim", inspectionCall(ctxTicket),
				inspectedWithinPolicy())
			if !errors.Is(err, ErrGraviteeRequester) {
				t.Fatalf("Inspect = %v, want the requester refused", err)
			}
		})
	}
}

type fakeAccountEmails struct {
	email string
	err   error
	asked string
}

func (f *fakeAccountEmails) AccountEmail(_ context.Context, connection, account string) (string, error) {
	f.asked = connection + "/" + account
	return f.email, f.err
}

func inspectedWithinPolicy() GraviteeInspectInput {
	return GraviteeInspectInput{
		SubscriptionID: "sub-42", ExpiresAt: graviteeNow.Add(48 * time.Hour).Format(time.RFC3339),
	}
}
