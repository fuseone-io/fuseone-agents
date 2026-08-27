package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

/*
Three kinds of no, told apart.

They used to be one. A body the server would not accept, a state that
contradicts the write, and a database that is not answering all came back as
400 with a sentence, so the console could offer nothing better than "check your
input" to somebody whose installation was down — and nothing at all to somebody
whose memory has two rows claiming one identity, which is the one case where
there is something for a person to go and fix.
*/
func TestCreateMemoryAssertion_tellsInvalidFromConflictedFromUnavailable(t *testing.T) {
	t.Parallel()

	t.Run("a claim the store will not accept is a bad request", func(t *testing.T) {
		t.Parallel()
		resp := createAgainst(t, memstore.NewMemory(), func(in *openapi.MemoryAssertionInput) {
			in.Claim = strings.Repeat("a claim nobody could read ", 100)
		})
		if _, bad := resp.(openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse); !bad {
			t.Fatalf("response = %T, want 400", resp)
		}
	})

	t.Run("shared memory already answering it is a conflict", func(t *testing.T) {
		t.Parallel()
		store := memstore.NewMemory()
		// Shared memory, which the agent-scoped write covers rather than
		// corrects: it is what every agent in the scope reads.
		remember(t, store, memoryAssertionFixture("cx", "grafana datasource",
			func(a *domain.MemoryAssertion) {
				a.AgentID, a.Signature = "", "grafana.datasource.down"
			}))
		assertMemoryConflict(t, createAgainst(t, store, nil))
	})

	t.Run("a database that does not answer is not the caller's fault", func(t *testing.T) {
		t.Parallel()
		resp := createAgainst(t, unavailableMemory{}, nil)
		if resp != nil {
			t.Fatalf("response = %T, want the error propagated as an internal failure", resp)
		}
	})
}

func TestAcceptMemorySuggestion_tellsConflictFromUnavailable(t *testing.T) {
	t.Parallel()

	t.Run("a disabled assertion refuses the accept", func(t *testing.T) {
		t.Parallel()
		store := memstore.NewMemory()
		stored := memoryAssertionFixture("cx", "grafana datasource", nil)
		remember(t, store, stored)
		id := domain.MemoryAssertionID(stored)
		if err := store.Disable(context.Background(), id,
			domain.Scope{Company: "acme", Area: "cx"}, "usr_ana", "wrong", time.Unix(0, 0)); err != nil {
			t.Fatalf("Disable: %v", err)
		}
		pending := suggest(t, store, memorySuggestionFixture("cx", "grafana datasource",
			func(s *domain.MemorySuggestion) { s.Signature = "grafana datasource.signature" }))

		resp, err := NewServer(ledger.NewMemory(), "test").WithMemory(store).
			AcceptMemorySuggestion(inArea("cx", domain.RoleAuthor),
				openapi.AcceptMemorySuggestionRequestObject{
					SuggestionId: pending.ID,
					Body: &openapi.MemorySuggestionReviewInput{
						Company: "acme", Area: "cx", Reason: "agreed",
					},
				})
		if err != nil {
			t.Fatalf("AcceptMemorySuggestion: %v", err)
		}
		if _, conflict := resp.(openapi.AcceptMemorySuggestion409ApplicationProblemPlusJSONResponse); !conflict {
			t.Fatalf("response = %T, want 409", resp)
		}
	})

	t.Run("a database that does not answer is not the caller's fault", func(t *testing.T) {
		t.Parallel()
		resp, err := NewServer(ledger.NewMemory(), "test").WithMemory(unavailableMemory{}).
			AcceptMemorySuggestion(inArea("cx", domain.RoleAuthor),
				openapi.AcceptMemorySuggestionRequestObject{
					SuggestionId: "mems_whatever",
					Body: &openapi.MemorySuggestionReviewInput{
						Company: "acme", Area: "cx", Reason: "agreed",
					},
				})
		if err == nil {
			t.Fatalf("response = %T, want the error propagated as an internal failure", resp)
		}
	})
}

