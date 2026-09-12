package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/ticket"
)

type TicketOutcomeRenderer interface {
	RenderTicketOutcome(ticket.Phase, []byte) (string, error)
}

// TicketOutcomeConsumer pays the final reply owed by a terminal governed
// effect. It is separate from model answers: connector code owns the wording
// and the only bytes it receives are the connector's safe projection.
type TicketOutcomeConsumer struct {
	notices  ticket.OutcomeNotices
	content  engine.ContentStore
	answers  Answers
	renderer TicketOutcomeRenderer
	owner    string
	clock    func() time.Time
}

func NewTicketOutcomeConsumer(
	notices ticket.OutcomeNotices, content engine.ContentStore, answers Answers,
	renderer TicketOutcomeRenderer, owner string,
) *TicketOutcomeConsumer {
	return &TicketOutcomeConsumer{
		notices: notices, content: content, answers: answers,
		renderer: renderer, owner: strings.TrimSpace(owner), clock: time.Now,
	}
}

func (c *TicketOutcomeConsumer) Sweep(
	ctx context.Context, lease time.Duration, limit int,
) (int, error) {
	if c == nil || c.notices == nil || c.content == nil || c.answers == nil ||
		c.renderer == nil || c.owner == "" {
		return 0, fmt.Errorf("%w: governed ticket outcomes", ErrNotWired)
	}
	now := c.clock().UTC()
	owed, err := c.notices.ClaimOutcomes(ctx, c.owner, now, lease, limit)
	if err != nil {
		return 0, err
	}
	said, failures := 0, []error{}
	for _, notice := range owed {
		if err := c.say(ctx, notice); err != nil {
			failures = append(failures, err)
			continue
		}
		said++
	}
	return said, errors.Join(failures...)
}

func (c *TicketOutcomeConsumer) say(ctx context.Context, notice ticket.OutcomeNotice) error {
	raw, err := c.content.Get(ctx, notice.Result.Ref)
	if err != nil || engine.ResultDigest(raw) != notice.Result.Digest {
		return fmt.Errorf("channel: read safe outcome for ticket %s: unavailable", notice.Ref.Key)
	}
	message, err := c.renderer.RenderTicketOutcome(notice.Phase, raw)
	if err != nil || strings.TrimSpace(message) == "" {
		return fmt.Errorf("channel: render safe outcome for ticket %s: invalid", notice.Ref.Key)
	}
	if err := c.answers.ReplyOutcome(ctx, notice.Origin.Connection,
		notice.Origin.Conversation, notice.Origin.Root, message); err != nil {
		return fmt.Errorf("channel: reply to governed ticket %s: %w", notice.Ref.Key, err)
	}
	if err := c.notices.MarkOutcomeAnnounced(ctx, notice.Ref, c.owner, c.clock().UTC()); err != nil {
		return fmt.Errorf("channel: settle governed ticket %s: %w", notice.Ref.Key, err)
	}
	return nil
}
