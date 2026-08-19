package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/httpapi/openapi"
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
	if got := valueOr(problem.Type); got != string(CodeUpstreamRefused) {
		t.Fatalf("type = %q, want %q", got, CodeUpstreamRefused)
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
