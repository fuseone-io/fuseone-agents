package channel

import "github.com/fuseone/agents/internal/channelmetrics"

const (
	MetricOther = channelmetrics.CodeOther
	MetricNone  = channelmetrics.CodeNone

	MetricResultError = channelmetrics.ResultError
	MetricResultOK    = channelmetrics.ResultOK

	MetricTaskAnnouncements     = channelmetrics.TaskAnnouncements
	MetricTaskAnswersDelivered  = channelmetrics.TaskAnswersDelivered
	MetricTaskAsksOpened        = channelmetrics.TaskAsksOpened
	MetricTaskRefusalsDelivered = channelmetrics.TaskRefusalsDelivered
)

// MetricResult bounds a channel sweep result before it can become a metric label.
func MetricResult(result string) string { return channelmetrics.Result(result) }

// MetricTask bounds a channel sweep task before it can become a metric label.
func MetricTask(task string) string { return channelmetrics.Task(task) }

// MetricCode bounds a channel failure code before it can become a metric label.
func MetricCode(code string) string { return channelmetrics.Code(code) }

// MetricTasks returns the stable task vocabulary used by metrics and UI.
func MetricTasks() []string { return channelmetrics.Tasks() }

// MetricCodes returns the stable channel failure vocabulary used by metrics
// and runtime views.
func MetricCodes() []string { return channelmetrics.Codes() }
