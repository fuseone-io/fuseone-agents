package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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
	if !retryable {
		if evidence, ok := transientEvidence(anthropicErrorMessage(apiErr), ""); ok {
			code, retryable = evidence, true
		}
	}
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

// anthropicErrorMessage reads the body's own message text, which the SDK's
// Error() buries under request metadata that would defeat a prefix match.
func anthropicErrorMessage(apiErr *anthropic.Error) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(apiErr.RawJSON()), &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Error.Message)
}

func openAIHTTPFailure(provider string, status int, requestID, body string) error {
	kind, bodyCode, message := openAIErrorBody(body)
	code, retryable := classifyStatus(status, kind)
	if !retryable {
		if evidence, ok := transientEvidence(message, bodyCode); ok {
			code, retryable = evidence, true
		}
	}
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

func openAIErrorBody(body string) (kind, code, message string) {
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return "", "", ""
	}
	return first(parsed.Error.Type, parsed.Error.Code), strings.TrimSpace(parsed.Error.Code),
		strings.TrimSpace(parsed.Error.Message)
}

// transientMarkers are the proxy's normalised exception classes that prove a
// failure was transient whatever status carried it. A closed list on purpose:
// reclassifying a permanent verdict upward is only safe on explicit evidence,
// and a generic substring match would eventually promote a genuine bad
// request into an endless retry (NF-14).
var transientMarkers = map[string]string{
	"litellm.Timeout":                 CodeProviderUnavailable,
	"litellm.APIConnectionError":      CodeProviderUnavailable,
	"litellm.ServiceUnavailableError": CodeProviderUnavailable,
	"litellm.InternalServerError":     CodeProviderUnavailable,
	"litellm.RateLimitError":          CodeRateLimited,
}

// transientEvidence reads a non-retryable verdict's evidence for transience:
// a proxy (LiteLLM, most commonly) sometimes wraps a timeout or an upstream
// outage in a 400, and the body is then the only witness. The message names
// the proxy's exception class at its start; the body's code field may carry
// the upstream status the wire status hid.
func transientEvidence(message, bodyCode string) (string, bool) {
	for marker, code := range transientMarkers {
		if strings.HasPrefix(message, marker+":") || message == marker {
			return code, true
		}
	}
	// The code field usually carries the upstream HTTP status, but it may be
	// a provider's own numeric code instead; outside the HTTP range it is
	// noise, never evidence.
	if status, err := strconv.Atoi(bodyCode); err == nil && status >= 100 && status <= 599 {
		if code, retryable := classifyStatus(status, ""); retryable {
			return code, true
		}
	}
	return "", false
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
