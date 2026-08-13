package slack_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channel/slack"
)

/*
Offering the conversations rather than asking for an identifier.

`C0123ABCDEF` is not something anybody knows: finding it means opening a
channel's details and scrolling to the bottom. What the operator knows is
`#alertas`.

The listing is `users.conversations` and not `conversations.list` on purpose —
it answers with the conversations the bot is already a member of, so the picker
cannot offer a channel where posting would fail. The commonest mistake in
configuring this, forgetting to invite the bot, stops being possible rather
than being caught later.
*/
func TestConversations_listsOnlyWhatTheBotIsIn(t *testing.T) {
	t.Parallel()
	var asked url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"channels":[
			{"id":"C07","name":"alertas","is_archived":false},
			{"id":"C08","name":"velho","is_archived":true}]}`)
	}))
	defer server.Close()

	found, err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Conversations(t.Context())
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}

	if len(found) != 1 || found[0].ID != "C07" || found[0].Name != "alertas" {
		t.Fatalf("found = %+v, want the live channel only", found)
	}
	// An archived channel accepts no messages. Offering one is offering a
	// configuration that silently delivers nothing.
	if asked.Get("exclude_archived") != "true" {
		t.Errorf("exclude_archived = %q", asked.Get("exclude_archived"))
	}
	if !strings.Contains(asked.Get("types"), "private_channel") {
		t.Errorf("types = %q, want private channels too — an alerts channel is often one",
			asked.Get("types"))
	}
}

func TestConversations_workspaceIsPaged_walksEveryPage(t *testing.T) {
	t.Parallel()
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("cursor") == "" {
			_, _ = io.WriteString(w, `{"ok":true,"channels":[{"id":"C01","name":"um"}],
				"response_metadata":{"next_cursor":"more"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"channels":[{"id":"C02","name":"dois"}]}`)
	}))
	defer server.Close()

	found, err := slack.New("xoxb-test").WithEndpointBase(server.URL).
		Conversations(t.Context())
	if err != nil {
		t.Fatalf("conversations: %v", err)
	}
	if len(found) != 2 || pages != 2 {
		t.Errorf("found %d over %d pages, want both", len(found), pages)
	}
}

// The scope for listing is not the scope for posting. An installation whose app
// was granted only chat:write has to be told that, not handed an empty list
// that reads as "the bot is in no channels".
func TestConversations_missingScope_saysWhichOne(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":false,"error":"missing_scope","needed":"channels:read"}`)
	}))
	defer server.Close()

	_, err := slack.New("xoxb-test").WithEndpointBase(server.URL).Conversations(t.Context())
	if err == nil {
		t.Fatal("a refusal was read as an empty workspace")
	}
	if !strings.Contains(err.Error(), "channels:read") {
		t.Errorf("err = %v, want the scope Slack says it needs", err)
	}
}

var _ = json.Marshal
