package httpapi

import (
	"context"
	"fmt"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/standing"
)

// Whether a standing approval may release a call of the agent is part of the
// definition: published from the form, handed back to the editor, and the
// reason a grant for that agent is refused (#192).

func withStandingApprovals(p openapi.StandingPolicy) func(*openapi.AgentDefinition) {
	return func(d *openapi.AgentDefinition) { d.StandingApprovals = &p }
}

func TestPublishAgent_standingApprovalsSurviveTheConsolePath(t *testing.T) {
	t.Parallel()
	for wire, want := range map[openapi.StandingPolicy]domain.StandingPolicy{
		openapi.StandingPolicyForbidden: domain.StandingForbidden,
		openapi.StandingPolicyAllowed:   domain.StandingAllowed,
	} {
		t.Run(string(wire), func(t *testing.T) {
			t.Parallel()
			pub := newPublisher()
			_, err := publishServer(t, pub).PublishAgent(inArea("cx", domain.RoleAuthor),
				openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(withStandingApprovals(wire))})
			if err != nil {
				t.Fatalf("PublishAgent: %v", err)
			}
			if len(pub.published) != 1 || pub.published[0].StandingApprovals != want {
				t.Fatalf("published = %+v, want standing approvals %q", pub.published, want)
			}
		})
	}
}

func TestPublishAgent_aStandingApprovalsValueNobodyKnows_isRefused(t *testing.T) {
	t.Parallel()
	pub := newPublisher()

	resp, err := publishServer(t, pub).PublishAgent(inArea("cx", domain.RoleAuthor),
		openapi.PublishAgentRequestObject{AgentId: "triage", Body: definition(withStandingApprovals("sometimes"))})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, ok := resp.(openapi.PublishAgent400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	if len(pub.published) != 0 {
		t.Error("published a setting nobody can act on")
	}
}

func TestAgentFrom_handsTheStandingApprovalsSettingBackToTheEditor(t *testing.T) {
	t.Parallel()

	got := agentFrom(domain.AgentSummary{ID: "triage", StandingApprovals: domain.StandingForbidden})
	if got.StandingApprovals == nil || *got.StandingApprovals != openapi.StandingPolicyForbidden {
		t.Fatalf("StandingApprovals = %v, want forbidden: what a read omits, a republish deletes",
			got.StandingApprovals)
	}
	if none := agentFrom(domain.AgentSummary{ID: "triage"}); none.StandingApprovals != nil {
		t.Errorf("StandingApprovals = %v, want absent when allowed", *none.StandingApprovals)
	}
}

// forbiddingStore refuses every grant the way the store does for an agent
// whose current version forbids standing approvals.
type forbiddingStore struct{ standingSpy }

func (f *forbiddingStore) Create(_ context.Context, g standing.Grant) error {
	return fmt.Errorf("%w: %s", standing.ErrStandingForbidden, g.Agent)
}

func TestCreateStandingApproval_agentForbidsThem_refusedWithItsOwnCode(t *testing.T) {
	t.Parallel()
	store := &forbiddingStore{}
	server := standingServer(&store.standingSpy).WithStandingApprovals(store, store)

	resp, err := server.CreateStandingApproval(as(domain.RoleAdmin), openapi.CreateStandingApprovalRequestObject{
		Body: &openapi.CreateStandingApprovalJSONRequestBody{
			ToolId: "github.issue.comment", AgentId: "factory-developer",
			Scope:    openapi.Scope{Company: "acme", Area: "factory"},
			DailyCap: 5, Reason: "would skip the pilot's measurement",
		},
	})
	if err != nil {
		t.Fatalf("CreateStandingApproval: %v", err)
	}
	bad, ok := resp.(openapi.CreateStandingApproval400ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want a refusal", resp)
	}
	// Its own code, so the console says why in the reader's language rather
	// than printing the store's English sentence.
	if valueOr(bad.Type) != string(CodeStandingForbidden) || valueOr(bad.Detail) != "factory-developer" {
		t.Fatalf("problem = %s / %q, want %s naming the agent", valueOr(bad.Type), valueOr(bad.Detail), CodeStandingForbidden)
	}
	if len(store.events) != 0 {
		t.Errorf("events = %v, want no trail for a grant that was not made", store.events)
	}
}
