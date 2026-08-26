package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	memstore "github.com/fuseone/agents/internal/memory"
)

func TestListMemoryAssertions_narrowsToReadableScopes(t *testing.T) {
	t.Parallel()
	memory := memstore.NewMemory()
	remember(t, memory, memoryAssertionFixture("cx", "cx fact", nil))
	remember(t, memory, memoryAssertionFixture("marketing", "marketing fact", nil))

	resp, err := NewServer(ledger.NewMemory(), "test").WithMemory(memory).
		ListMemoryAssertions(inArea("cx", domain.RoleAuthor), openapi.ListMemoryAssertionsRequestObject{})
	if err != nil {
		t.Fatalf("ListMemoryAssertions: %v", err)
	}
	page := resp.(openapi.ListMemoryAssertions200JSONResponse)
	if len(page.Items) != 1 || page.Items[0].Subject != "cx fact" {
		t.Fatalf("items = %+v, want only cx memory", page.Items)
	}
}

func TestCreateMemoryAssertion_copiesLabelsFromLedgerEvidence(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	labels := domain.NewLabels(domain.LabelUntrusted).Union(domain.ScopeLabels(scope))
	seedFinishedEvidence(t, store, "run-evidence", scope, labels, "sha256:answer")
	memory := memstore.NewMemory()

	resp, err := NewServer(store, "test").WithMemory(memory).WithClock(fixedAt{t: time.Unix(0, 0)}).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), memoryCreateRequest("sha256:answer"))
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	created := resp.(openapi.CreateMemoryAssertion200JSONResponse)
	if !hasAll(created.Labels, domain.LabelUntrusted, domain.LabelArea(scope)) {
		t.Fatalf("labels = %v, want evidence labels", created.Labels)
	}
}

func TestCreateMemoryAssertion_refusesEvidenceThatDoesNotMatchTheLedger(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	seedFinishedEvidence(t, store, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")

	resp, err := NewServer(store, "test").WithMemory(memstore.NewMemory()).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), memoryCreateRequest("sha256:other"))
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	if _, bad := resp.(openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse); !bad {
		t.Fatalf("response = %T, want 400", resp)
	}
}

func TestCreateMemoryAssertion_withoutPublishPermissionDoesNotReadEvidence(t *testing.T) {
	t.Parallel()
	resp, err := NewServer(readPanicker{Memory: ledger.NewMemory()}, "test").
		WithMemory(memstore.NewMemory()).
		CreateMemoryAssertion(context.Background(), memoryCreateRequest("sha256:answer"))
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	if _, refused := resp.(openapi.CreateMemoryAssertion403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
}

func TestListMemorySuggestions_narrowsToReadableScopes(t *testing.T) {
	t.Parallel()
	memory := memstore.NewMemory()
	suggest(t, memory, memorySuggestionFixture("cx", "cx suggestion", nil))
	suggest(t, memory, memorySuggestionFixture("marketing", "marketing suggestion", nil))

	resp, err := NewServer(ledger.NewMemory(), "test").WithMemory(memory).
		ListMemorySuggestions(inArea("cx", domain.RoleAuthor), openapi.ListMemorySuggestionsRequestObject{})
	if err != nil {
		t.Fatalf("ListMemorySuggestions: %v", err)
	}
	page := resp.(openapi.ListMemorySuggestions200JSONResponse)
	if len(page.Items) != 1 || page.Items[0].Subject != "cx suggestion" {
		t.Fatalf("items = %+v, want only cx suggestions", page.Items)
	}
}

func TestAcceptMemorySuggestion_promotesOnlyInsideTheReviewScope(t *testing.T) {
	t.Parallel()
	memory := memstore.NewMemory()
	created := suggest(t, memory, memorySuggestionFixture("cx", "cx suggestion", func(s *domain.MemorySuggestion) {
		s.Labels = domain.NewLabels(domain.LabelUntrusted)
	}))

	resp, err := NewServer(ledger.NewMemory(), "test").WithMemory(memory).WithClock(fixedAt{t: time.Unix(0, 0)}).
		AcceptMemorySuggestion(inArea("marketing", domain.RoleAuthor), openapi.AcceptMemorySuggestionRequestObject{
			SuggestionId: created.ID,
			Body: &openapi.MemorySuggestionReviewInput{
				Company: "acme", Area: "marketing", Reason: "reviewed",
			},
		})
	if err != nil {
		t.Fatalf("AcceptMemorySuggestion outside scope: %v", err)
	}
	if _, absent := resp.(openapi.AcceptMemorySuggestion404ApplicationProblemPlusJSONResponse); !absent {
		t.Fatalf("response = %T, want 404 for suggestion outside review scope", resp)
	}

	resp, err = NewServer(ledger.NewMemory(), "test").WithMemory(memory).WithClock(fixedAt{t: time.Unix(0, 0)}).
		AcceptMemorySuggestion(inArea("cx", domain.RoleAuthor), openapi.AcceptMemorySuggestionRequestObject{
			SuggestionId: created.ID,
			Body: &openapi.MemorySuggestionReviewInput{
				Company: "acme", Area: "cx", Reason: "operator confirmed",
			},
		})
	if err != nil {
		t.Fatalf("AcceptMemorySuggestion: %v", err)
	}
	assertion := resp.(openapi.AcceptMemorySuggestion200JSONResponse)
	if assertion.Subject != "cx suggestion" || !hasAll(assertion.Labels, domain.LabelUntrusted) {
		t.Fatalf("assertion = %+v, want accepted suggestion labels preserved", assertion)
	}
}

type readPanicker struct{ *ledger.Memory }

func (readPanicker) Read(context.Context, domain.RunID, int64) ([]domain.Step, error) {
	panic("evidence was read before authorisation")
}

func remember(t *testing.T, memory *memstore.Memory, a domain.MemoryAssertion) {
	t.Helper()
	if _, err := memory.Assert(context.Background(), a, "usr_ana", "reviewed", time.Unix(0, 0)); err != nil {
		t.Fatalf("Assert: %v", err)
	}
}

func suggest(t *testing.T, memory *memstore.Memory, s domain.MemorySuggestion) domain.MemorySuggestion {
	t.Helper()
	out, err := memory.Suggest(context.Background(), s, domain.MemoryLearningPolicy{
		Mode: domain.MemoryLearningReview,
	}, "agent:triage", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	return out.Suggestion
}

func memoryAssertionFixture(area, subject string, edit func(*domain.MemoryAssertion)) domain.MemoryAssertion {
	a := domain.MemoryAssertion{
		Scope: domain.Scope{Company: "acme", Area: domain.AreaID(area)}, AgentID: "triage",
		Kind: "incident", Subject: subject, Signature: subject + ".signature",
		Claim: "remembered operator-approved behaviour",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-evidence", Artifact: domain.ArtifactFinalAnswer, Digest: "sha256:answer",
		}},
		Observations: 1, Confirmed: 1, Status: domain.MemoryActive,
	}
	if edit != nil {
		edit(&a)
	}
	return a
}

