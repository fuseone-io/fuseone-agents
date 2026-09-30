package connectortools_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/connectortools"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

var answerNow = time.Date(2026, 9, 24, 18, 0, 0, 0, time.UTC)

/*
Answering a ticket is an effect, and the approval is about the exact words.

The text the approver reads is sealed as evidence when the approval is asked
for. Executing compares what was approved with what is being published, so a
run that rewrote its answer after the card went out publishes nothing.
*/
func TestTicketAnswers_theApprovedTextIsWhatTheTicketPublishes(t *testing.T) {
	t.Parallel()
	content := engine.NewMemoryContent()
	tickets := ticket.NewMemory()
	held := openAnswerTicket(t, tickets)
	answers := connectortools.NewTicketAnswers(content, tickets).At(func() time.Time { return answerNow })

	call := engine.Call{
		RunID: "run-1", Seq: 4, Scope: held.Scope, AgentID: held.Agent,
		Tool: connectortools.TicketAnswerTool,
		Args: []byte(`{"text":"Monitor criado por Terraform, veja o PR."}`),
		Ticket: domain.TicketContext{
			Ref: held.Current.Ref, RequestedBy: held.RequestedBy, AddressedBy: held.AddressedBy,
		},
		At: answerNow,
	}

	evidence, err := answers.ApprovalEvidence(t.Context(), call)
	if err != nil || !evidence.Valid() || evidence.Ticket != held.Current.Ref {
		t.Fatalf("ApprovalEvidence = (%+v, %v)", evidence, err)
	}
	sealed, err := content.Get(t.Context(), evidence.Ref)
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if !json.Valid(sealed) {
		t.Fatalf("evidence is not readable: %q", sealed)
	}

	call.ApprovalEvidence, call.ApprovalAtSeq, call.DecidedBy = evidence, 3, "usr_manager"
	if _, err := answers.Answer(t.Context(), call); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	settled, err := tickets.Current(t.Context(), held.Key)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if settled.Current.Phase != ticket.PhaseCompleted || settled.Current.Outcome == nil {
		t.Fatalf("ticket = %+v, want it completed with an outcome", settled.Current)
	}
	raw, err := content.Get(t.Context(), settled.Current.Outcome.Result.Ref)
	if err != nil {
		t.Fatalf("read outcome: %v", err)
	}
	message, err := connectortools.TicketAnswerRenderer{}.RenderTicketOutcome(ticket.PhaseCompleted, raw)
	if err != nil || message != "Monitor criado por Terraform, veja o PR." {
		t.Fatalf("rendered = %q, %v, want the approved text", message, err)
	}
}

func TestTicketAnswers_textThatIsNotTheApprovedOne_publishesNothing(t *testing.T) {
	t.Parallel()
	content := engine.NewMemoryContent()
	tickets := ticket.NewMemory()
	held := openAnswerTicket(t, tickets)
	answers := connectortools.NewTicketAnswers(content, tickets).At(func() time.Time { return answerNow })

	call := engine.Call{
		RunID: "run-1", Seq: 4, Scope: held.Scope, AgentID: held.Agent,
		Tool: connectortools.TicketAnswerTool, Args: []byte(`{"text":"o texto aprovado"}`),
		Ticket: domain.TicketContext{Ref: held.Current.Ref, RequestedBy: held.RequestedBy},
		At:     answerNow,
	}
	evidence, err := answers.ApprovalEvidence(t.Context(), call)
	if err != nil {
		t.Fatalf("ApprovalEvidence: %v", err)
	}

	call.ApprovalEvidence, call.ApprovalAtSeq = evidence, 3
	call.Args = []byte(`{"text":"outro texto, escrito depois do card"}`)
	if _, err := answers.Answer(t.Context(), call); err == nil {
		t.Fatal("Answer accepted text the approver never read")
	}
	settled, _ := tickets.Current(t.Context(), held.Key)
	if settled.Current.Phase == ticket.PhaseCompleted {
		t.Fatalf("phase = %s, want the ticket untouched", settled.Current.Phase)
	}
}

func TestTicketAnswers_aCallOutsideATicket_isRefused(t *testing.T) {
	t.Parallel()
	answers := connectortools.NewTicketAnswers(engine.NewMemoryContent(), ticket.NewMemory())
	call := engine.Call{
		RunID: "run-1", Seq: 4, Tool: connectortools.TicketAnswerTool,
		Args: []byte(`{"text":"olá"}`), At: answerNow,
	}
	if _, err := answers.ApprovalEvidence(t.Context(), call); err == nil {
		t.Fatal("sealed an answer for no ticket")
	}
	if _, err := answers.Answer(t.Context(), call); err == nil {
		t.Fatal("answered no ticket")
	}
}

func TestTicketAnswers_anEmptyOrOversizedAnswer_isRefused(t *testing.T) {
	t.Parallel()
	content := engine.NewMemoryContent()
	tickets := ticket.NewMemory()
	held := openAnswerTicket(t, tickets)
	answers := connectortools.NewTicketAnswers(content, tickets)

	for name, text := range map[string]string{
		"nothing to say":                "   ",
		"more than the thread can take": string(make([]byte, connectortools.MaxTicketAnswerBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			args, err := json.Marshal(map[string]string{"text": text})
			if err != nil {
				t.Fatal(err)
			}
			call := engine.Call{
				RunID: "run-1", Seq: 4, Tool: connectortools.TicketAnswerTool, Args: args,
				Ticket: domain.TicketContext{Ref: held.Current.Ref}, At: answerNow,
			}
			if _, err := answers.ApprovalEvidence(t.Context(), call); err == nil {
				t.Fatal("sealed an answer nobody can publish")
			}
		})
	}
}

func openAnswerTicket(t *testing.T, tickets *ticket.Memory) ticket.Ticket {
	t.Helper()
	origin := ticket.Origin{Connection: "acme-slack", Conversation: "C-help", Root: "171.1"}
	key, err := ticket.Key(origin.Connection, origin.Conversation, origin.Root)
	if err != nil {
		t.Fatal(err)
	}
	held, _, err := tickets.Open(t.Context(), ticket.OpenInput{
		Key: key, Origin: origin,
		Scope: domain.Scope{Company: "acme", Area: "platform"},
		Agent: "ticketito", RunAs: "usr_platform", RequestedBy: "usr_requester",
		AddressedBy: "bot:B-triage", ReviewIn: "C-agents", EventID: "event-open",
		Draft: ticket.ContentRef{Ref: "ticket://draft-1", Digest: "sha256:draft-1"},
		At:    answerNow,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return held
}
