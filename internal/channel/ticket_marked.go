package channel

import (
	"context"
	"fmt"
)

/*
Opening a ticket somebody else's bot wrote the root of.

Under `marked_threads` the mark admits the thread and the root is the request.
Neither the text nor the person who asked arrives with the mark, so both are
read here, where the connection can be asked — and the root is trusted only
when its author is the source the rule named.

The author is named inside the root because the message is the form bot's, not
theirs. That mention is evidence and not authority: it is resolved against the
accounts bound to this connection, and a ticket whose requester cannot be
resolved is not opened. Who may decide anything is unchanged — the scope's
approvers, narrowed by the addressing source.
*/

// WithThreads gives the handler the thread reader it needs to open marked
// threads. Optional: an installation that configures no marked room never
// calls it, and a marked arrival without it fails closed rather than opening a
// ticket whose request nobody read.
func (h *TicketHandler) WithThreads(threads ThreadReader) *TicketHandler {
	h.threads = threads
	return h
}

func (h *TicketHandler) openMarked(ctx context.Context, arrival Claimed) (TicketResult, error) {
	if h.threads == nil {
		return TicketResult{}, fmt.Errorf("%w: a thread reader", ErrNotWired)
	}
	root, found, err := h.markedRoot(ctx, arrival)
	if err != nil {
		return TicketResult{}, err
	}
	if !found {
		// The root is gone, or the thread cannot be read as one. Recorded and
		// finished: replaying it would ask the same unanswerable question.
		return handled("ticket_root_unreadable"), nil
	}
	if !sameSourceKey(root.Source, arrival.Ticket.RootFrom) {
		return handled("ticket_root_not_admitted"), nil
	}
	account, named := firstMentioned(root.Text)
	if !named {
		// Nobody to be the requester. A different sentence from an unlinked
		// account: this is the form's shape, and no amount of linking fixes it.
		return TicketResult{Refusal: Refusal{
			Why: "The first message of this thread names nobody, so there is no requester to open a " +
				"ticket for. The form has to say who asked, as a Slack mention.",
			Reason: "ticket_requester_unnamed",
		}}, nil
	}
	who, linked, err := h.bindings.PrincipalFor(ctx, arrival.Channel, account)
	if err != nil {
		return TicketResult{}, fmt.Errorf("channel: resolve marked ticket requester: %w", err)
	}
	if !linked {
		return unboundRequester(), nil
	}
	return h.open(ctx, arrival, who, root.Ref, root.Text)
}

// markedRoot reads the message the thread hangs from. Bounded by the reader:
// what comes back is already trimmed evidence, not a Slack payload.
func (h *TicketHandler) markedRoot(
	ctx context.Context, arrival Claimed,
) (ThreadMessage, bool, error) {
	thread, err := h.threads.Thread(ctx, arrival.Channel, arrival.Conversation, arrival.Thread, "")
	if err != nil {
		return ThreadMessage{}, false, fmt.Errorf("channel: read marked thread: %w", err)
	}
	for _, message := range thread.Messages {
		if message.Ref == arrival.Thread {
			return message, true, nil
		}
	}
	return ThreadMessage{}, false, nil
}
