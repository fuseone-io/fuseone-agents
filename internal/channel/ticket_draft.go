package channel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

const (
	MaxTicketDraftMessages = 32
	MaxTicketDraftBytes    = 32 << 10
)

type ticketDraft struct {
	Messages []ticketDraftMessage `json:"messages"`
}

type ticketDraftMessage struct {
	Ref  string        `json:"ref"`
	By   domain.UserID `json:"by"`
	Text string        `json:"text"`
}

func firstTicketDraft(ref string, by domain.UserID, text string) ([]byte, error) {
	return encodeTicketDraft(ticketDraft{Messages: []ticketDraftMessage{{
		Ref: ref, By: by, Text: strings.TrimSpace(text),
	}}})
}

func appendTicketDraft(raw []byte, ref string, by domain.UserID, text string) ([]byte, error) {
	draft, err := decodeTicketDraft(raw)
	if err != nil {
		return nil, err
	}
	draft.Messages = append(draft.Messages, ticketDraftMessage{
		Ref: ref, By: by, Text: strings.TrimSpace(text),
	})
	return encodeTicketDraft(draft)
}

func encodeTicketDraft(draft ticketDraft) ([]byte, error) {
	if len(draft.Messages) == 0 || len(draft.Messages) > MaxTicketDraftMessages {
		return nil, errors.New("channel: ticket context has too many messages")
	}
	for _, message := range draft.Messages {
		if strings.TrimSpace(message.Ref) == "" || message.By == "" ||
			strings.TrimSpace(message.Text) == "" {
			return nil, errors.New("channel: ticket context contains an empty message")
		}
	}
	raw, err := json.Marshal(draft)
	if err != nil {
		return nil, fmt.Errorf("channel: encode ticket context: %w", err)
	}
	if len(raw) > MaxTicketDraftBytes {
		return nil, errors.New("channel: ticket context is too large")
	}
	return raw, nil
}

func decodeTicketDraft(raw []byte) (ticketDraft, error) {
	if len(raw) == 0 || len(raw) > MaxTicketDraftBytes {
		return ticketDraft{}, errors.New("channel: ticket context is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var draft ticketDraft
	if err := decoder.Decode(&draft); err != nil {
		return ticketDraft{}, errors.New("channel: ticket context is unreadable")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ticketDraft{}, errors.New("channel: ticket context has trailing data")
	}
	if _, err := encodeTicketDraft(draft); err != nil {
		return ticketDraft{}, err
	}
	return draft, nil
}
