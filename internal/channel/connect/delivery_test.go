package connect

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
)

/*
Which connections may carry a button.

A button is an answer coming back, and an answer is only worth taking from a
connection whose requests can be verified — the signing secret on the HTTP
path. Chosen by "anything that is not socket", a delivery mode this version
does not know took that path: a connection restored from a newer version would
offer decisions and accept them, checked by whichever secret happened to be
sealed beside it.
*/
func TestDrivers_onlyTheHTTPPathOffersDecisions(t *testing.T) {
	for _, one := range []struct {
		name  string
		mode  string
		wants bool
	}{
		{"http", channel.DeliveryHTTP, true},
		{"a row from before socket mode existed", "", true},
		{"socket", channel.DeliverySocket, false},
		{"a mode this version does not know", "a-future-mode", false},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			body := ""
			server := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					raw, _ := io.ReadAll(r.Body)
					body = string(raw)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"ok":true,"ts":"1.1"}`)
				}))
			t.Cleanup(server.Close)

			driver := drivers["slack"](
				channel.Connection{Kind: "slack", DeliveryMode: one.mode},
				channel.Credentials{Token: "xoxb-test", Signing: "signing"},
			)
			poster, ok := driver.(*slack.Poster)
			if !ok {
				t.Fatalf("driver = %T, want the slack poster", driver)
			}
			if _, err := poster.WithEndpointBase(server.URL).Post(t.Context(),
				channel.Conversation{ID: "C07"}, channel.Message{
					Event: channel.EventParked, RunID: "run-1", Agent: "triage",
					Tool: "crm.reply", AtSeq: 12, AwaitingDecision: true,
				}); err != nil {
				t.Fatalf("post: %v", err)
			}

			if got := strings.Contains(body, `"type":"actions"`); got != one.wants {
				t.Errorf("offers decisions = %v, want %v", got, one.wants)
			}
		})
	}
}
