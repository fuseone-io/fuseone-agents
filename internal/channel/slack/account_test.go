package slack_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fuseone/agents/internal/channel/slack"
)

func TestAccount_readsTheNameAndEmailSlackKnows(t *testing.T) {
	t.Parallel()
	var asked, auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked, auth = r.URL.Path+"?"+r.URL.RawQuery, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"user":{"id":"U09","name":"ana",
			"profile":{"real_name":"Ana Souza","display_name":"Ana","email":"ana@example.com"}}}`)
	}))
	defer server.Close()

	got, err := slack.New("xoxb-test").WithEndpointBase(server.URL).Account(t.Context(), "U09")
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if asked != "/users.info?user=U09" || auth != "Bearer xoxb-test" {
		t.Fatalf("asked %q with %q", asked, auth)
	}
	if got.Name != "Ana Souza" || got.Email != "ana@example.com" {
		t.Errorf("account = %+v", got)
	}
}

func TestAccount_refusesWhatCannotBeSomebody(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"missing scope": `{"ok":false,"error":"missing_scope","needed":"users:read"}`,
		"deactivated":   `{"ok":true,"user":{"id":"U09","deleted":true,"profile":{"email":"a@b.c"}}}`,
		"a bot":         `{"ok":true,"user":{"id":"U09","is_bot":true,"profile":{"real_name":"Triage"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()

			got, err := slack.New("xoxb-test").WithEndpointBase(server.URL).Account(t.Context(), "U09")
			if err == nil {
				t.Fatalf("account = %+v, want a refusal", got)
			}
		})
	}
}
