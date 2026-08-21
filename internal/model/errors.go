package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/fuseone/agents/internal/domain"
)

const (
	CodeProviderOverloaded  = "model_provider_overloaded"
	CodeRateLimited         = "model_rate_limited"
	CodeAuthFailed          = "model_auth_failed"
	CodeBadRequest          = "model_bad_request"
	CodeProviderUnavailable = "model_provider_unavailable"
	CodeNetwork             = "model_network"
	CodeRefused             = "model_refused"
	CodeProviderError       = "model_provider_error"
)

// ProviderError is the stable, low-cardinality shape of a model provider
// failure. Its Error string may carry the provider's message for local
// diagnosis, but dashboards and ledgers use Summary instead.
type ProviderError struct {
	Provider  string
	Code      string
	Status    int
	RequestID string
	Retryable bool
	Message   string
	Err       error
}

func (e *ProviderError) Error() string {
	parts := []string{"model", e.Provider, e.Code}
	if e.Status != 0 {
		parts = append(parts, fmt.Sprintf("status %d", e.Status))
	}
	if e.RequestID != "" {
		parts = append(parts, "request "+e.RequestID)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, ": ")
}

func (e *ProviderError) Unwrap() error { return e.Err }

func (e *ProviderError) Summary() domain.FailureSummary {
	return domain.FailureSummary{
		Code:      e.Code,
		Provider:  e.Provider,
		Status:    e.Status,
		RequestID: e.RequestID,
		Retryable: e.Retryable,
	}
}

type failureSummarizer interface {
	Summary() domain.FailureSummary
}

// FailureSummaryOf reads the stable part of a failure through any wrapping
// added by the engine and worker.
func FailureSummaryOf(err error) (domain.FailureSummary, bool) {
	var summarized failureSummarizer
	if !errors.As(err, &summarized) {
		return domain.FailureSummary{}, false
	}
	return summarized.Summary(), true
}

func providerRefused(provider string) error {
	return &ProviderError{
		Provider: provider,
		Code:     CodeRefused,
		Err:      ErrRefused,
	}
}

func classifyAnthropic(provider string, err error) error {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return networkFailure(provider, err)
	}
	code, retryable := classifyStatus(apiErr.StatusCode, string(apiErr.Type()))
	return &ProviderError{
		Provider:  provider,
		Code:      code,
		Status:    apiErr.StatusCode,
		RequestID: apiErr.RequestID,
		Retryable: retryable,
		Message:   messageFrom(err),
		Err:       err,
	}
}

func openAIHTTPFailure(provider string, status int, requestID, body string) error {
	kind, message := openAIErrorBody(body)
	code, retryable := classifyStatus(status, kind)
	return &ProviderError{
		Provider:  provider,
		Code:      code,
		Status:    status,
		RequestID: requestID,
		Retryable: retryable,
		Message:   first(message, trimDiagnostic(body)),
		Err:       errors.New(strings.TrimSpace(body)),
	}
}

func networkFailure(provider string, err error) error {
	return &ProviderError{
		Provider:  provider,
		Code:      CodeNetwork,
		Retryable: true,
		Message:   messageFrom(err),
		Err:       err,
	}
}

func classifyStatus(status int, kind string) (string, bool) {
	switch {
	case kind == "overloaded_error" || status == 529:
		return CodeProviderOverloaded, true
	case kind == "rate_limit_error" || status == http.StatusTooManyRequests:
		return CodeRateLimited, true
	case kind == "authentication_error" || kind == "permission_error" ||
		status == http.StatusUnauthorized || status == http.StatusForbidden:
		return CodeAuthFailed, false
	case kind == "invalid_request_error" || kind == "not_found_error" ||
		status == http.StatusBadRequest || status == http.StatusNotFound:
		return CodeBadRequest, false
	case status == http.StatusRequestTimeout || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout ||
		status >= 500:
		return CodeProviderUnavailable, true
	default:
		return CodeProviderError, false
	}
}

func requestIDFrom(h http.Header) string {
	for _, key := range []string{"x-request-id", "request-id", "openai-request-id"} {
		if v := h.Get(key); v != "" {
			return v
		}
	}
	return ""
}

func openAIErrorBody(body string) (kind, message string) {
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return "", ""
	}
	return first(parsed.Error.Type, parsed.Error.Code), strings.TrimSpace(parsed.Error.Message)
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func messageFrom(err error) string {
	if err == nil {
		return ""
	}
	return trimDiagnostic(err.Error())
}

func trimDiagnostic(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 512 {
		return s
	}
	return s[:512] + "..."
}
