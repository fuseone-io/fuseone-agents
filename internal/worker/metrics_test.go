package worker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRegistry_rendersOnlyLowCardinalityWorkerFacts(t *testing.T) {
	t.Parallel()

	reg := NewMetricsRegistry()
	pool := reg.Pool("runs", 4)
	pool.Claim("claimed")
	pool.Advance("parked")
	pool.AdvanceFailure("model_provider_overloaded", true)
	pool.Park("model_provider_overloaded")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	reg.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{
		`fuseone_worker_slots{pool="runs"} 4`,
		`fuseone_worker_claims_total{pool="runs",result="claimed"} 1`,
		`fuseone_worker_advances_total{pool="runs",phase="parked"} 1`,
		`fuseone_worker_advance_failures_total{pool="runs",reason="model_provider_overloaded",parked="true"} 1`,
		`fuseone_worker_parks_total{pool="runs",reason="model_provider_overloaded"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"run_", "agent=", "tool=", "err="} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics body contains high-cardinality or diagnostic text %q:\n%s", forbidden, body)
		}
	}
}

func TestMetricsRegistry_onlyServesMetricsPath(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	NewMetricsRegistry().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
