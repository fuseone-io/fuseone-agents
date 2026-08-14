package httpapi

import (
	gocontext "context"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

/*
Taking an agent out of circulation.

There is no delete and there will not be one: a run is pinned to a version and
that version is the only correct explanation of what the run did. What this
covers is the honest alternative — the agent leaves the listing and keeps
everything.
*/

func TestListAgents_aRetiredAgent_isNotInTheListing(t *testing.T) {
	t.Parallel()

	resp, err := retirable(t, &retirementSpy{out: map[domain.AgentID]bool{"triage": true}}).
		ListAgents(inArea("cx", domain.RoleAuthor), openapi.ListAgentsRequestObject{})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if items := resp.(openapi.ListAgents200JSONResponse).Items; len(items) != 0 {
		t.Errorf("items = %+v, want the retired agent left out", items)
	}
}

// Asked for by name, they come back. Their runs are still readable, and
// somebody reading one has to be able to find the agent it belonged to.
func TestListAgents_askingForTheRetiredOnes_answersWithThem(t *testing.T) {
	t.Parallel()

	resp, err := retirable(t, &retirementSpy{out: map[domain.AgentID]bool{"triage": true}}).
		ListAgents(inArea("cx", domain.RoleAuthor), openapi.ListAgentsRequestObject{
			Params: openapi.ListAgentsParams{Retired: ptr(true)},
		})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	items := resp.(openapi.ListAgents200JSONResponse).Items
	if len(items) != 1 {
		t.Fatalf("items = %+v, want the retired agent", items)
	}
	if items[0].Retired == nil || !*items[0].Retired {
		t.Error("the row does not say it is retired")
	}
}

func TestSetAgentRetired_withoutTheAuthorityToPublish_isRefused(t *testing.T) {
	t.Parallel()

	// Retiring decides whether this agent is still part of the estate, which
	// is an authoring decision. An auditor reads what it did.
	resp, err := retirable(t, &retirementSpy{}).SetAgentRetired(
		inArea("cx", domain.RoleAuditor),
		openapi.SetAgentRetiredRequestObject{
			AgentId: "triage",
			Body:    &openapi.SetAgentRetiredJSONRequestBody{Retired: true},
		})
	if err != nil {
		t.Fatalf("SetAgentRetired: %v", err)
	}
	if _, refused := resp.(openapi.SetAgentRetired403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want a refusal", resp)
	}
}

func TestSetAgentRetired_bringingItBack_restoresRatherThanRetires(t *testing.T) {
	t.Parallel()
	spy := &retirementSpy{}

	if _, err := retirable(t, spy).SetAgentRetired(
		inArea("cx", domain.RoleAuthor),
		openapi.SetAgentRetiredRequestObject{
			AgentId: "triage",
			Body:    &openapi.SetAgentRetiredJSONRequestBody{Retired: false},
		}); err != nil {
		t.Fatalf("SetAgentRetired: %v", err)
	}
	if spy.restored != 1 || spy.retired != 0 {
		t.Errorf("restored=%d retired=%d, want one restore", spy.restored, spy.retired)
	}
}

// --- fixtures ---------------------------------------------------------------

func retirable(t *testing.T, spy *retirementSpy) *Server {
	t.Helper()
	return NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).
		WithRetirements(spy)
}

type retirementSpy struct {
	retired, restored int
	out               map[domain.AgentID]bool
}

func (r *retirementSpy) Retire(gocontext.Context, domain.AgentID, domain.UserID) error {
	r.retired++
	return nil
}

func (r *retirementSpy) Restore(gocontext.Context, domain.AgentID, domain.UserID) error {
	r.restored++
	return nil
}

func (r *retirementSpy) Retired(gocontext.Context) (map[domain.AgentID]bool, error) {
	return r.out, nil
}

// Retiring stops it, and starting it again must not be a way around that: a
// run from an agent no screen lists is one nobody is watching for.
func TestSetAgentPaused_aRetiredAgent_cannotBeStarted(t *testing.T) {
	t.Parallel()

	server := NewServer(ledger.NewMemory(), "test").WithAgents(triggerable(t)).
		WithPublisher(newPublisher()).
		WithRetirements(&retirementSpy{out: map[domain.AgentID]bool{"triage": true}})

	resp, err := server.SetAgentPaused(inArea("cx", domain.RoleAuthor), startAgent(false))
	if err != nil {
		t.Fatalf("SetAgentPaused: %v", err)
	}
	if _, refused := resp.(openapi.SetAgentPaused409ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want it refused", resp)
	}
}
