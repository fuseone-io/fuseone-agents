package slack_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/fuseone/agents/internal/channel/slack"
)

func TestThread_readsMessagesBeforeTheAsk(t *testing.T) {
	t.Parallel()
	var asked url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"messages":[
			{"ts":"1700.0","app_id":"A-alerts","text":"firing alertGatewayRTMInterfaceErrors"},
			{"ts":"1701.0","user":"U-ops","text":"looking"}]}`)
	}))
	defer server.Close()

	got, err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Thread(t.Context(), "C-alerts", "1700.0", "1702.0")
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}

	if asked.Get("channel") != "C-alerts" || asked.Get("ts") != "1700.0" {
		t.Fatalf("query = %s, want the conversation and root thread", asked.Encode())
	}
	if asked.Get("latest") != "1702.0" || asked.Get("inclusive") != "false" {
		t.Fatalf("query = %s, want messages before the ask only", asked.Encode())
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages = %+v, want the earlier thread messages", got.Messages)
	}
	if got.Messages[0].Source != "app:A-alerts" || got.Messages[0].Text == "" {
		t.Errorf("message = %+v, want the Slack source and alert text", got.Messages[0])
	}
}

func TestThread_missingHistoryScope_saysWhichScopeSlackNeeds(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":false,"error":"missing_scope","needed":"channels:history"}`)
	}))
	defer server.Close()

	_, err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Thread(t.Context(), "C-alerts", "1700.0", "1702.0")
	if err == nil {
		t.Fatal("missing history scope was read as an empty thread")
	}
	if got := err.Error(); got != "slack: refused: missing_scope (the app needs channels:history)" {
		t.Fatalf("err = %q", got)
	}
}
