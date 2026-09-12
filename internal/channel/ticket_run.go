package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

func (h *TicketHandler) openRevision(
	ctx context.Context, held ticket.Ticket, revision ticket.Revision,
) (TicketResult, error) {
	if err := h.closeSupersededApprovals(ctx, held.Current.Ref); err != nil {
		return TicketResult{}, err
	}
	if revision.Phase != ticket.PhaseCollecting {
		return handled("ticket_revision_already_advanced"), nil
	}
	if held.Active != nil && held.Active.Ref != revision.Ref {
		// The reply is durable in the inbox, so retrying it after the active
		// revision finishes loses nothing. Opening now would let the new run
		// ask for an approval it cannot claim, then strand that revision after
		// the existing execution releases the ticket.
		return TicketResult{}, ticket.ErrExecutionActive
	}
	raw, err := h.content.Get(ctx, revision.Draft.Ref)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: read ticket context: %w", err)
	}
	opened, err := h.opener.Open(ctx, Request{
		Agent: held.Agent, IdemKey: ticketRunKey(revision.Ref), Trigger: "channel_ticket",
		By: held.RunAs, Input: raw, Labels: domain.NewLabels(domain.LabelUntrusted),
		Origin: &domain.RunOrigin{Channel: held.Origin.Connection,
			Conversation: held.Origin.Conversation, Message: held.Origin.Root, Thread: held.Origin.Root},
		Ticket: &domain.TicketContext{Ref: revision.Ref,
			RequestedBy: held.RequestedBy, AddressedBy: held.AddressedBy},
	})
	if errors.Is(err, ErrWontStart) {
		return h.cancelUnstartable(ctx, held, revision)
	}
	if err != nil {
		return TicketResult{}, err
	}
	return TicketResult{RunID: opened.RunID}, nil
}

func (h *TicketHandler) closeSupersededApprovals(
	ctx context.Context, current domain.TicketRef,
) error {
	if current.Revision <= 1 {
		return nil
	}
	approvals, err := h.store.SupersededApprovals(ctx, current)
	if err != nil {
		return fmt.Errorf("channel: find superseded ticket approvals: %w", err)
	}
	for _, approval := range approvals {
		if err := h.closeSupersededApproval(ctx, approval.Approval); err != nil {
			return err
		}
		if err := h.store.MarkApprovalSuperseded(ctx, approval.Ref, h.now().UTC()); err != nil {
			return fmt.Errorf("channel: settle superseded ticket approval: %w", err)
		}
	}
	return nil
}

func (h *TicketHandler) closeSupersededApproval(
	ctx context.Context, approval ticket.Approval,
) error {
	steps, err := h.runs.Read(ctx, approval.RunID, approval.AtSeq)
	if err != nil {
		return fmt.Errorf("channel: read superseded ticket run: %w", err)
	}
	if len(steps) == 0 || steps[0].Seq != approval.AtSeq ||
		steps[0].Kind != domain.StepApprovalRequested {
		return errors.New("channel: ticket approval does not name an approval request")
	}
	question := steps[0]
	payload, err := json.Marshal(domain.FailedPayload{
		Code: "ticket_revision_superseded", Retryable: false,
	})
	if err != nil {
		return fmt.Errorf("channel: encode superseded ticket failure: %w", err)
	}
	_, err = h.runs.AppendIfHead(ctx,
		domain.StepRef{Seq: approval.AtSeq, Kind: domain.StepApprovalRequested},
		domain.Step{
			RunID: approval.RunID, Kind: domain.StepFailed,
			Scope: question.Scope, AgentID: question.AgentID, VersionID: question.VersionID,
			OnBehalfOf: question.OnBehalfOf, Labels: question.Labels,
			Payload: payload, At: h.now().UTC(),
		},
	)
	if errors.Is(err, domain.ErrHeadMoved) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("channel: close superseded ticket approval: %w", err)
	}
	return nil
}

func (h *TicketHandler) cancelUnstartable(
	ctx context.Context, held ticket.Ticket, revision ticket.Revision,
) (TicketResult, error) {
	raw := []byte(`{"status":"cancelled","reason":"agent_will_not_start"}`)
	result, err := h.storeTicketContent(ctx, "ticket-result", held.Key, revision.Ref.Revision, raw)
	if err != nil {
		return TicketResult{}, err
	}
	if _, _, err := h.store.Close(ctx, ticket.CloseInput{
		Ref: revision.Ref, Phase: ticket.PhaseCancelled, Result: result, At: h.now().UTC(),
	}); err != nil {
		return TicketResult{}, err
	}
	return TicketResult{Refusal: Refusal{
		Why:    "The configured agent cannot start this ticket right now.",
		Reason: "ticket_agent_unavailable",
	}}, nil
}

func (h *TicketHandler) storeDraft(
	ctx context.Context, key domain.TicketKey, revision int64, raw []byte,
) (ticket.ContentRef, error) {
	return h.storeTicketContent(ctx, "ticket", key, revision, raw)
}

func (h *TicketHandler) storeTicketContent(
	ctx context.Context, kind string, key domain.TicketKey, revision int64, raw []byte,
) (ticket.ContentRef, error) {
	ref, err := h.content.PutFor(ctx, kind, string(key), revision, raw)
	if err != nil {
		return ticket.ContentRef{}, fmt.Errorf("channel: store ticket context: %w", err)
	}
	return ticket.ContentRef{Ref: ref, Digest: engine.ResultDigest(raw)}, nil
}

func ticketRunKey(ref domain.TicketRef) string {
	return fmt.Sprintf("ticket:%s:%d", ref.Key, ref.Revision)
}

func ticketContextRefusal(err error) TicketResult {
	reason := "ticket_context_invalid"
	if strings.Contains(err.Error(), "too many") || strings.Contains(err.Error(), "too large") {
		reason = "ticket_context_full"
	}
	return TicketResult{Refusal: Refusal{
		Why:    "This ticket has reached its context limit. Open a new root message to continue.",
		Reason: reason,
	}}
}
