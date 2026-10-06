package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/ticket"
)

/*
TicketReviewConsumer opens the thread a ticket is worked in.

A sweep rather than a step inside opening the ticket, for the reason the
reporter is one: a worker that died between writing the ticket and posting its
thread would leave a ticket nobody can review, and a sweep that runs again
cannot. The lease is what stops two workers opening two threads for one ticket;
the thread is recorded after it exists and never before, so a lost process
repeats a message rather than losing the room.

Until the thread exists the ticket's cards have nowhere to go, and the reporter
leaves them unreported rather than falling back to the support thread. That is
the whole point of a review room: nothing reaches the person who asked until
somebody has decided it should.
*/
type TicketReviewConsumer struct {
	rooms  ticket.ReviewRooms
	opener ReviewOpener
	owner  string
	clock  func() time.Time
}

func NewTicketReviewConsumer(
	rooms ticket.ReviewRooms, opener ReviewOpener, owner string,
) *TicketReviewConsumer {
	return &TicketReviewConsumer{
		rooms: rooms, opener: opener, owner: strings.TrimSpace(owner), clock: time.Now,
	}
}

func (c *TicketReviewConsumer) Sweep(
	ctx context.Context, lease time.Duration, limit int,
) (int, error) {
	if c == nil || c.rooms == nil || c.opener == nil || c.owner == "" {
		return 0, fmt.Errorf("%w: governed ticket review rooms", ErrNotWired)
	}
	owed, err := c.rooms.ClaimReviews(ctx, c.owner, c.clock().UTC(), lease, limit)
	if err != nil {
		return 0, err
	}
	opened, failures := 0, []error{}
	for _, notice := range owed {
		if err := c.open(ctx, notice); err != nil {
			// One unreachable room does not hold the others: the rooms belong
			// to different areas, and a bot removed from one of them would
			// otherwise stop every ticket the installation is reviewing.
			failures = append(failures, err)
			continue
		}
		opened++
	}
	return opened, errors.Join(failures...)
}

func (c *TicketReviewConsumer) open(ctx context.Context, notice ticket.ReviewNotice) error {
	ref, err := c.opener.OpenReview(ctx, notice.Origin.Connection, ReviewOpening{
		Conversation: notice.Conversation,
		From:         notice.Origin.Conversation,
		Root:         notice.Origin.Root,
	})
	if err != nil {
		return fmt.Errorf("channel: open review room for ticket %s: %w", notice.Key, err)
	}
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("channel: review room for ticket %s said nothing about where", notice.Key)
	}
	if err := c.rooms.MarkReviewOpened(ctx, notice.Key, ref, c.owner, c.clock().UTC()); err != nil {
		return fmt.Errorf("channel: record review room for ticket %s: %w", notice.Key, err)
	}
	return nil
}
