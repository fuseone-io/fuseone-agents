package channel

import (
	"errors"

	"github.com/fuseone/agents/internal/channelmetrics"
	"github.com/fuseone/agents/internal/domain"
)

const (
	CodeConfigurationReadFailed = channelmetrics.CodeConfigurationReadFailed
	CodeConnectionDisabled      = channelmetrics.CodeConnectionDisabled
	CodeInvalidConfiguration    = channelmetrics.CodeInvalidConfiguration
	CodeMissingCredential       = channelmetrics.CodeMissingCredential
	CodeUnsupportedKind         = channelmetrics.CodeUnsupportedKind
	CodeDeliveryFailed          = channelmetrics.CodeDeliveryFailed
	CodeCredentialRejected      = channelmetrics.CodeCredentialRejected
	CodeConversationUnavailable = channelmetrics.CodeConversationUnavailable
	CodeMissingScope            = channelmetrics.CodeMissingScope
	CodeRateLimited             = channelmetrics.CodeRateLimited
	CodeTooManyRecipients       = channelmetrics.CodeTooManyRecipients
	CodeUnsupportedCapability   = channelmetrics.CodeUnsupportedCapability
)

// Error carries the stable reason a channel operation failed. The wrapped text
// remains for logs; metrics and payloads read only Summary, never vendor text.
type Error struct {
	Code string
	Err  error
}

func NewError(code, message string) error {
	return &Error{Code: code, Err: errors.New(message)}
}

func WrapError(code string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Code: code, Err: err}
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) Summary() domain.FailureSummary {
	return domain.FailureSummary{
		Code:      MetricCode(e.Code),
		Retryable: e.Code == CodeRateLimited,
	}
}

type failureSummarizer interface {
	Summary() domain.FailureSummary
}

// FailureCodes returns every stable channel code inside err. A sweep can join
// one failure per conversation; picking the first would hide the rest forever
// when the same conversation sorts first on every pass.
func FailureCodes(err error) []string {
	var out []string
	collectFailureCodes(err, &out)
	if len(out) == 0 {
		return []string{MetricOther}
	}
	return out
}

func collectFailureCodes(err error, out *[]string) {
	if err == nil {
		return
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, one := range joined.Unwrap() {
			collectFailureCodes(one, out)
		}
		return
	}
	if summarized, ok := err.(failureSummarizer); ok {
		*out = append(*out, MetricCode(summarized.Summary().Code))
		return
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		collectFailureCodes(wrapped.Unwrap(), out)
		return
	}
	*out = append(*out, MetricOther)
}

// ErrUnaddressed means a delivery named no place.
//
// Refused rather than stored, and refused for either half. A row with both
// empty is the shape a run reported everywhere is filed under, so storing one
// retires the run from the sweep and every real conversation loses the
// message. A row with only the conversation empty claims somebody was told
// and suppresses the retry that would have told them.
var ErrUnaddressed = errors.New("channel: a delivery must name a connection and a conversation")
