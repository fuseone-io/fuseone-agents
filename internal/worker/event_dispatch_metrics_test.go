package worker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var observedAt = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func scrape(t *testing.T, reg *MetricsRegistry) string {
	t.Helper()
	rec := httptest.NewRecorder()
	reg.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

func requireSamples(t *testing.T, body string, want []string) {
	t.Helper()
	for _, line := range want {
		if !strings.Contains(body, line+"\n") {
			t.Errorf("missing %q in\n%s", line, body)
		}
	}
}

func TestEventDispatchMetrics_aHeldBacklog_rendersLagHolderAndDegradation(t *testing.T) {
	t.Parallel()
	reg := NewMetricsRegistry()
	reg.EventDispatch(domain.EventDispatchBacklog{
		Ready:            2,
		HeldByXmin:       5,
		OldestFinishedAt: observedAt.Add(-6 * time.Minute),
		Xact:             domain.OpenXact{Open: true, StartedAt: observedAt.Add(-7 * time.Minute)},
		Capped:           true,
	}, observedAt)

	requireSamples(t, scrape(t, reg), []string{
		`fuseone_event_dispatch_backlog_runs{state="ready"} 2`,
		`fuseone_event_dispatch_backlog_runs{state="held_by_xmin"} 5`,
		`fuseone_event_dispatch_backlog_capped 1`,
		`fuseone_event_dispatch_lag_seconds 360`,
		`fuseone_event_dispatch_held_by_xmin 1`,
		`fuseone_event_dispatch_oldest_xact_age_seconds 420`,
		`fuseone_event_dispatch_oldest_xact_age_known 1`,
		`fuseone_event_dispatch_degraded 1`,
		`fuseone_event_dispatch_observed_timestamp_seconds 1793534400`,
	})
}

func TestEventDispatchMetrics_caughtUp_isHealthyAndHoldsNothing(t *testing.T) {
	t.Parallel()
	reg := NewMetricsRegistry()
	reg.EventDispatch(domain.EventDispatchBacklog{}, observedAt)

	requireSamples(t, scrape(t, reg), []string{
		`fuseone_event_dispatch_backlog_runs{state="ready"} 0`,
		`fuseone_event_dispatch_backlog_runs{state="held_by_xmin"} 0`,
		`fuseone_event_dispatch_backlog_capped 0`,
		`fuseone_event_dispatch_lag_seconds 0`,
		`fuseone_event_dispatch_held_by_xmin 0`,
		`fuseone_event_dispatch_oldest_xact_age_seconds 0`,
		`fuseone_event_dispatch_degraded 0`,
	})
}

// A holder whose start the role cannot see has no age to report: a zero would
// read as "nothing open", which is the one thing that is not true.
func TestEventDispatchMetrics_aHiddenHolder_omitsTheAgeAndSaysItIsUnknown(t *testing.T) {
	t.Parallel()
	reg := NewMetricsRegistry()
	reg.EventDispatch(domain.EventDispatchBacklog{
		HeldByXmin: 1, OldestFinishedAt: observedAt.Add(-time.Minute),
		Xact: domain.OpenXact{Open: true},
	}, observedAt)

	body := scrape(t, reg)
	requireSamples(t, body, []string{`fuseone_event_dispatch_oldest_xact_age_known 0`})
	if strings.Contains(body, "\nfuseone_event_dispatch_oldest_xact_age_seconds ") {
		t.Fatalf("an unknown age was rendered as a number:\n%s", body)
	}
}

// Before the first observation there is nothing to say: a zero lag would be
// a claim that dispatch is caught up, made by a worker that never looked.
func TestEventDispatchMetrics_neverObserved_rendersNoGauges(t *testing.T) {
	t.Parallel()
	reg := NewMetricsRegistry()
	reg.EventDispatchUnobserved()

	body := scrape(t, reg)
	requireSamples(t, body, []string{`fuseone_event_dispatch_observe_failures_total 1`})
	for _, gauge := range []string{"\nfuseone_event_dispatch_lag_seconds ", "\nfuseone_event_dispatch_degraded "} {
		if strings.Contains(body, gauge) {
			t.Fatalf("%q rendered before any observation:\n%s", gauge, body)
		}
	}
}
