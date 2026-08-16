package channel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

/*
Asks that arrived from a conversation, written down before they are
acknowledged.

A channel wants an answer in seconds and retries what it does not get, so a run
cannot be opened on the request that delivers the ask. Moving the work off the
request only solves the retry: between acknowledging and opening there is a
window, and a process that dies inside it has told the sender the ask arrived
and holds no record that it did. The sender is satisfied and the question is
gone.

So the order is: verify, write, commit, acknowledge. This is that write, and it
is the piece most likely to be skipped for looking like plumbing.
*/

// Arrival is one ask, as it came off the wire.
type Arrival struct {
	Channel      string
	Conversation string
	// EventID is the sender's own identifier for this delivery. Theirs and not
	// ours, because the sender is the only party who knows that two deliveries
	// are the same delivery.
	EventID string
	Payload []byte
}

// Inbox is where an ask waits between arriving and being opened.
type Inbox struct{ pool *pgxpool.Pool }

func NewInbox(pool *pgxpool.Pool) *Inbox { return &Inbox{pool: pool} }

/*
Receive writes an ask down and answers whether it is new.

A redelivery is not an error and not a second ask: the insert conflicts, and
false comes back so the caller acknowledges without queueing anything. Every
sender in existence redelivers, and a channel that opened a second run for the
same message would be a channel nobody could use twice.
*/
func (i *Inbox) Receive(ctx context.Context, a Arrival) (fresh bool, err error) {
	sum := sha256.Sum256(a.Payload)

	tag, err := i.pool.Exec(ctx, `
		insert into channel_inbox (channel, conversation, event_id, payload, digest)
		values ($1, $2, $3, $4, $5)
		on conflict (channel, conversation, event_id) do nothing`,
		a.Channel, a.Conversation, a.EventID, a.Payload,
		"sha256:"+hex.EncodeToString(sum[:8]))
	if err != nil {
		return false, fmt.Errorf("channel: receive %s: %w", a.EventID, err)
	}
	return tag.RowsAffected() == 1, nil
}

// Waiting is what has arrived and not been opened, oldest first.
//
// Oldest first because an ask that arrived earlier was asked earlier, and
// answering out of order in a conversation reads as the platform ignoring
// somebody.
func (i *Inbox) Waiting(ctx context.Context, limit int) ([]Arrival, error) {
	rows, err := i.pool.Query(ctx, `
		select channel, conversation, event_id, payload
		from channel_inbox
		where status = 'pending'
		order by at
		limit $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("channel: read the inbox: %w", err)
	}
	defer rows.Close()

	var out []Arrival
	for rows.Next() {
		var a Arrival
		if err := rows.Scan(&a.Channel, &a.Conversation, &a.EventID, &a.Payload); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Opened records which run an ask became.
func (i *Inbox) Opened(ctx context.Context, a Arrival, run string, at time.Time) error {
	return i.settle(ctx, a, "opened", "", run, at)
}

/*
Refused records that an ask became nothing, and why.

Kept rather than deleted. "Somebody mentioned an agent that cannot be started
here" is exactly what an operator needs when the person says nothing happened,
and a row removed on refusal makes that conversation unanswerable — the ask is
gone and the platform has no memory of ever having been asked.
*/
func (i *Inbox) Refused(ctx context.Context, a Arrival, why string, at time.Time) error {
	return i.settle(ctx, a, "refused", why, "", at)
}

func (i *Inbox) settle(
	ctx context.Context, a Arrival, status, detail, run string, at time.Time,
) error {
	_, err := i.pool.Exec(ctx, `
		update channel_inbox
		set status = $4, detail = $5, run_id = $6, at = $7
		where channel = $1 and conversation = $2 and event_id = $3`,
		a.Channel, a.Conversation, a.EventID, status, detail, run, at.UTC())
	if err != nil {
		return fmt.Errorf("channel: settle %s: %w", a.EventID, err)
	}
	return nil
}
