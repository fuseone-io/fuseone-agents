package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/worker"
)

func TestRecordChannelSweep_countsEveryJoinedFailureCode(t *testing.T) {
	t.Parallel()

	metrics := worker.NewMetricsRegistry()
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

	recordChannelSweep(metrics, channel.MetricTaskAnnouncements, 0, err)

	rec := httptest.NewRecorder()
	metrics.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`fuseone_channel_sweeps_total{task="announcements",result="error"} 1`,
		`fuseone_channel_failures_total{task="announcements",code="channel_delivery_failed"} 1`,
		`fuseone_channel_failures_total{task="announcements",code="channel_missing_scope"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}
