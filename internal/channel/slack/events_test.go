package slack_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/channel/slack"
)

/*
What arrives on the events path, read as the two things it can be.

Only a mention is read. A trigger is a deliberate ask, never every message in
the channel: the listening version costs tokens on ambient conversation, puts
people's talk in front of a model nobody asked to involve, and fills the record
with runs nobody started. The mention is the consent, and it is free.
*/

func TestReadDelivery_aMention_isAnAsk(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev123",
	  "event":{"type":"app_mention","channel":"C07-ops","user":"U9",
	           "text":"<@U07BOT> triagem esse chamado","ts":"1786.1"}
	}`))
	if err != nil {
		t.Fatalf("ReadDelivery: %v", err)
	}
	if got.EventID != "Ev123" || got.Conversation != "C07-ops" || got.User != "U9" {
		t.Errorf("got %+v, want the ask", got)
	}
	// A reply belongs where the ask was made. An ask that started a thread is
	// its own parent.
	if got.Thread != "1786.1" {
		t.Errorf("thread = %q, want the message that started it", got.Thread)
	}
}

func TestReadDelivery_insideAThread_repliesToTheParent(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev124",
	  "event":{"type":"app_mention","channel":"C07-ops","user":"U9",
	           "text":"<@U07BOT> triagem","ts":"1786.9","thread_ts":"1786.1"}
	}`))
	if err != nil {
		t.Fatalf("ReadDelivery: %v", err)
	}
	if got.Thread != "1786.1" {
		t.Errorf("thread = %q, want the parent", got.Thread)
	}
}

/*
An ordinary message is not an ask, even in a channel the bot is in.

Subscribing to every message would deliver every word typed where the bot sits.
That is the difference between a platform people invite and one they discover
reading their conversations.
*/
func TestReadDelivery_anOrdinaryMessage_isNotAnAsk(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev125",
	  "event":{"type":"message","channel":"C07-ops","user":"U9","text":"bom dia","ts":"1786.2"}
	}`))
	if !errors.Is(err, slack.ErrNotAnAsk) {
		t.Errorf("err = %v, want ErrNotAnAsk", err)
	}
}

func TestReadAnyDelivery_aBotMessageCanBeAWatchCandidate(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadAnyDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev300",
	  "event":{"type":"message","subtype":"bot_message","channel":"C07-ops",
	           "bot_id":"B-alerts","app_id":"A-alerts",
	           "text":"firing alertGatewayRTMInterfaceErrors","ts":"1786.5"}
	}`))
	if err != nil {
		t.Fatalf("ReadAnyDelivery: %v", err)
	}
	if got.Kind != slack.DeliveryMessage || got.Source.Bot != "B-alerts" ||
		got.Source.App != "A-alerts" || got.Text == "" {
		t.Fatalf("delivery = %+v, want the watched-message candidate", got)
	}
}

// A bot's own mention is how two bots discover each other and never stop.
func TestReadDelivery_aBotSpeaking_isNotAnAsk(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev126",
	  "event":{"type":"app_mention","channel":"C07-ops","bot_id":"B1",
	           "text":"<@U07BOT> triagem","ts":"1786.3"}
	}`))
	if !errors.Is(err, slack.ErrNotAnAsk) {
		t.Errorf("err = %v, want ErrNotAnAsk", err)
	}
}

// An edit is not somebody asking again. It arrives as a message with a
// subtype, and reading it as an ask would run an agent because a typo was
// corrected.
func TestReadDelivery_anEditedMessage_isNotAnAsk(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev127",
	  "event":{"type":"app_mention","subtype":"message_changed","channel":"C07-ops",
	           "user":"U9","text":"<@U07BOT> triagem","ts":"1786.4"}
	}`))
	if !errors.Is(err, slack.ErrNotAnAsk) {
		t.Errorf("err = %v, want ErrNotAnAsk", err)
	}
}

