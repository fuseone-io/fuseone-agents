package slack_test

import (
	"context"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
	"github.com/fuseone/agents/internal/domain"
)

/*
Every event a Socket Mode session receives is judged by the scope the session
was opened with (NT-019, phase 4 readers): the ticket route, the watch rule and
the ask written to the inbox all carry it. The row may change before the next
reconciliation; the session that received the event does not.
*/
func TestSocketReceiver_everyJudgementCarriesTheSessionsScope(t *testing.T) {
	t.Parallel()
	cx := domain.Scope{Company: "acme", Area: "cx"}
	inbox := &socketInbox{}
	rules := &scopeRules{}
	tickets := &scopeTickets{}

	if _, err := (slack.SocketReceiver{
		Channel: "cx-slack", Scope: cx, Inbox: inbox, Rules: rules, Tickets: tickets,
	}).Handle(context.Background(), socketFrame("env-s", map[string]any{
		"type": "event_callback", "event_id": "Ev-s",
		"event": map[string]any{
			"type": "message", "subtype": "bot_message",
			"channel": "C07", "bot_id": "B-alerts", "text": "firing", "ts": "1786.60",
		},
	})); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if tickets.got.ConnectionScope != cx {
		t.Errorf("ticket candidate scope = %+v, want the session's", tickets.got.ConnectionScope)
	}
	if rules.got != (channel.Via{Name: "cx-slack", Scope: cx}) {
		t.Errorf("watch rule via = %+v, want the session's", rules.got)
	}
	if inbox.got.ConnectionScope != cx {
		t.Errorf("arrival scope = %+v, want the session's", inbox.got.ConnectionScope)
	}
}

type scopeRules struct{ got channel.Via }

func (r *scopeRules) WatchFor(
	_ context.Context, via channel.Via, _ string, _ channel.Source,
) (channel.WatchRule, bool, error) {
	r.got = via
	return channel.WatchRule{Agent: "triagem", RunAs: "usr_opsbot"}, true, nil
}

type scopeTickets struct{ got channel.TicketCandidate }

func (r *scopeTickets) Route(
	_ context.Context, c channel.TicketCandidate,
) (channel.TicketIntent, bool, error) {
	r.got = c
	return channel.TicketIntent{}, false, nil
}
