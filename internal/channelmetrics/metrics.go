package channelmetrics

import "sort"

const (
	CodeOther = "other"
	CodeNone  = "none"

	ResultError = "error"
	ResultOK    = "ok"

	TaskAnnouncements     = "announcements"
	TaskAnswersDelivered  = "answers_delivered"
	TaskAsksOpened        = "asks_opened"
	TaskRefusalsDelivered = "refusals_delivered"

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

var (
	results = map[string]bool{
		ResultError: true,
		ResultOK:    true,
	}
	tasks = map[string]bool{
		TaskAnnouncements:     true,
		TaskAnswersDelivered:  true,
		TaskAsksOpened:        true,
		TaskRefusalsDelivered: true,
	}
	codes = map[string]bool{
		CodeNone:                    true,
		CodeConfigurationReadFailed: true,
		CodeConnectionDisabled:      true,
		CodeInvalidConfiguration:    true,
		CodeMissingCredential:       true,
		CodeUnsupportedKind:         true,
		CodeDeliveryFailed:          true,
		CodeCredentialRejected:      true,
		CodeConversationUnavailable: true,
		CodeMissingScope:            true,
		CodeRateLimited:             true,
	}
)

// Result bounds a channel sweep result before it can become a metric label.
func Result(result string) string {
	if results[result] {
		return result
	}
	return CodeOther
}

// Task bounds a channel sweep task before it can become a metric label.
func Task(task string) string {
	if tasks[task] {
		return task
	}
	return CodeOther
}

// Code bounds a channel failure code before it can become a metric label or UI bucket.
func Code(code string) string {
	if codes[code] {
		return code
	}
	return CodeOther
}

// Tasks returns the stable task vocabulary used by metrics and runtime views.
func Tasks() []string {
	out := make([]string, 0, len(tasks)+1)
	for task := range tasks {
		out = append(out, task)
	}
	out = append(out, CodeOther)
	sort.Strings(out)
	return out
}

// Codes returns the stable channel failure vocabulary used by metrics and runtime views.
func Codes() []string {
	out := make([]string, 0, len(codes)+1)
	for code := range codes {
		out = append(out, code)
	}
	out = append(out, CodeOther)
	sort.Strings(out)
	return out
}
