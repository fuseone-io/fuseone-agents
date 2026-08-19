package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/channel"
)

/*
Socket Mode for Slack Events API.

HTTP and Socket Mode differ only in how Slack delivers the same event payload.
The read path below deliberately converges before anything platform-specific
happens: the payload is still parsed by ReadDelivery and written to the same
channel inbox. That keeps idempotency, taint and refusal behaviour in one
place rather than inventing a second Slack trigger.
*/

const SocketAPI = API

// SocketEnvelope is one frame Slack sends over Socket Mode.
type SocketEnvelope struct {
	Type       string          `json:"type"`
	EnvelopeID string          `json:"envelope_id"`
	Payload    json.RawMessage `json:"payload"`
	Reason     string          `json:"reason"`
}

// SocketAck is what Slack requires before it stops retrying an envelope.
type SocketAck struct {
	EnvelopeID string `json:"envelope_id"`
}

func ReadSocketEnvelope(body []byte) (SocketEnvelope, error) {
	var e SocketEnvelope
	if err := json.Unmarshal(body, &e); err != nil {
		return SocketEnvelope{}, fmt.Errorf("slack: unreadable socket envelope: %w", err)
	}
	return e, nil
}

func AckSocketEnvelope(envelopeID string) ([]byte, error) {
	raw, err := json.Marshal(SocketAck{EnvelopeID: envelopeID})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// SocketArrivals is where an ask waits between arriving and being opened.
type SocketArrivals interface {
	Receive(ctx context.Context, a channel.Arrival) (fresh bool, err error)
}

// WatchRules answers whether an ordinary channel message may become a run.
type WatchRules interface {
	WatchFor(
		ctx context.Context, channelName, conversation string, source channel.Source,
	) (channel.WatchRule, bool, error)
}

// SeenAccounts records account hints for administrators. It grants nothing,
// so failure here is logged and the ask path continues.
type SeenAccounts interface {
	MarkAccountSeen(ctx context.Context, channelName, account, conversation string, at time.Time) error
}

// SocketReceiver handles one Slack Socket Mode frame.
type SocketReceiver struct {
	Channel string
	Inbox   SocketArrivals
	Rules   WatchRules
	Seen    SeenAccounts
	Now     func() time.Time
	Log     *slog.Logger
}

// Handle returns the acknowledgement to write back, or nil for frames that do
// not require one. A nil ack with an error means "do not acknowledge": Slack
// should retry because the platform did not durably record the ask.
func (r SocketReceiver) Handle(ctx context.Context, body []byte) ([]byte, error) {
	log := r.Log
	if log == nil {
		log = slog.Default()
	}

	envelope, err := ReadSocketEnvelope(body)
	if err != nil {
		return nil, err
	}

	switch envelope.Type {
	case "hello":
		return nil, nil
	case "disconnect":
		return nil, fmt.Errorf("slack: socket disconnected: %s", envelope.Reason)
	case "events_api":
		return r.handleEvent(ctx, envelope, log)
	default:
		if envelope.EnvelopeID == "" {
			log.Warn("slack socket sent an envelope this worker does not handle",
				"channel", r.Channel, "type", envelope.Type)
			return nil, nil
		}
		log.Warn("slack socket envelope ignored",
			"channel", r.Channel, "type", envelope.Type)
		return AckSocketEnvelope(envelope.EnvelopeID)
	}
}

func (r SocketReceiver) handleEvent(
	ctx context.Context, envelope SocketEnvelope, log *slog.Logger,
) ([]byte, error) {
	if envelope.EnvelopeID == "" {
		return nil, errors.New("slack: socket event without an envelope id")
	}

	delivery, err := ReadAnyDelivery(envelope.Payload)
	switch {
	case errors.Is(err, ErrNotAnAsk):
		return AckSocketEnvelope(envelope.EnvelopeID)
	case errors.Is(err, ErrMalformedAsk):
		// Socket Mode has no HTTP status to return. Retrying the same malformed
		// Slack payload will not create the missing channel or timestamp, so it
		// is logged and acknowledged rather than amplified forever.
		log.Warn("a socket mention arrived with nothing to act on",
			"channel", r.Channel, "err", err)
		return AckSocketEnvelope(envelope.EnvelopeID)
	case err != nil:
		log.Warn("a socket event was unreadable",
			"channel", r.Channel, "err", err)
		return AckSocketEnvelope(envelope.EnvelopeID)
	}

	if delivery.Challenge != "" {
		return AckSocketEnvelope(envelope.EnvelopeID)
	}
	if delivery.Kind == DeliveryMention {
		r.markSeen(ctx, delivery, log)
	}
	if r.Inbox == nil {
		return nil, errors.New("slack: socket ask arrived with no inbox")
	}

	arrival := channel.Arrival{
		Channel: r.Channel, Conversation: delivery.Conversation,
		EventID: delivery.EventID, Message: delivery.Message,
		AskedBy: delivery.User, Text: delivery.Text, Thread: delivery.Thread,
		Source: delivery.Source, Payload: envelope.Payload,
	}
	if delivery.Kind == DeliveryMessage {
		if r.Rules == nil {
			return AckSocketEnvelope(envelope.EnvelopeID)
		}
		rule, ok, err := r.Rules.WatchFor(ctx, r.Channel, delivery.Conversation, delivery.Source)
		if err != nil {
			return nil, fmt.Errorf("slack: read watch rule: %w", err)
		}
		if !ok {
			return AckSocketEnvelope(envelope.EnvelopeID)
		}
		arrival.Agent = rule.Agent
		arrival.RunAs = rule.RunAs
		arrival.AskedBy = delivery.Source.Key()
	}
	if arrival.AskedBy == "" {
		return AckSocketEnvelope(envelope.EnvelopeID)
	}

	fresh, err := r.Inbox.Receive(ctx, channel.Arrival{
		Channel: arrival.Channel, Conversation: arrival.Conversation,
		EventID: arrival.EventID, Message: arrival.Message,
		AskedBy: arrival.AskedBy, Text: arrival.Text, Thread: arrival.Thread,
		Agent: arrival.Agent, RunAs: arrival.RunAs, Source: arrival.Source,
		Payload: arrival.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("slack: record socket ask: %w", err)
	}

	log.Info("an ask arrived over slack socket",
		"channel", r.Channel, "conversation", delivery.Conversation,
		"event", delivery.EventID, "fresh", fresh)
	return AckSocketEnvelope(envelope.EnvelopeID)
}

func (r SocketReceiver) markSeen(ctx context.Context, delivery Delivery, log *slog.Logger) {
	if r.Seen == nil || strings.TrimSpace(delivery.User) == "" {
		return
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}
	if err := r.Seen.MarkAccountSeen(
		ctx, r.Channel, delivery.User, delivery.Conversation, now(),
	); err != nil {
		log.Warn("a channel account could not be marked as seen",
			"channel", r.Channel, "account", delivery.User, "err", err)
	}
}

// OpenSocketURL asks Slack for a one-use WebSocket URL.
func OpenSocketURL(ctx context.Context, appToken, base string, client *http.Client) (string, error) {
	if strings.TrimSpace(appToken) == "" {
		return "", errors.New("slack: socket mode needs an app-level token")
	}
	if client == nil {
		client = http.DefaultClient
	}
	if base == "" {
		base = SocketAPI
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(base, "/")+"/apps.connections.open", bytes.NewReader(nil))
	if err != nil {
		return "", fmt.Errorf("slack: build socket URL request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("slack: open socket URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var answer struct {
		OK    bool   `json:"ok"`
		URL   string `json:"url"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return "", fmt.Errorf("slack: read socket URL answer (status %d): %w", resp.StatusCode, err)
	}
	if !answer.OK || answer.URL == "" {
		return "", fmt.Errorf("slack: socket URL refused: %s", answer.Error)
	}
	return answer.URL, nil
}
