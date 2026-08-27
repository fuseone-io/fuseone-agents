package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
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
/*
The agent, the counters and the expiry are the platform's, not the caller's.

Each of them changes what a memory means: which runs may recall it, how it ranks
against its neighbours, and how long it lasts. A caller that could assert them
about itself could write a memory that outranks, outlives and reaches further
than the ones the platform derived — without any of that showing up as a
permission it was refused.

The agent in particular comes from the run the evidence names. There is always
one, because evidence is required and names a run, so an agent-scoped memory
cannot be filed against an agent whose run never produced it.
*/
func TestCreateMemoryAssertion_platformOwnsAgentCountersAndExpiry(t *testing.T) {
	t.Parallel()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	led := ledger.NewMemory()
	seedFinishedEvidence(t, led, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")

	at := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	resp, err := NewServer(led, "test").WithMemory(memstore.NewMemory()).
		WithClock(fixedAt{t: at}).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), memoryCreateRequest("sha256:answer"))
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	created, ok := resp.(openapi.CreateMemoryAssertion200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the assertion", resp)
	}

	// seedFinishedEvidence opens the run for "triage"; nothing in the request
	// says so, and the memory is filed against it anyway.
	if created.AgentId != "triage" {
		t.Errorf("agent = %q, want the one whose run the evidence names", created.AgentId)
	}
	if created.Observations != 1 || created.Confirmed != 1 {
		t.Errorf("counters = %d/%d, want a first sighting", created.Observations, created.Confirmed)
	}
	if created.ExpiresAt == nil || !created.ExpiresAt.Equal(at.Add(memstore.DefaultMemoryTTL)) {
		t.Errorf("expiry = %v, want thirty days from the decision", created.ExpiresAt)
	}
}

