package httpapi

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/policy"
)

// A policy constrains people who must not be able to change it. Reading and
// writing are different authorities, and the API is where that holds.

type policyStore struct {
	set     policy.Set
	written domain.Policy
	deleted string
}

func (s *policyStore) Active(context.Context) (policy.Set, error) { return s.set, nil }

func (s *policyStore) Put(_ context.Context, p domain.Policy, _ domain.UserID) (policy.Set, error) {
	s.written = p
	return policy.Set{Hash: "pol_after"}, nil
}

func (s *policyStore) Delete(_ context.Context, code string) (policy.Set, error) {
	s.deleted = code
	return policy.Set{Hash: "pol_after"}, nil
}

func written(over func(*openapi.PolicyInput)) *openapi.PutPolicyJSONRequestBody {
	body := openapi.PolicyInput{
		Name: "Sem exportação em massa", Effect: "deny", Mode: "monitor",
	}
	if over != nil {
		over(&body)
	}
	return &body
}

func TestPutPolicy_asAnAuthor_isRefused(t *testing.T) {
	t.Parallel()

	// An author reads the rule that stopped their agent and cannot edit it.
	// A rule its subject can change is that person's rule, not the
	// organisation's.
	resp, err := NewServer(ledger.NewMemory(), "test").WithPolicies(&policyStore{}).
		PutPolicy(inArea("cx", domain.RoleAuthor), openapi.PutPolicyRequestObject{
			Code: "POL-114", Body: written(nil),
		})
	if err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}
	if _, refused := resp.(openapi.PutPolicy403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestListPolicies_asAnAuthor_isAllowed(t *testing.T) {
	t.Parallel()

	// The other half of the same rule: somebody constrained by a policy has to
	// be able to read it, or they cannot act on being stopped.
	resp, err := NewServer(ledger.NewMemory(), "test").WithPolicies(&policyStore{}).
		ListPolicies(inArea("cx", domain.RoleAuthor), openapi.ListPoliciesRequestObject{})
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if _, ok := resp.(openapi.ListPolicies200JSONResponse); !ok {
		t.Fatalf("response = %T, want the policies", resp)
	}
}

func TestPutPolicy_asACurator_storesTheRuleAndReturnsTheNewHash(t *testing.T) {
	t.Parallel()
	store := &policyStore{}

	resp, err := NewServer(ledger.NewMemory(), "test").WithPolicies(store).
		PutPolicy(as(domain.RoleCurator), openapi.PutPolicyRequestObject{
			Code: "POL-114",
			Body: written(func(in *openapi.PolicyInput) {
				in.Resource = ptr("crm.*")
				in.Conditions = &[]openapi.PolicyCondition{
					{Field: "args.rows", Op: "gt", Value: "100"},
				}
			}),
		})
	if err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}

	got := resp.(openapi.PutPolicy200JSONResponse)
	if got.PolicyHash != "pol_after" {
		t.Errorf("hash = %q, want the set's new name", got.PolicyHash)
	}
	if store.written.Code != "POL-114" || len(store.written.Conditions) != 1 {
		t.Errorf("stored = %+v, want the rule as written", store.written)
	}
	// The fields the Gate evaluates travel back, and the console renders the
	// line from them. The server used to compose that sentence too, which was
	// two renderings of one structure and, being prose in a binary, arrived in
	// one language for every reader.
	if got.Policy.Resource == nil || *got.Policy.Resource == "" || got.Policy.Effect == "" {
		t.Errorf("policy = %+v, want the fields a sentence is rendered from", got.Policy)
	}
}

func TestPutPolicy_reachNamingNothing_isRefused(t *testing.T) {
	t.Parallel()

	// A rule scoped to no agents covers nothing while reading on the screen
	// as a rule in force.
	resp, err := NewServer(ledger.NewMemory(), "test").WithPolicies(&policyStore{}).
		PutPolicy(as(domain.RoleCurator), openapi.PutPolicyRequestObject{
			Code: "POL-114",
			Body: written(func(in *openapi.PolicyInput) { in.Reach = ptr(openapi.PolicyInputReach("agents")) }),
		})
	if err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}
	if _, refused := resp.(openapi.PutPolicy400ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 400", resp)
	}
}

func TestListPolicies_countsWhatEachRuleActuallyDecided(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := ledger.NewMemory()

	seedPolicyDecision(t, store, "run-1", "POL-114")
	seedPolicyDecision(t, store, "run-2", "POL-114")

	resp, err := NewServer(store, "test").WithPolicies(&policyStore{
		set: policy.Set{Hash: "pol_x", Policies: []domain.Policy{{Code: "POL-114", Name: "x"}}},
	}).ListPolicies(as(domain.RoleCurator), openapi.ListPoliciesRequestObject{})
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}

	page := resp.(openapi.ListPolicies200JSONResponse)
	// From the ledger, not a counter. A counter drifts; the trail is what
	// happened.
	if page.Items[0].Hits == nil || *page.Items[0].Hits != 2 {
		t.Errorf("hits = %v, want 2", page.Items[0].Hits)
	}
	_ = ctx
}

func seedPolicyDecision(t *testing.T, store *ledger.Memory, runID domain.RunID, code string) {
	t.Helper()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	now := time.Now()
	for _, step := range []domain.Step{
		{RunID: runID, Kind: domain.StepRunStarted, Scope: scope, AgentID: "triage", VersionID: "v1", At: now},
		{RunID: runID, Kind: domain.StepGateDecided, Scope: scope, AgentID: "triage", VersionID: "v1", At: now,
			Payload: mustPayload(t, domain.GateDecidedPayload{
				Tool: "crm.reply", Verdict: domain.VerdictBlock, Rule: "policy", PolicyCode: code,
			})},
	} {
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestSimulatePolicy_beforeTheRuleIsNamed_stillAnswers(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	seedPolicyDecision(t, store, "run-1", "")

	// What a rule does has nothing to do with what it is called. Refusing
	// until somebody names it would make the safety check the last thing
	// anybody runs, which is the one place it is useless.
	resp, err := NewServer(store, "test").WithPolicies(&policyStore{}).
		SimulatePolicy(inArea("cx", domain.RoleAuthor), openapi.SimulatePolicyRequestObject{
			Body: &openapi.SimulatePolicyJSONRequestBody{
				Name: "", Effect: "deny", Mode: "monitor", Resource: ptr("crm.*"),
			},
		})
	if err != nil {
		t.Fatalf("SimulatePolicy: %v", err)
	}

	got, ok := resp.(openapi.SimulatePolicy200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the simulation", resp)
	}
	if got.Considered == 0 {
		t.Error("nothing was considered, so nothing was replayed")
	}
}
