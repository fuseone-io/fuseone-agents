package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
)

func interviewProblem(t *testing.T, err error) (int, openapi.Problem) {
	t.Helper()

	rec := httptest.NewRecorder()
	if err := assistantUnavailable(err).VisitInterviewAgentResponse(rec); err != nil {
		t.Fatalf("write response: %v", err)
	}
	var problem openapi.Problem
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return rec.Code, problem
}

func TestAssistantUnavailable_retryableProviderFailureIsTemporary(t *testing.T) {
	t.Parallel()

	status, problem := interviewProblem(t, fmt.Errorf("authoring: %w", &model.ProviderError{
		Provider:  "anthropic",
		Code:      model.CodeProviderOverloaded,
		Status:    529,
		RequestID: "req_123",
		Retryable: true,
		Message:   "Overloaded",
	}))

	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
	if got := valueOr(problem.Type); got != string(CodeUpstreamBusy) {
		t.Fatalf("type = %q, want %q", got, CodeUpstreamBusy)
	}
	if problem.Title != "Upstream busy" {
		t.Fatalf("title = %q, want Upstream busy", problem.Title)
	}
	if problem.Status != http.StatusServiceUnavailable {
		t.Fatalf("problem status = %d, want 503", problem.Status)
	}
}

func TestAssistantUnavailable_nonRetryableProviderFailureIsBadRequest(t *testing.T) {
	t.Parallel()

	status, problem := interviewProblem(t, &model.ProviderError{
		Provider:  "anthropic",
		Code:      model.CodeAuthFailed,
		Status:    http.StatusUnauthorized,
		Retryable: false,
		Message:   "invalid api key",
	})

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if got := valueOr(problem.Type); got != string(CodeUpstreamRefused) {
		t.Fatalf("type = %q, want %q", got, CodeUpstreamRefused)
	}
	if problem.Status != http.StatusBadRequest {
		t.Fatalf("problem status = %d, want 400", problem.Status)
	}
}

type interviewCompleter struct {
	reply  string
	spent  int64
	called bool
}

func (f *interviewCompleter) Complete(context.Context, string) (model.Completion, error) {
	f.called = true
	return model.Completion{Text: f.reply, Cost: domain.Cost{Micros: f.spent}}, nil
}

type interviewAssistants struct {
	completer model.Completer
}

func (f interviewAssistants) Completer(string, model.Config) (model.Completer, error) {
	return f.completer, nil
}

type interviewAuthoring struct {
	choice authoring.Choice
	err    error
}

func (f interviewAuthoring) Current(context.Context) (authoring.Choice, error) {
	if f.err != nil {
		return authoring.Choice{}, f.err
	}
	return f.choice, nil
}
func (interviewAuthoring) Choose(context.Context, authoring.Choice, domain.UserID) error {
	return nil
}
func (interviewAuthoring) Disable(context.Context, domain.UserID) error { return nil }

type interviewSpend struct {
	today    int64
	recorded domain.Cost
	by       domain.UserID
}

func (f *interviewSpend) SpentToday(context.Context) (int64, error) {
	return f.today, nil
}
func (f *interviewSpend) RecordSpend(_ context.Context, cost domain.Cost, by domain.UserID) error {
	f.recorded, f.by = cost, by
	return nil
}

func TestSuggestInterviewAnswers_returnsFixedFieldsAndRecordsSpend(t *testing.T) {
	t.Parallel()

	completer := &interviewCompleter{
		reply: `{"trigger":"quando chega alerta","mustKnow":"métricas","steps":"ler e resumir","goesWrong":"","notDecide":"acionar alguém","closing":"resumo pronto","neverDo":"fechar incidente"}`,
		spent: 3_700,
	}
	spend := &interviewSpend{}
	server := NewServer(ledger.NewMemory(), "test").
		WithAuthoring(interviewAuthoring{choice: authoring.Choice{
			Provider: "authoring", Model: "model", DailyMicros: 10_000, Enabled: true,
		}}).
		WithAssistants(interviewAssistants{completer: completer}, spend)

	resp, err := server.SuggestInterviewAnswers(
		inArea("devops", domain.RoleAuthor),
		openapi.SuggestInterviewAnswersRequestObject{
			Body: &openapi.SuggestInterviewAnswersJSONRequestBody{
				Text: "quando chega alerta eu leio métricas e escrevo resumo",
			},
		},
	)
	if err != nil {
		t.Fatalf("SuggestInterviewAnswers: %v", err)
	}
	got, ok := resp.(openapi.SuggestInterviewAnswers200JSONResponse)
	if !ok {
		t.Fatalf("got %T", resp)
	}
	if got.Answers.Trigger != "quando chega alerta" || got.Answers.NeverDo != "fechar incidente" {
		t.Fatalf("answers = %+v", got.Answers)
	}
	if !completer.called {
		t.Fatal("the assistant was not called")
	}
	if spend.recorded.Micros != 3_700 || spend.by != "usr_ana" {
		t.Fatalf("recorded spend = %+v by %q", spend.recorded, spend.by)
	}
}

func TestSuggestInterviewAnswers_refusesBlankCaptureBeforeCallingTheAssistant(t *testing.T) {
	t.Parallel()

	completer := &interviewCompleter{reply: `{"steps":"anything"}`}
	server := NewServer(ledger.NewMemory(), "test").
		WithAuthoring(interviewAuthoring{choice: authoring.Choice{
			Provider: "authoring", Model: "model", DailyMicros: 10_000, Enabled: true,
		}}).
		WithAssistants(interviewAssistants{completer: completer}, &interviewSpend{})

	resp, err := server.SuggestInterviewAnswers(
		inArea("devops", domain.RoleAuthor),
		openapi.SuggestInterviewAnswersRequestObject{
			Body: &openapi.SuggestInterviewAnswersJSONRequestBody{Text: "   "},
		},
	)
	if err != nil {
		t.Fatalf("SuggestInterviewAnswers: %v", err)
	}
	if _, ok := resp.(openapi.SuggestInterviewAnswers400ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("got %T, want bad request", resp)
	}
	if completer.called {
		t.Fatal("blank input reached the assistant")
	}
}

func TestSuggestInterviewAnswers_authoringStoreFailureIsNotReportedAsUpstreamRefusal(t *testing.T) {
	t.Parallel()

	boom := errors.New("settings unavailable")
	server := NewServer(ledger.NewMemory(), "test").
		WithAuthoring(interviewAuthoring{err: boom}).
		WithAssistants(interviewAssistants{completer: &interviewCompleter{}}, &interviewSpend{})

	resp, err := server.SuggestInterviewAnswers(
		inArea("devops", domain.RoleAuthor),
		openapi.SuggestInterviewAnswersRequestObject{
			Body: &openapi.SuggestInterviewAnswersJSONRequestBody{Text: "descrevo o trabalho"},
		},
	)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want store failure", err)
	}
	if resp != nil {
		t.Fatalf("resp = %T, want nil", resp)
	}
}