/*
Shared memory is chosen, never arrived at by leaving a field blank.

It is what every agent in the scope reads, so the difference between "for this
agent" and "for everybody" cannot be the difference between filling something in
and forgetting to. The field is required and has two values; there is no third
meaning nothing.
*/
func TestCreateMemoryAssertion_sharedIsAnExplicitChoice(t *testing.T) {
	t.Parallel()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	led := ledger.NewMemory()
	seedFinishedEvidence(t, led, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")

	req := memoryCreateRequest("sha256:answer")
	req.Body.Namespace = openapi.MemoryAssertionInputNamespaceShared
	resp, err := NewServer(led, "test").WithMemory(memstore.NewMemory()).
		WithClock(fixedAt{t: time.Unix(0, 0)}).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	created, ok := resp.(openapi.CreateMemoryAssertion200JSONResponse)
	if !ok {
		t.Fatalf("response = %T, want the assertion", resp)
	}
	if created.AgentId != "" {
		t.Errorf("agent = %q, want shared memory to belong to none of them", created.AgentId)
	}
}

/*
Evidence from two agents' runs is refused rather than attributed to one.

An agent-scoped memory belongs to one agent. Taking whichever run came first
would decide that silently, and the memory would be recalled by an agent whose
run only half explains it.
*/
func TestCreateMemoryAssertion_evidenceFromTwoAgents_isRefused(t *testing.T) {
	t.Parallel()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	led := ledger.NewMemory()
	seedFinishedEvidence(t, led, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")
	seedFinishedEvidenceFor(t, led, "run-other", scope, "billing", "sha256:other")

	req := memoryCreateRequest("sha256:answer")
	req.Body.Evidence = append(req.Body.Evidence, openapi.MemoryEvidence{
		RunId: "run-other", Artifact: domain.ArtifactFinalAnswer, Digest: "sha256:other",
	})
	resp, err := NewServer(led, "test").WithMemory(memstore.NewMemory()).
		WithClock(fixedAt{t: time.Unix(0, 0)}).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	if _, bad := resp.(openapi.CreateMemoryAssertion400ApplicationProblemPlusJSONResponse); !bad {
		t.Fatalf("response = %T, want the ambiguous attribution refused", resp)
	}
}

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

func (unavailableMemory) Reactivate(
	context.Context, *memstore.Resolver, memstore.ReactivateInput,
) (domain.MemoryAssertion, error) {
	return domain.MemoryAssertion{}, errMemoryUnreachable
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

func seedFinishedEvidenceFor(
	t *testing.T, store *ledger.Memory, run domain.RunID, scope domain.Scope,
	agent domain.AgentID, digest string,
) {
	t.Helper()
	appendMemoryStep(t, store, domain.Step{RunID: run, Kind: domain.StepRunStarted,
		Scope: scope, AgentID: agent, VersionID: "v1", Labels: domain.ScopeLabels(scope)})
	appendMemoryStep(t, store, domain.Step{RunID: run, Kind: domain.StepRunFinished,
		Scope: scope, AgentID: agent, VersionID: "v1", Labels: domain.ScopeLabels(scope),
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
		Company: "acme", Area: "cx", Kind: "incident",
		Namespace: openapi.MemoryAssertionInputNamespaceAgent,
		Subject:   "grafana datasource", Signature: "grafana.datasource.down",
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

/*
Reactivation is refused when the platform cannot prove the memory.

The endpoint answers with the state that refused it rather than with "invalid
input", which is the difference between a person knowing to go and look at the
shared memory and a person retyping a form that was never wrong. And an
installation with no evidence resolver refuses outright: reactivating without
checking the citations is the thing this act exists to prevent.
*/
func TestReactivateMemoryAssertion_answersWithTheStateThatRefusedIt(t *testing.T) {
	t.Parallel()

	t.Run("a memory that is not disabled is a conflict", func(t *testing.T) {
		t.Parallel()
		store := memstore.NewMemory()
		stored := memoryAssertionFixture("cx", "grafana datasource", nil)
		remember(t, store, stored)

		resp := reactivateAgainst(t, store, domain.MemoryAssertionID(stored), "it is true again")
		if _, conflict := resp.(openapi.ReactivateMemoryAssertion409ApplicationProblemPlusJSONResponse); !conflict {
			t.Fatalf("response = %T, want 409", resp)
		}
	})

	t.Run("a memory nobody has is not found", func(t *testing.T) {
		t.Parallel()
		resp := reactivateAgainst(t, memstore.NewMemory(), "mem_absent", "it is true again")
		if _, missing := resp.(openapi.ReactivateMemoryAssertion404ApplicationProblemPlusJSONResponse); !missing {
			t.Fatalf("response = %T, want 404", resp)
		}
	})

	t.Run("no reason is a bad request", func(t *testing.T) {
		t.Parallel()
		store := memstore.NewMemory()
		stored := memoryAssertionFixture("cx", "grafana datasource", nil)
		remember(t, store, stored)

		resp := reactivateAgainst(t, store, domain.MemoryAssertionID(stored), "  ")
		if _, bad := resp.(openapi.ReactivateMemoryAssertion400ApplicationProblemPlusJSONResponse); !bad {
			t.Fatalf("response = %T, want 400", resp)
		}
	})

	t.Run("an installation with no resolver refuses rather than skipping the proof", func(t *testing.T) {
		t.Parallel()
		resp, err := NewServer(ledger.NewMemory(), "test").WithMemory(memstore.NewMemory()).
			ReactivateMemoryAssertion(inArea("cx", domain.RoleAuthor),
				openapi.ReactivateMemoryAssertionRequestObject{
					AssertionId: "mem_whatever",
					Body: &openapi.MemoryReactivateInput{
						Company: "acme", Area: "cx", Reason: "it is true again",
					},
				})
		if err == nil {
			t.Fatalf("response = %T, want the missing resolver to refuse", resp)
		}
	})
}

/*
Every state the store can refuse with reaches the caller as a conflict.

Named one by one rather than through a scenario each, because what is under test
is the mapping: a sentinel added to the store and forgotten here would come back
as "invalid input", and the console would tell somebody to check a form that was
never the problem. Only the endpoint's own body validation is a 400.
*/
func TestReactivateMemoryAssertion_everyRefusedStateIsAConflict(t *testing.T) {
	t.Parallel()
	for _, refusal := range []struct {
		name string
		err  error
	}{
		{"not disabled", memstore.ErrMemoryTerminal},
		{"the citations no longer prove it", memstore.ErrEvidenceInvalid},
		{"shared memory answers it", memstore.ErrCovered},
		{"somebody moved it meanwhile", memstore.ErrMovedMeanwhile},
		{"two rows are this identity", memstore.ErrCanonicalConflict},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			t.Parallel()
			resp := reactivateAgainst(t, refusingMemory{err: refusal.err},
				"mem_whatever", "it is true again")
			if _, conflict := resp.(openapi.ReactivateMemoryAssertion409ApplicationProblemPlusJSONResponse); !conflict {
				t.Fatalf("response = %T, want %v answered as 409", resp, refusal.err)
			}
		})
	}
}

// refusingMemory answers one chosen refusal, so the mapping can be asked about
// each state without a scenario that produces it.
type refusingMemory struct {
	unavailableMemory
	err error
}

func (m refusingMemory) Reactivate(
	context.Context, *memstore.Resolver, memstore.ReactivateInput,
) (domain.MemoryAssertion, error) {
	return domain.MemoryAssertion{}, fmt.Errorf("%w: for the test", m.err)
}

func reactivateAgainst(
	t *testing.T, store Memory, id, reason string,
) openapi.ReactivateMemoryAssertionResponseObject {
	t.Helper()
	led := ledger.NewMemory()
	resp, err := NewServer(led, "test").WithMemory(store).
		WithMemoryEvidence(led, engine.NewMemoryContent()).
		WithClock(fixedAt{t: time.Unix(0, 0)}).
		ReactivateMemoryAssertion(inArea("cx", domain.RoleAuthor),
			openapi.ReactivateMemoryAssertionRequestObject{
				AssertionId: id,
				Body: &openapi.MemoryReactivateInput{
					Company: "acme", Area: "cx", Reason: reason,
				},
			})
	if err != nil {
		t.Fatalf("ReactivateMemoryAssertion: %v", err)
	}
	return resp
}
