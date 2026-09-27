package connectortools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

/*
Answering a governed ticket.

The agent does not write in the support thread. It proposes an answer, and the
proposal is an effect like any other: the Gate stops it, the card carries the
exact words, and a person who may decide releases them. Publishing is then the
platform's, not the model's — the approved text is sealed as the ticket's
outcome and the thread is answered by the consumer that owes it.

The approval is about those words and no others. What the approver read is
sealed as evidence when the request is made, and execution compares it with
what is being published: a run that rewrote its answer after the card went out
publishes nothing, which is the only reading of an approval that survives being
looked at afterwards.
*/

// TicketAnswerTool is the platform's own tool: nothing is reached over a
// network, and the effect is entirely inside this installation. The name lives
// in the domain beside the other platform tools, because an agent declares it
// there and the spec has to be able to read it.
const TicketAnswerTool = domain.ToolTicketAnswer

// MaxTicketAnswerBytes bounds one answer. A support thread is read by people
// on telephones, and a model with no limit writes until it runs out of budget.
const MaxTicketAnswerBytes = 8 << 10

const ticketAnswerKind = "ticket_answer"

var (
	ErrTicketAnswerScope    = errors.New("connector: the ticket answer tool runs only inside a governed ticket")
	ErrTicketAnswerText     = errors.New("connector: a ticket answer is one bounded piece of text")
	ErrTicketAnswerApproval = errors.New("connector: the text being published is not the text that was approved")
)

type ticketAnswerArgs struct {
	Text string `json:"text"`
}

// ticketAnswerResult is the safe projection: what was published, as it was
// approved. It is what the thread is answered with and what an auditor reads.
type ticketAnswerResult struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// TicketAnswers turns an approved proposal into the ticket's outcome.
type TicketAnswers struct {
	content engine.ContentStore
	tickets ticket.Store
	now     func() time.Time
}

func NewTicketAnswers(content engine.ContentStore, tickets ticket.Store) *TicketAnswers {
	return &TicketAnswers{content: content, tickets: tickets, now: time.Now}
}

// At fixes the clock. Tests read the ticket back by the instant it settled.
func (a *TicketAnswers) At(now func() time.Time) *TicketAnswers {
	if now != nil {
		a.now = now
	}
	return a
}

// ApprovalEvidence seals the proposed answer so the decision is about words a
// person actually read.
func (a *TicketAnswers) ApprovalEvidence(
	ctx context.Context, call engine.Call,
) (domain.ApprovalEvidence, error) {
	text, err := a.proposed(call)
	if err != nil {
		return domain.ApprovalEvidence{}, err
	}
	ref, digest, err := a.seal(ctx, call, text)
	if err != nil {
		return domain.ApprovalEvidence{}, err
	}
	// Recorded against the ticket as well as returned: the approval this is
	// about is released against the snapshot the ticket holds, so evidence the
	// ticket never saw would clear a decision nobody could reconstruct.
	if _, _, err := a.tickets.RecordInspection(ctx, ticket.InspectionInput{
		Ref: call.Ticket.Ref, Snapshot: ticket.ContentRef{Ref: ref, Digest: digest},
		At: call.At,
	}); err != nil {
		return domain.ApprovalEvidence{}, err
	}
	return domain.ApprovalEvidence{
		Kind: ticketAnswerKind, Ticket: call.Ticket.Ref, Ref: ref, Digest: digest,
	}, nil
}

// Answer publishes what was approved, by completing the ticket with it. The
// thread itself is written by the outcome consumer: this side owns the words
// and their governance, and the connector owns how Slack is spoken to.
func (a *TicketAnswers) Answer(
	ctx context.Context, call engine.Call,
) (engine.ToolResult, error) {
	text, err := a.proposed(call)
	if err != nil {
		return engine.ToolResult{}, err
	}
	if err := a.approved(ctx, call, text); err != nil {
		return engine.ToolResult{}, err
	}
	approved := ticket.ContentRef{Ref: call.ApprovalEvidence.Ref, Digest: call.ApprovalEvidence.Digest}
	if _, _, err := a.tickets.AwaitApproval(ctx, ticket.ApprovalInput{
		Ref: call.Ticket.Ref, RunID: call.RunID, AtSeq: call.ApprovalAtSeq,
		Snapshot: approved, At: call.At,
	}); err != nil {
		return engine.ToolResult{}, err
	}
	claimed, _, err := a.tickets.ClaimExecution(ctx, ticket.ClaimInput{
		Ref: call.Ticket.Ref, RunID: call.RunID,
		ApprovalAtSeq: call.ApprovalAtSeq, At: call.At.UTC(),
	})
	if err != nil {
		return engine.ToolResult{}, err
	}
	if claimed.Active == nil {
		return engine.ToolResult{}, ticket.ErrExecutionActive
	}
	ref, digest, err := a.seal(ctx, call, text)
	if err != nil {
		return engine.ToolResult{}, err
	}
	result := ticket.ContentRef{Ref: ref, Digest: digest}
	if _, err := a.tickets.FinishExecution(ctx, ticket.FinishInput{
		Execution: *claimed.Active, Phase: ticket.PhaseCompleted,
		Result: result, At: a.now().UTC(),
	}); err != nil {
		return engine.ToolResult{}, err
	}
	return engine.ToolResult{
		ResultRef: ref, ResultDigest: digest, ResultBytes: int64(len(text)),
	}, nil
}