// createAgainst runs a creation whose evidence the ledger vouches for, so the
// only thing under test is what the store answered.
func createAgainst(
	t *testing.T, store Memory, edit func(*openapi.MemoryAssertionInput),
) openapi.CreateMemoryAssertionResponseObject {
	t.Helper()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	led := ledger.NewMemory()
	seedFinishedEvidence(t, led, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")

	req := memoryCreateRequest("sha256:answer")
	if edit != nil {
		edit(req.Body)
	}
	resp, err := NewServer(led, "test").WithMemory(store).
		WithClock(fixedAt{t: time.Unix(0, 0)}).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		return nil
	}
	return resp
}

func assertMemoryConflict(t *testing.T, resp openapi.CreateMemoryAssertionResponseObject) {
	t.Helper()
	conflict, ok := resp.(openapi.CreateMemoryAssertion409ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 409", resp)
	}
	if conflict.Type == nil || *conflict.Type != string(CodeConflict) {
		t.Errorf("type = %v, want the stable conflict code", conflict.Type)
	}
}

// unavailableMemory is the installation not answering: every call fails the way
// an unreachable database does, with nothing the caller could have done
// differently.
type unavailableMemory struct{}

var errMemoryUnreachable = errors.New("dial tcp: connection refused")

func (unavailableMemory) List(context.Context, memstore.Filter) ([]domain.MemoryAssertion, error) {
	return nil, errMemoryUnreachable
}

func (unavailableMemory) Assert(
	context.Context, domain.MemoryAssertion, domain.UserID, string, time.Time,
) (domain.MemoryAssertion, error) {
	return domain.MemoryAssertion{}, errMemoryUnreachable
}

func (unavailableMemory) Disable(
	context.Context, string, domain.Scope, domain.UserID, string, time.Time,
) error {
	return errMemoryUnreachable
}

func (unavailableMemory) ListSuggestions(
	context.Context, memstore.SuggestionFilter,
) ([]domain.MemorySuggestion, error) {
	return nil, errMemoryUnreachable
}

func (unavailableMemory) AcceptSuggestion(
	context.Context, string, domain.Scope, domain.UserID, string, time.Time,
) (domain.MemoryAssertion, error) {
	return domain.MemoryAssertion{}, errMemoryUnreachable
}

func (unavailableMemory) DismissSuggestion(
	context.Context, string, domain.Scope, domain.UserID, string, time.Time,
) error {
	return errMemoryUnreachable
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

/*
seedFinishedEvidence builds a run the way the runner builds one.

The taint is named once, on the step that brought it in, and the closing answer
carries what the run had accumulated by then. Stamping both from the same
literal was how this fixture used to be written, and it hid a live bug for
releases: the runner did not label the finish step at all, so the assertion that
labels reach memory passed against a shape production never wrote.
*/
func seedFinishedEvidence(
	t *testing.T, store *ledger.Memory, run domain.RunID, scope domain.Scope,
	openedWith domain.Labels, digest string,
) {
	t.Helper()
	appendMemoryStep(t, store, domain.Step{RunID: run, Kind: domain.StepRunStarted,
		Scope: scope, AgentID: "triage", VersionID: "v1", Labels: openedWith})

	accumulated := domain.ScopeLabels(scope).Union(openedWith)
	appendMemoryStep(t, store, domain.Step{RunID: run, Kind: domain.StepRunFinished,
		Scope: scope, AgentID: "triage", VersionID: "v1", Labels: accumulated,
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

/*
A memory taught from the answer of a tainted run inherits the taint.

This is the shape the console cites by default and the one that was broken: the
run opened untrusted, the answer restated what arrived, and the assertion came
out clean because the finish step carried no labels for the handler to copy.
The taint is set here only where it enters, so nothing but the run's own fold
can put it on the memory.
*/
func TestCreateMemoryAssertion_citingTheFinalAnswerOfATaintedRun_inheritsUntrusted(t *testing.T) {
	t.Parallel()

	// The area the create request names; memoryCreateRequest fixes it.
	scope := domain.Scope{Company: "acme", Area: "cx"}
	store := ledger.NewMemory()
	seedFinishedEvidence(t, store, "run-evidence", scope,
		domain.NewLabels(domain.LabelUntrusted), "sha256:answer")

	resp, err := NewServer(store, "test").WithMemory(memstore.NewMemory()).
		WithClock(fixedAt{t: time.Unix(0, 0)}).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), memoryCreateRequest("sha256:answer"))
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	created := resp.(openapi.CreateMemoryAssertion200JSONResponse)
	if !hasAll(created.Labels, domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the taint the run opened with", created.Labels)
	}
}
