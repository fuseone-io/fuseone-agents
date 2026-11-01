package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

// Generating a webhook's secret is granting the ability to make an agent run,
// so it is guarded like pressing the button — and the secret it returns is the
// only time anybody will see it.

// spyHooks stands in for the store: one declared path, unarmed until rotated.
type spyHooks struct {
	path  string
	agent domain.AgentID
	armed bool
	// from makes the path a GitHub hook; with noVault its rotation is refused
	// the way the store refuses it without a master key.
	from    string
	noVault bool
}

func (s *spyHooks) Find(_ context.Context, path string) (trigger.Hook, error) {
	if path != s.path {
		return trigger.Hook{}, trigger.ErrNoHook
	}
	return trigger.Hook{Path: s.path, Agent: s.agent, Armed: s.armed, From: s.from}, nil
}

func (s *spyHooks) Verify(context.Context, string, string) (bool, error) { return false, nil }

func (s *spyHooks) SigningSecret(context.Context, string) ([]byte, error) {
	return nil, trigger.ErrNotGitHub
}

func (s *spyHooks) Rotate(_ context.Context, path string, _ domain.UserID, _ time.Time) (string, error) {
	if path != s.path {
		return "", trigger.ErrNoHook
	}
	if s.from == domain.WebhookFromGitHub && s.noVault {
		return "", trigger.ErrNoVault
	}
	s.armed = true
	secret, _, err := trigger.NewSecret()
	return secret, err
}

func (s *spyHooks) Sync(context.Context, domain.AgentID, domain.Scope, []domain.WebhookTrigger) error {
	return nil
}

func (s *spyHooks) ForAgent(context.Context, domain.AgentID) ([]trigger.Hook, error) {
	return []trigger.Hook{{Path: s.path, Agent: s.agent, Armed: s.armed, From: s.from}}, nil
}

func webhookServer(t *testing.T) *Server {
	t.Helper()
	hooks := &spyHooks{path: "crm/ticket", agent: "triage"}
	return NewServer(ledger.NewMemory(), "test").
		WithAgents(triggerable(t)).WithWebhooks(hooks)
}

func TestRotateWebhookSecret_returnsASecretAndWhereToPostIt(t *testing.T) {
	t.Parallel()

	resp, err := webhookServer(t).RotateWebhookSecret(
		inArea("cx", domain.RoleAuthor),
		openapi.RotateWebhookSecretRequestObject{AgentId: "triage", Path: "crm/ticket"},
	)
	if err != nil {
		t.Fatalf("RotateWebhookSecret: %v", err)
	}

	got, ok := resp.(openapi.RotateWebhookSecret200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the secret", resp)
	}
	if got.Secret == "" {
		t.Error("no secret was returned, and there is no second chance to read it")
	}
	// The integrator has to know where to send it, and guessing the shape of
	// the URL is not something anybody should have to do.
	if got.Url != "/hooks/crm/ticket" {
		t.Errorf("url = %q, want the path the sender posts to", got.Url)
	}
}

func TestRotateWebhookSecret_withoutTriggerPermission_isRefused(t *testing.T) {
	t.Parallel()

	// An auditor can read everything this agent ever did and must not be able
	// to hand somebody a key that makes it run again.
	resp, err := webhookServer(t).RotateWebhookSecret(
		inArea("cx", domain.RoleAuditor),
		openapi.RotateWebhookSecretRequestObject{AgentId: "triage", Path: "crm/ticket"},
	)
	if err != nil {
		t.Fatalf("RotateWebhookSecret: %v", err)
	}
	if _, refused := resp.(openapi.RotateWebhookSecret403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestRotateWebhookSecret_pathOfAnotherAgent_readsAsAbsent(t *testing.T) {
	t.Parallel()

	// Which paths exist is information about the installation, so a path that
	// belongs to somebody else is absent rather than forbidden.
	resp, err := webhookServer(t).RotateWebhookSecret(
		inArea("cx", domain.RoleAuthor),
		openapi.RotateWebhookSecretRequestObject{AgentId: "triage", Path: "erp/order"},
	)
	if err != nil {
		t.Fatalf("RotateWebhookSecret: %v", err)
	}
	if _, absent := resp.(openapi.RotateWebhookSecret404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404", resp)
	}
}

func TestListWebhooks_saysWhichPathsCanActuallyFire(t *testing.T) {
	t.Parallel()

	resp, err := webhookServer(t).ListWebhooks(
		inArea("cx", domain.RoleAuthor),
		openapi.ListWebhooksRequestObject{AgentId: "triage"},
	)
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}

	got := resp.(openapi.ListWebhooks200JSONResponse)
	if len(got.Items) != 1 {
		t.Fatalf("items = %d, want the declared path", len(got.Items))
	}
	// A declared path with no secret is closed, and silence is a bad way to
	// find that out.
	if got.Items[0].Armed {
		t.Error("a path nobody rotated reports itself armed")
	}
}