// proposed reads the one argument the model owns, and bounds it.
func (a *TicketAnswers) proposed(call engine.Call) (string, error) {
	if !call.Ticket.Ref.Valid() {
		return "", ErrTicketAnswerScope
	}
	var args ticketAnswerArgs
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return "", fmt.Errorf("%w: %s", ErrTicketAnswerText, "unreadable arguments")
	}
	text := strings.TrimSpace(args.Text)
	if text == "" || len(text) > MaxTicketAnswerBytes || !utf8.ValidString(text) {
		return "", ErrTicketAnswerText
	}
	return text, nil
}

// approved answers whether these are the words the decision was about.
func (a *TicketAnswers) approved(ctx context.Context, call engine.Call, text string) error {
	evidence := call.ApprovalEvidence
	if evidence.Kind != ticketAnswerKind || !evidence.Valid() ||
		evidence.Ticket != call.Ticket.Ref || call.ApprovalAtSeq <= 0 {
		return ErrTicketAnswerApproval
	}
	raw, err := a.content.Get(ctx, evidence.Ref)
	if err != nil {
		return ErrTicketAnswerApproval
	}
	sealed, err := decodeTicketAnswer(raw)
	if err != nil || sealed.Text != text ||
		engine.ResultDigest(raw) != evidence.Digest {
		return ErrTicketAnswerApproval
	}
	return nil
}

func (a *TicketAnswers) seal(
	ctx context.Context, call engine.Call, text string,
) (string, string, error) {
	raw, err := json.Marshal(ticketAnswerResult{Kind: ticketAnswerKind, Text: text})
	if err != nil {
		return "", "", fmt.Errorf("connector: encode ticket answer: %w", err)
	}
	ref, err := a.content.Put(ctx, call.RunID, call.Seq, raw)
	if err != nil {
		return "", "", fmt.Errorf("connector: store ticket answer: %w", err)
	}
	return ref, engine.ResultDigest(raw), nil
}

func decodeTicketAnswer(raw []byte) (ticketAnswerResult, error) {
	var out ticketAnswerResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return ticketAnswerResult{}, err
	}
	if out.Kind != ticketAnswerKind || strings.TrimSpace(out.Text) == "" {
		return ticketAnswerResult{}, errors.New("connector: not a ticket answer")
	}
	return out, nil
}

// TicketAnswerRenderer writes the approved words into the support thread,
// unchanged. Nothing is added: an answer a person approved is not improved by
// a heading nobody read.
type TicketAnswerRenderer struct{}

func (TicketAnswerRenderer) RenderTicketOutcome(
	phase ticket.Phase, raw []byte,
) (string, error) {
	answer, err := decodeTicketAnswer(raw)
	if err != nil {
		return "", err
	}
	if phase != ticket.PhaseCompleted {
		return "", fmt.Errorf("connector: a ticket answer settles as completed, not %s", phase)
	}
	return answer.Text, nil
}

// TicketAnswerEntry is how the tool appears to an agent that declares it.
func TicketAnswerEntry() domain.ToolEntry {
	return domain.ToolEntry{
		ID:     TicketAnswerTool,
		Server: "fuseone",
		Description: "Propose the answer to publish in the ticket thread. " +
			"It is published only after somebody who may decide approves the exact text.",
		Effect: domain.EffectWrite,
		Native: true,
	}
}

// ticketAnswerSchema is the one field the model owns. The fields alone: a tool
// schema here is what goes inside the object, and each provider builds the
// object around it.
func ticketAnswerSchema() map[string]any {
	return map[string]any{
		"text": map[string]any{
			"type":        "string",
			"minLength":   1,
			"maxLength":   MaxTicketAnswerBytes,
			"description": "The answer, as it should appear in the ticket thread.",
		},
	}
}

// TicketAnswerEvidenceKind names this evidence on an approval request, so a
// reader can tell which decoder the sealed bytes belong to.
const TicketAnswerEvidenceKind = ticketAnswerKind

// DecodeTicketAnswerEvidence reads the words an approval is about.
//
// The digest is checked against what the request sealed: an approval is about
// the text a person read, and bytes that no longer hash to it are not that
// text however well they decode.
func DecodeTicketAnswerEvidence(raw []byte, evidence domain.ApprovalEvidence) (string, error) {
	if evidence.Kind != ticketAnswerKind || !evidence.Valid() {
		return "", errors.New("connector: not a ticket answer approval")
	}
	if engine.ResultDigest(raw) != evidence.Digest {
		return "", errors.New("connector: the sealed ticket answer has changed")
	}
	answer, err := decodeTicketAnswer(raw)
	if err != nil {
		return "", err
	}
	return answer.Text, nil
}
