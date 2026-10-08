package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/standing"
)

type standingSpy struct {
	created []standing.Grant
	revoked []string
	listed  []standing.Grant
	events  []string
}

func (s *standingSpy) Create(_ context.Context, g standing.Grant) error {
	if err := standing.Validate(g, g.CreatedAt); err != nil {
		return err
	}
	s.created = append(s.created, g)
	return nil
}
func (s *standingSpy) Revoke(_ context.Context, id string, _ domain.UserID, _ time.Time) error {
	s.revoked = append(s.revoked, id)
	return nil
}
func (s *standingSpy) List(context.Context) ([]standing.Grant, error) { return s.listed, nil }
func (s *standingSpy) RecordStanding(_ context.Context, action, target string, _ domain.UserID, _ domain.Scope) error {
	s.events = append(s.events, action+":"+target)
	return nil
}

func standingServer(spy *standingSpy) *Server {
	return NewServer(ledger.NewMemory(), "test").WithStandingApprovals(spy, spy)
}

// Granting ahead of time is strictly more than clicking one card: the
// Approver and the Curator are refused; only the holder of approval:grant
// creates, and the creation lands in the operator trail.
func TestCreateStandingApproval_needsItsOwnPermissionAndLeavesATrail(t *testing.T) {
	t.Parallel()
	spy := &standingSpy{}
	server := standingServer(spy)
	body := &openapi.CreateStandingApprovalJSONRequestBody{
		ToolId: "cloudflare.edge.block_ip", AgentId: "sentinel",
		Scope: openapi.Scope{Company: "acme", Area: "platform"},
		DailyCap: 5, Reason: "night shift blocking",
	}

	for _, role := range []domain.Role{domain.RoleApprover, domain.RoleCurator} {
		resp, err := server.CreateStandingApproval(as(role),
			openapi.CreateStandingApprovalRequestObject{Body: body})
		if err != nil {
			t.Fatalf("CreateStandingApproval(%s): %v", role, err)
		}
		if _, ok := resp.(openapi.CreateStandingApproval403ApplicationProblemPlusJSONResponse); !ok {
			t.Fatalf("%s got %T, want 403", role, resp)
		}
	}
	if len(spy.created) != 0 {
		t.Fatal("a refused role created a grant")
	}

	resp, err := server.CreateStandingApproval(as(domain.RoleAdmin),
		openapi.CreateStandingApprovalRequestObject{Body: body})
	if err != nil {
		t.Fatalf("CreateStandingApproval: %v", err)
	}
	if _, ok := resp.(openapi.CreateStandingApproval204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if len(spy.created) != 1 || spy.created[0].CreatedBy == "" {
		t.Fatalf("created = %+v, want the caller's name on the grant", spy.created)
	}
	// Empty expiry means the default, not forever.
	if spy.created[0].ExpiresAt.IsZero() {
		t.Fatal("the grant has no expiry")
	}
	if len(spy.events) != 1 || spy.events[0] != "standing_approval.created:"+spy.created[0].ID {
		t.Fatalf("events = %v", spy.events)
	}
}

// A pattern for a tool is refused at the boundary with the store's own words.
func TestCreateStandingApproval_aGlobTool_is400(t *testing.T) {
	t.Parallel()
	spy := &standingSpy{}
	resp, err := standingServer(spy).CreateStandingApproval(as(domain.RoleAdmin),
		openapi.CreateStandingApprovalRequestObject{
			Body: &openapi.CreateStandingApprovalJSONRequestBody{
				ToolId: "cloudflare.*", AgentId: "sentinel",
				Scope:    openapi.Scope{Company: "acme"},
				DailyCap: 5, Reason: "x",
			},
		})
	if err != nil {
		t.Fatalf("CreateStandingApproval: %v", err)
	}
	if _, ok := resp.(openapi.CreateStandingApproval400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want 400", resp)
	}
}

func TestRevokeStandingApproval_recordsWhoEndedTheMandate(t *testing.T) {
	t.Parallel()
	spy := &standingSpy{}
	resp, err := standingServer(spy).RevokeStandingApproval(as(domain.RoleAdmin),
		openapi.RevokeStandingApprovalRequestObject{Grant: "SG-1"})
	if err != nil {
		t.Fatalf("RevokeStandingApproval: %v", err)
	}
	if _, ok := resp.(openapi.RevokeStandingApproval204Response); !ok {
		t.Fatalf("response = %T, want 204", resp)
	}
	if len(spy.revoked) != 1 || spy.events[len(spy.events)-1] != "standing_approval.revoked:SG-1" {
		t.Fatalf("revoked = %v events = %v", spy.revoked, spy.events)
	}
}