func TestReadDelivery_theUrlHandshake_comesBackToBeAnswered(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadDelivery([]byte(`{"type":"url_verification","challenge":"abc123"}`))
	if err != nil {
		t.Fatalf("ReadDelivery: %v", err)
	}
	if got.Challenge != "abc123" {
		t.Errorf("challenge = %q, want it echoed", got.Challenge)
	}
}

/*
A signed payload missing what an ask is made of.

Not defensiveness about Slack, which sends these: it is about everything else
that can put a signed body on this path — a proxy that rewrote it, a replay of
an older shape, a fixture somebody copied. Read leniently, a mention with no
channel becomes an arrival filed under an empty conversation, which resolves to
no scope and refuses later, further from the cause.
*/
func TestReadDelivery_aMentionWithNoConversation_isMalformed(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev200",
	  "event":{"type":"app_mention","user":"U9","text":"<@U07BOT> triagem","ts":"1786.1"}
	}`))
	// Malformed rather than "not an ask": the two deserve different answers,
	// and a mention nobody can read is not a message that was never for us.
	if !errors.Is(err, slack.ErrMalformedAsk) {
		t.Errorf("err = %v, want ErrMalformedAsk", err)
	}
}

func TestReadDelivery_aMentionWithNoTimestamp_isMalformed(t *testing.T) {
	t.Parallel()

	_, err := slack.ReadDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev201",
	  "event":{"type":"app_mention","channel":"C07-ops","user":"U9","text":"<@U07BOT> triagem"}
	}`))
	if !errors.Is(err, slack.ErrMalformedAsk) {
		t.Errorf("err = %v, want ErrMalformedAsk", err)
	}
}

/*
An alert whose words are in its blocks is still an alert.

Alerting systems posting through Slack routinely leave `text` empty and put
everything in blocks or attachments. Read from `text` alone, such a message
became a run with an empty input: a model call paid for with no question in it.
*/
func TestReadAnyDelivery_anAlertWithNoTextButBlocks_carriesTheBlockText(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadAnyDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev301",
	  "event":{"type":"message","subtype":"bot_message","channel":"C07-ops",
	           "bot_id":"B-alerts","text":"","ts":"1786.6",
	           "blocks":[
	             {"type":"header","text":{"type":"plain_text","text":"FIRING: GatewayRTMInterfaceErrors"}},
	             {"type":"section","fields":[
	               {"type":"mrkdwn","text":"*severity:* critical"},
	               {"type":"mrkdwn","text":"*cluster:* prod-1"}]}]}
	}`))
	if err != nil {
		t.Fatalf("ReadAnyDelivery: %v", err)
	}
	for _, want := range []string{
		"FIRING: GatewayRTMInterfaceErrors", "*severity:* critical", "*cluster:* prod-1",
	} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("text = %q, want it to carry %q", got.Text, want)
		}
	}
}

func TestReadAnyDelivery_anAlertWithNoTextButAttachments_carriesTheAttachmentText(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadAnyDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev302",
	  "event":{"type":"message","subtype":"bot_message","channel":"C07-ops",
	           "bot_id":"B-alerts","text":"","ts":"1786.7",
	           "attachments":[{"title":"Grafana OnCall",
	                           "text":"latency above 2s for 5m",
	                           "fields":[{"title":"env","value":"prod"}]}]}
	}`))
	if err != nil {
		t.Fatalf("ReadAnyDelivery: %v", err)
	}
	for _, want := range []string{"Grafana OnCall", "latency above 2s for 5m", "env", "prod"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("text = %q, want it to carry %q", got.Text, want)
		}
	}
}

