package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/channel/slack"
)

/*
The door an ask arrives at.

Stage 3 of NT-005, and the half of it that is infrastructure rather than
design. Slack wants a 2xx within three seconds and retries what it does not
get, which is why nothing here opens a run: **verify, write it down, commit,
acknowledge.** Opening happens afterwards, from the inbox, and may be a
different process or a later one.

Moving the work off the request would solve the retry and not the crash.
Between acknowledging and opening there is a window, and a process that dies
inside it has told Slack the ask arrived and holds no record that it did — the
sender is satisfied and the question is gone. A failure that reports success is
the worst pair available, and it is the reason this file exists at all rather
than the handler simply calling the opener.
*/

// Arrivals is where an ask waits between arriving and being opened, declared
// here by the consumer.
type Arrivals interface {
	Receive(ctx context.Context, a channel.Arrival) (fresh bool, err error)
}

// WithArrivals wires the inbox.
func (h *ChannelHooks) WithArrivals(inbox Arrivals) *ChannelHooks {
	h.inbox = inbox
	return h
}

// MountEvents wires the path a channel delivers asks to.
//
// Separate from the interaction path on purpose: one carries a decision
// somebody pressed and the other carries something they typed, and they differ
// in everything that matters — what is trusted, what is recorded, and what
// happens on the request.
func (h *ChannelHooks) MountEvents(mux *http.ServeMux) {
	mux.HandleFunc("POST /hooks/channel/{channel}/slack/events", h.slackEvent)
}

func (h *ChannelHooks) slackEvent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("channel")

	// Read once and verify those exact bytes. A verifier that re-read the body
	// would check one set and let the handler act on another.
	body, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit))
	if err != nil {
		http.Error(w, "unreadable", http.StatusBadRequest)
		return
	}

	creds, known := h.bindings.Secrets(r.Context(), name)
	if !known {
		http.Error(w, "no such channel", http.StatusNotFound)
		return
	}
	if err := slack.Verify(r, body, creds.Signing, h.now()); err != nil {
		h.log.Warn("a channel event was refused", "channel", name, "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	delivery, err := slack.ReadDelivery(body)
	switch {
	case errors.Is(err, slack.ErrNotAnAsk):
		// Well-formed and not for us. Acknowledged, because a retry would
		// deliver the same non-ask forever.
		w.WriteHeader(http.StatusOK)
		return
	case err != nil:
		http.Error(w, "unreadable payload", http.StatusBadRequest)
		return
	}

	// The handshake, answered only after the signature checked out: it is the
	// one request where replying to an unverified caller would prove this
	// endpoint exists to anybody who guesses the path.
	if delivery.Challenge != "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"challenge": delivery.Challenge})
		return
	}

	if h.inbox == nil {
		// No inbox is a platform that cannot promise to remember the ask, and
		// acknowledging one it will lose is worse than making Slack retry.
		h.log.Error("an ask arrived with no inbox to hold it", "channel", name)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	fresh, err := h.inbox.Receive(r.Context(), channel.Arrival{
		Channel: name, Conversation: delivery.Conversation,
		EventID: delivery.EventID, Payload: body,
	})
	if err != nil {
		// Refused rather than acknowledged. Slack retrying is the correct
		// outcome of a write that did not happen — this is the one failure
		// where the sender doing it again is what we want.
		h.log.Error("an ask could not be written down", "channel", name, "err", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}

	// Written and committed. Only now.
	h.log.Info("an ask arrived", "channel", name,
		"conversation", delivery.Conversation, "event", delivery.EventID, "fresh", fresh)
	w.WriteHeader(http.StatusOK)
}
