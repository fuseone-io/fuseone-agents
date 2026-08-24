package channel

import (
	"errors"

	"github.com/fuseone/agents/internal/domain"
)

const (
	CodeConfigurationReadFailed = "channel_configuration_read_failed"
	CodeConnectionDisabled      = "channel_connection_disabled"
	CodeInvalidConfiguration    = "channel_invalid_configuration"
	CodeMissingCredential       = "channel_missing_credential"
	CodeUnsupportedKind         = "channel_unsupported_kind"
	CodeDeliveryFailed          = "channel_delivery_failed"
	CodeCredentialRejected      = "channel_credential_rejected"
	CodeConversationUnavailable = "channel_conversation_unavailable"
	CodeMissingScope            = "channel_missing_scope"
	CodeRateLimited             = "channel_rate_limited"
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
		Code: MetricCode(e.Code),
	}
}
