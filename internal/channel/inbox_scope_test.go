package channel_test

import (
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
The scope an ask was received under travels with it through the inbox (NT-019,
phase 4 readers, migration 0112). The consumer judges the ask later, after the
connection's row may have been removed or recreated at another scope; what it
judges by is the scope the door or the socket session held when the ask
arrived.
*/
func TestInbox_theScopeAnAskWasReceivedUnder_isClaimedWithIt(t *testing.T) {
	inbox := freshInbox(t)
	a := ask("Ev-scope")
	a.ConnectionScope = domain.Scope{Company: "acme", Area: "cx"}
	if _, err := inbox.Receive(t.Context(), a); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	claimed, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("Claim = %+v, %v", claimed, err)
	}
	if got := claimed[0].Via(); got != (channel.Via{Name: "acme-slack", Scope: a.ConnectionScope}) {
		t.Errorf("via = %+v, want the scope it was received under", got)
	}
}

// The previous binary writes an ask without the scope. It verified every
// event with a connection of the installation's, the only place it looked, so
// that is the scope its asks were received under.
func TestInbox_anAskThePreviousBinaryWrote_wasReceivedAtTheInstallation(t *testing.T) {
	inbox := freshInbox(t)
	_, pool := channelStore(t)
	if _, err := pool.Exec(t.Context(), `
		insert into channel_inbox (channel, conversation, event_id, message, asked_by, text, thread,
			agent, run_as, source, payload, digest)
		values ('acme-slack', 'C07-ops', 'Ev-old', '1.1', 'U1', 'hi', '', '', '', '{}', '\x7b7d', 'sha256:00')`); err != nil {
		t.Fatalf("the previous binary's insert: %v", err)
	}

	claimed, err := inbox.Claim(t.Context(), "worker-1", time.Minute, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("Claim = %+v, %v", claimed, err)
	}
	if got := claimed[0].ConnectionScope; got != (domain.Scope{Company: domain.Installation}) {
		t.Errorf("scope = %+v, want the installation's", got)
	}
}