// The message's own text is what the sender chose to say. Blocks are read only
// when there is none, so a message that has both is not doubled.
func TestReadAnyDelivery_anAlertWithTextAndBlocks_keepsOnlyTheText(t *testing.T) {
	t.Parallel()

	got, err := slack.ReadAnyDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev303",
	  "event":{"type":"message","subtype":"bot_message","channel":"C07-ops",
	           "bot_id":"B-alerts","text":"firing GatewayRTMInterfaceErrors","ts":"1786.8",
	           "blocks":[{"type":"section","text":{"type":"mrkdwn","text":"duplicated detail"}}]}
	}`))
	if err != nil {
		t.Fatalf("ReadAnyDelivery: %v", err)
	}
	if got.Text != "firing GatewayRTMInterfaceErrors" {
		t.Errorf("text = %q, want the sender's own words alone", got.Text)
	}
}

/*
A control's callback data is not something a person said.

An interactive element carries a `value` the app reads back when somebody
clicks — callback data, an identifier, sometimes a token — and it is never on
screen. Only the label a person can see belongs in what an agent is asked
about.
*/
func TestReadAnyDelivery_blockText_keepsControlLabelsAndNotTheirValues(t *testing.T) {
	t.Parallel()

	got := blockDelivery(t, `"blocks":[
	  {"type":"section","text":{"type":"mrkdwn","text":"Disk full on db-a"},
	   "accessory":{"type":"button","text":{"type":"plain_text","text":"Acknowledge"},
	                "value":"private-callback-token","action_id":"ack","url":"https://internal/ack"}},
	  {"type":"actions","elements":[
	    {"type":"button","text":{"type":"plain_text","text":"Silence"},"value":"another-token"}]}]`)

	for _, want := range []string{"Disk full on db-a", "Acknowledge", "Silence"} {
		if !strings.Contains(got, want) {
			t.Errorf("text = %q, want the visible label %q", got, want)
		}
	}
	for _, unwanted := range []string{"private-callback-token", "another-token", "ack", "internal/ack"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("text = %q, want the control's own wiring left out (%q)", got, unwanted)
		}
	}
}

// An attachment field's value is the other thing entirely: it is one half of a
// label-and-value pair somebody reads off the screen.
func TestReadAnyDelivery_blockText_keepsAttachmentFieldValues(t *testing.T) {
	t.Parallel()

	got := blockDelivery(t, `"attachments":[{"title":"Grafana OnCall",
	  "fields":[{"title":"env","value":"prod"},{"title":"latency","value":"2.4s"}]}]`)

	for _, want := range []string{"Grafana OnCall", "env", "prod", "latency", "2.4s"} {
		if !strings.Contains(got, want) {
			t.Errorf("text = %q, want it to carry %q", got, want)
		}
	}
}

/*
The same word in two places is two facts.

Two hosts reported down is not one host reported down. Dropping repeats across
the whole payload deleted the second state and left the reader pairing labels
with the wrong values.
*/
func TestReadAnyDelivery_blockText_keepsRepeatedValuesInDifferentBlocks(t *testing.T) {
	t.Parallel()

	got := blockDelivery(t, `"blocks":[
	  {"type":"section","fields":[{"type":"mrkdwn","text":"db-a"},{"type":"mrkdwn","text":"down"}]},
	  {"type":"section","fields":[{"type":"mrkdwn","text":"db-b"},{"type":"mrkdwn","text":"down"}]}]`)

	if got != "db-a\ndown\ndb-b\ndown" {
		t.Errorf("text = %q, want both states kept", got)
	}
}

// An attachment's fallback is a plain-text rendering of its own text. Saying it
// twice tells a reader it matters twice.
func TestReadAnyDelivery_blockText_saysAnAttachmentsFallbackOnce(t *testing.T) {
	t.Parallel()

	got := blockDelivery(t, `"attachments":[{"fallback":"latency above 2s","text":"latency above 2s",
	  "title":"Grafana OnCall"}]`)

	if strings.Count(got, "latency above 2s") != 1 {
		t.Errorf("text = %q, want the attachment's own repeat dropped", got)
	}
	if !strings.Contains(got, "Grafana OnCall") {
		t.Errorf("text = %q, want the title kept", got)
	}
}

/*
The cut lands on a character boundary, and says that it happened.

A prefix taken by byte count splits a multi-byte rune and produces a string
Postgres will not store — so a valid Slack delivery fails to be written down
and the webhook is retried forever. And a model told nothing about the omission
reasons from a sentence that stops mid-fact.
*/
func TestReadAnyDelivery_blockText_cutsOnARuneBoundaryAndSaysSo(t *testing.T) {
	t.Parallel()

	// Padded four ways, so where the cut lands relative to a two-byte rune
	// does not depend on the exact size of the ceiling or of the note. One of
	// these four always asks the cut to land in the middle of a character.
	for pad := range 4 {
		said := strings.Repeat("A", pad) + strings.Repeat("é", 4<<10)
		got := blockDelivery(t, `"blocks":[{"type":"section","text":{"type":"mrkdwn","text":"`+
			said+`"}}]`)

		if !utf8.ValidString(got) {
			t.Fatalf("pad %d: text is not valid UTF-8, ends %q", pad, got[len(got)-8:])
		}
		if len(got) > 4<<10 {
			t.Errorf("pad %d: text is %d bytes, want it inside the 4 KiB ceiling", pad, len(got))
		}
		if !strings.Contains(got, "not read") {
			t.Errorf("pad %d: text does not say that the rest was omitted", pad)
		}
	}
}

