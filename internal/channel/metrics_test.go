package channel_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
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

func TestMetricCode_boundsUnknownCodes(t *testing.T) {
	if got := channel.MetricCode(channel.CodeMissingCredential); got != channel.CodeMissingCredential {
		t.Fatalf("known code = %q", got)
	}
	if got := channel.MetricCode("slack-team-alerts"); got != channel.MetricOther {
		t.Fatalf("unknown code = %q, want other", got)
	}
}

func TestChannelErrorSummary_marksOnlyRateLimitRetryable(t *testing.T) {
	type summarized interface {
		Summary() domain.FailureSummary
	}
	rateLimit := channel.NewError(channel.CodeRateLimited, "slack: rate limited").(summarized)
	if !rateLimit.Summary().Retryable {
		t.Fatal("rate-limit summary is not retryable")
	}
	missingScope := channel.NewError(channel.CodeMissingScope, "slack: missing scope").(summarized)
	if missingScope.Summary().Retryable {
		t.Fatal("missing-scope summary is retryable")
	}
}

func TestFailureCodes_readsEveryJoinedChannelFailure(t *testing.T) {
	err := errors.Join(
		fmt.Errorf("post to first: %w", channel.NewError(
			channel.CodeDeliveryFailed, "slack: refused: unknown_error",
		)),
		fmt.Errorf("post to second: %w", channel.NewError(
			channel.CodeMissingScope, "slack: refused: missing_scope",
		)),
		fmt.Errorf("post to third: %w", channel.NewError(
			channel.CodeMissingScope, "slack: refused: missing_scope",
		)),
	)

	got := channel.FailureCodes(err)
	want := []string{
		channel.CodeDeliveryFailed,
		channel.CodeMissingScope,
		channel.CodeMissingScope,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("FailureCodes = %v, want %v", got, want)
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

func TestMetricCodes_exposesTheBoundedVocabulary(t *testing.T) {
	codes := channel.MetricCodes()
	for _, want := range []string{
		channel.MetricOther,
		channel.CodeDeliveryFailed,
		channel.CodeMissingScope,
		channel.CodeRateLimited,
	} {
		if !slices.Contains(codes, want) {
			t.Fatalf("codes %v missing %q", codes, want)
		}
	}
}
