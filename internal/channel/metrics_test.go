package channel_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/channel"
)

func TestMetricTask_boundsUnknownTasks(t *testing.T) {
	if got := channel.MetricTask(channel.MetricTaskAnswersDelivered); got != channel.MetricTaskAnswersDelivered {
		t.Fatalf("known task = %q", got)
	}
	if got := channel.MetricTask("slack-team-alerts"); got != channel.MetricOther {
		t.Fatalf("unknown task = %q, want other", got)
	}
}

func TestMetricResult_boundsUnknownResults(t *testing.T) {
	if got := channel.MetricResult(channel.MetricResultError); got != channel.MetricResultError {
		t.Fatalf("known result = %q", got)
	}
	if got := channel.MetricResult("socket_reconnect_failed"); got != channel.MetricOther {
		t.Fatalf("unknown result = %q, want other", got)
	}
}

func TestMetricTasks_exposesTheBoundedVocabulary(t *testing.T) {
	tasks := channel.MetricTasks()
	for _, want := range []string{
		channel.MetricOther,
		channel.MetricTaskAnnouncements,
		channel.MetricTaskAnswersDelivered,
	} {
		if !slices.Contains(tasks, want) {
			t.Fatalf("tasks %v missing %q", tasks, want)
		}
	}
}
