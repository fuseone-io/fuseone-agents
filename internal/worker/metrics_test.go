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
	reg.MCPToolCall("error", "mcp_personal_credential_missing", false)
	reg.MCPToolCall("ok", "cache_hit", true)
	reg.MCPToolCall("error", "github-mcp.create_issue", false)
	reg.MCPReservationRefused("mcp_server_rate_limited")
	reg.MCPReservationRefused("jira-prod.transition_ACME-4417")
	reg.ChannelSweep("answers_delivered", "error", 2)
	reg.ChannelSweep("announcements", "ok", 3)
	reg.ChannelSweep("slack-team-alerts", "error", 1)

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
		`fuseone_mcp_tool_calls_total{result="error",code="mcp_personal_credential_missing",cached="false"} 1`,
		`fuseone_mcp_tool_calls_total{result="error",code="other",cached="false"} 1`,
		`fuseone_mcp_tool_calls_total{result="ok",code="cache_hit",cached="true"} 1`,
		`fuseone_mcp_reservation_refusals_total{code="other"} 1`,
		`fuseone_mcp_reservation_refusals_total{code="mcp_server_rate_limited"} 1`,
		`fuseone_channel_sweeps_total{task="announcements",result="ok"} 1`,
		`fuseone_channel_sweeps_total{task="answers_delivered",result="error"} 1`,
		`fuseone_channel_sweeps_total{task="other",result="error"} 1`,
		`fuseone_channel_items_total{task="announcements"} 3`,
		`fuseone_channel_items_total{task="answers_delivered"} 2`,
		`fuseone_channel_items_total{task="other"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		"run_", "agent=", "tool=", "server=", "channel=", "conversation=", "usr_", "err=",
		"github-mcp", "ACME-4417", "jira-prod", "slack-team-alerts",
	} {
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
