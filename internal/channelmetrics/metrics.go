package channelmetrics

import "sort"

const (
	CodeOther = "other"
	CodeNone  = "none"

	ResultError = "error"
	ResultOK    = "ok"

	TaskAnnouncements = "announcements"
	// TaskCardsClosed counts approval cards rewritten because the question
	// they asked has an answer.
	TaskCardsClosed       = "cards_closed"
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
	// CodeTooManyRecipients means an approval reached more people who may
	// decide it than one announcement should ever be sent to, so it was sent
	// to none of them privately and the conversation heard alone.
	CodeTooManyRecipients = "channel_too_many_recipients"
	// CodeUnsupportedCapability means a driver was asked for something it
	// cannot do — rewriting a message it posted, today.
	CodeUnsupportedCapability = "channel_unsupported_capability"
	// CodeNoConnectionChosen means an agent asked for its approvals privately
	// and nothing said which workspace the bot should send them from. With no
	// conversation to name one, and more than one connection to choose
	// between, guessing would send one company's run into another company's
	// Slack — so nobody is told and the reason is recorded.
	CodeNoConnectionChosen = "channel_no_connection_chosen"
	// CodeNamedNobodyWhoDecides means an agent named the people to message and
	// none of them may decide in the run's scope. Addressing is not
	// authorising: a message to somebody the button will refuse is a message
	// that wastes their time and tells the owner nothing.
	CodeNamedNobodyWhoDecides = "channel_named_nobody_who_decides"
	// CodeNobodyMayDecide means an agent asked for its approvals privately and
	// nobody holds Approver in a scope covering the run. Nobody can be told,
	// and the fix is a grant.
	CodeNobodyMayDecide = "channel_nobody_may_decide"
	// CodeNobodyReachable means the people who may decide are known and none of
	// them has linked a channel account. Nobody can be told, and the fix is a
	// binding — a different screen and usually a different person.
	CodeNobodyReachable = "channel_nobody_reachable"
)

var (
	results = map[string]bool{
		ResultError: true,
		ResultOK:    true,
	}
	tasks = map[string]bool{
		TaskAnnouncements:     true,
		TaskCardsClosed:       true,
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
		CodeTooManyRecipients:       true,
		CodeUnsupportedCapability:   true,
		CodeNoConnectionChosen:      true,
		CodeNamedNobodyWhoDecides:   true,
		CodeNobodyMayDecide:         true,
		CodeNobodyReachable:         true,
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
