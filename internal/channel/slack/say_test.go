package slack_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel/slack"
)

/*
Saying something back, which is not the same job as posting a report.

A report is composed here from a run's facts: blocks, a link, sometimes
buttons. A reply is a sentence somebody is owed — why their ask will not start
— and it belongs in the thread they asked in, or it reads as the bot talking to
the room about them.
*/

type said struct {
	Channel string `json:"channel"`
	Thread  string `json:"thread_ts"`
	Text    string `json:"text"`
	Blocks  []any  `json:"blocks"`
}

func sink(t *testing.T, answer string) (*httptest.Server, *said) {
	t.Helper()
	var got said
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)
	return server, &got
}

func TestSay_aRefusal_landsInTheThreadItWasAskedIn(t *testing.T) {
	t.Parallel()
	server, got := sink(t, `{"ok":true,"ts":"1786.2"}`)

	err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Say(t.Context(), "C07", "1786.1", "triage will not start: it is paused.")
	if err != nil {
		t.Fatalf("say: %v", err)
	}

	if got.Channel != "C07" || got.Thread != "1786.1" {
		t.Errorf("said in %s/%s, want the conversation and thread it was asked in",
			got.Channel, got.Thread)
	}
	if got.Text != "triage will not start: it is paused." {
		t.Errorf("text = %q", got.Text)
	}
	if len(got.Blocks) != 0 {
		t.Errorf("blocks = %v; a sentence carries no buttons, and a button here would decide nothing", got.Blocks)
	}
}

/*
A refusal quotes what somebody typed, and Slack reads what it is given.

`<!channel>` in message text is a broadcast, not characters: everybody in the
conversation is notified. The person who typed it may hold no such permission —
the bot does, and the bot is what speaks here. So a mention of an agent nobody
published comes back as a refusal naming the word that was typed, and the
refusal pings the room.

Escaped at this edge for the same reason blocks are composed at this edge: it
is the only place that knows what the vendor treats as markup. The platform's
own sentence must not be able to act.
*/
func TestSay_aRefusalQuotingWhatSomebodyTyped_cannotBroadcastToTheChannel(t *testing.T) {
	t.Parallel()
	server, got := sink(t, `{"ok":true,"ts":"1786.2"}`)

	err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Say(t.Context(), "C07", "1786.1", `no agent named "<!channel>" & "<@U01>"`)
	if err != nil {
		t.Fatalf("say: %v", err)
	}

	for _, live := range []string{"<!channel>", "<@U01>"} {
		if strings.Contains(got.Text, live) {
			t.Errorf("text = %q; it still carries %s, which Slack will act on", got.Text, live)
		}
	}
	if !strings.Contains(got.Text, "&lt;!channel&gt;") || !strings.Contains(got.Text, "&amp;") {
		t.Errorf("text = %q; want the characters shown, not swallowed", got.Text)
	}
}

// Slack says no with a 200, and a driver that reads the status code reports
// every one of those as delivered — so the refusal is marked said and nobody
// ever hears it.
func TestSay_slackRefusesWithATwoHundred_isAnError(t *testing.T) {
	t.Parallel()
	server, _ := sink(t, `{"ok":false,"error":"not_in_channel"}`)

	err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Say(t.Context(), "C07", "1786.1", "triage will not start.")
	if err == nil {
		t.Fatal("no error; the reply never left and the ask would be marked answered")
	}
	if !strings.Contains(err.Error(), "not_in_channel") {
		t.Errorf("err = %v; want Slack's own word, which says what to fix", err)
	}
}
