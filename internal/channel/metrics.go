package channel

import "sort"

const (
	MetricOther = "other"

	MetricResultError = "error"
	MetricResultOK    = "ok"

	MetricTaskAnnouncements     = "announcements"
	MetricTaskAnswersDelivered  = "answers_delivered"
	MetricTaskAsksOpened        = "asks_opened"
	MetricTaskRefusalsDelivered = "refusals_delivered"
)

var (
	metricResults = map[string]bool{
		MetricResultError: true,
		MetricResultOK:    true,
	}
	metricTasks = map[string]bool{
		MetricTaskAnnouncements:     true,
		MetricTaskAnswersDelivered:  true,
		MetricTaskAsksOpened:        true,
		MetricTaskRefusalsDelivered: true,
	}
)

// MetricResult bounds a channel sweep result before it can become a metric label.
func MetricResult(result string) string {
	if metricResults[result] {
		return result
	}
	return MetricOther
}

// MetricTask bounds a channel sweep task before it can become a metric label.
func MetricTask(task string) string {
	if metricTasks[task] {
		return task
	}
	return MetricOther
}

// MetricTasks returns the stable task vocabulary used by metrics and UI.
func MetricTasks() []string {
	tasks := make([]string, 0, len(metricTasks)+1)
	for task := range metricTasks {
		tasks = append(tasks, task)
	}
	tasks = append(tasks, MetricOther)
	sort.Strings(tasks)
	return tasks
}
