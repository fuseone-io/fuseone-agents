package engine

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

/*
A run opened from a ticket can always answer it.

Answering is the whole reason that run exists, and leaving it to a step's
declared reach made the tool something an author has to remember twice: grant
it, and then place it. Forgetting the second finished runs with an answer
nobody ever read, and the transcript said only that the model chose to stop.
*/
func TestEnvelope_aTicketRun_alwaysReachesTheAnswer(t *testing.T) {
	t.Parallel()
	start := Start{
		Pack:  gate.NewPack("crm.lookup"),
		Steps: []Envelope{{Name: "entender"}, {Name: "responder", Reaches: []domain.ToolID{"crm.lookup"}}},
	}
	ticketed := State{Ticket: domain.TicketContext{
		Ref: domain.TicketRef{Key: "ticket-1", Revision: 1},
	}}

	pack := envelopeForState(start, ticketed)
	if !pack.Allows(domain.ToolTicketAnswer) {
		t.Fatalf("pack = %v, want the ticket answer reachable", pack.Tools())
	}

	// A run that is not a ticket's is unchanged: the tool is not offered to
	// agents that have nothing to answer.
	if envelopeForState(start, State{}).Allows(domain.ToolTicketAnswer) {
		t.Fatal("a run outside a ticket was offered the ticket answer")
	}
}