// The ceiling covers the separators and the note as well as the words, or a
// message assembled from many small blocks slips past it.
func TestReadAnyDelivery_blockText_countsSeparatorsAgainstTheCeiling(t *testing.T) {
	t.Parallel()

	var blocks []string
	for range 500 {
		blocks = append(blocks,
			`{"type":"section","text":{"type":"mrkdwn","text":"`+strings.Repeat("A", 60)+`"}}`)
	}
	got := blockDelivery(t, `"blocks":[`+strings.Join(blocks, ",")+`]`)

	if len(got) > 4<<10 {
		t.Errorf("text is %d bytes, want it inside the 4 KiB ceiling", len(got))
	}
	if !strings.Contains(got, "not read") {
		t.Error("a truncated message did not say so")
	}
}

/*
One payload reads as one string, whatever order Go walks a map in.

Read in map order the same alert reaches the model differently on different
days, and nobody can reproduce what an agent was asked.
*/
func TestReadAnyDelivery_blockTextReadsTheSameEveryTime(t *testing.T) {
	t.Parallel()

	body := `"attachments":[{"fallback":"F","title":"T","text":"X",
	  "fields":[{"title":"k","value":"v"}]}]`

	got := blockDelivery(t, body)
	if got != "F\nT\nX\nk\nv" {
		t.Fatalf("text = %q, want the fields in one settled order", got)
	}
	for range 16 {
		if again := blockDelivery(t, body); again != got {
			t.Fatalf("the same payload read as %q and then %q", got, again)
		}
	}
}

// Nesting is bounded. A payload deep enough to be a shape nobody posts on
// purpose is read as far as the bound and no further, and what is within reach
// still arrives.
func TestReadAnyDelivery_blockTextStopsDescendingAtTheBound(t *testing.T) {
	t.Parallel()

	deep := `{"type":"rich_text_section","elements":[{"type":"text","text":"deep-leaf"}]}`
	for range 40 {
		deep = `{"type":"rich_text","elements":[` + deep + `]}`
	}
	got := blockDelivery(t, `"blocks":[{"type":"section","text":{"type":"mrkdwn","text":"shallow"}},`+
		deep+`]`)

	if !strings.Contains(got, "shallow") {
		t.Errorf("text = %q, want what is within reach", got)
	}
	if strings.Contains(got, "deep-leaf") {
		t.Errorf("text = %q, want the walk to have stopped at the bound", got)
	}
}

// blockDelivery reads one bot message carrying no text of its own.
func blockDelivery(t *testing.T, content string) string {
	t.Helper()
	got, err := slack.ReadAnyDelivery([]byte(`{
	  "type":"event_callback","event_id":"Ev400",
	  "event":{"type":"message","subtype":"bot_message","channel":"C07-ops",
	           "bot_id":"B-alerts","text":"","ts":"1787.0",` + content + `}}`))
	if err != nil {
		t.Fatalf("ReadAnyDelivery: %v", err)
	}
	return got.Text
}