func memorySuggestionFixture(area, subject string, edit func(*domain.MemorySuggestion)) domain.MemorySuggestion {
	s := domain.MemorySuggestion{
		Scope: domain.Scope{Company: "acme", Area: domain.AreaID(area)}, AgentID: "triage",
		Kind: "incident", Subject: subject, Signature: subject + ".signature",
		Claim: "suggested operator-reviewed behaviour",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-evidence", Artifact: domain.ArtifactMemorySuggestion, Digest: "sha256:suggest",
		}},
		Labels: domain.ScopeLabels(domain.Scope{Company: "acme", Area: domain.AreaID(area)}),
	}
	if edit != nil {
		edit(&s)
	}
	return s
}

func seedFinishedEvidence(
	t *testing.T, store *ledger.Memory, run domain.RunID, scope domain.Scope,
	labels domain.Labels, digest string,
) {
	t.Helper()
	appendMemoryStep(t, store, domain.Step{RunID: run, Kind: domain.StepRunStarted,
		Scope: scope, AgentID: "triage", VersionID: "v1", Labels: labels})
	appendMemoryStep(t, store, domain.Step{RunID: run, Kind: domain.StepRunFinished,
		Scope: scope, AgentID: "triage", VersionID: "v1", Labels: labels,
		Payload: jsonPayload(t, domain.RunFinishedPayload{OutcomeDigest: digest})})
}

func appendMemoryStep(t *testing.T, store *ledger.Memory, step domain.Step) {
	t.Helper()
	if _, err := store.Append(context.Background(), step); err != nil {
		t.Fatalf("append %s: %v", step.Kind, err)
	}
}

func memoryCreateRequest(digest string) openapi.CreateMemoryAssertionRequestObject {
	return openapi.CreateMemoryAssertionRequestObject{Body: &openapi.MemoryAssertionInput{
		Company: "acme", Area: "cx", AgentId: ptr("triage"), Kind: "incident",
		Subject: "grafana datasource", Signature: "grafana.datasource.down",
		Claim: "refreshing the datasource token cleared this failure",
		Evidence: []openapi.MemoryEvidence{{
			RunId: "run-evidence", Artifact: domain.ArtifactFinalAnswer, Digest: digest,
		}},
		Reason: "operator reviewed the incident",
	}}
}

func jsonPayload(t *testing.T, v domain.RunFinishedPayload) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	return raw
}

func hasAll(values []string, expected ...string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range expected {
		if !seen[value] {
			return false
		}
	}
	return true
}
