package channel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

/*
Claim takes asks nobody else is working on.

Listing what is pending and acting on it is not a claim. Two consumers read the
same row, both open a run and both reply, and the person who asked is answered
twice by an agent that ran twice. The opener's idempotency key would catch the
duplicate *run* — and the second reply, the second refusal and the second entry
in the record are not runs, and it catches none of them.

A lease with an owner, which is the shape the run queue already uses. An
expired lease is claimable again: a consumer that dies stops renewing and the
next sweep picks the ask up, so there is no reaper to write and none to forget.

Oldest first, because an ask that arrived earlier was asked earlier and
answering out of order in a conversation reads as the platform ignoring
somebody.
*/
func (i *Inbox) Claim(
	ctx context.Context, owner string, lease time.Duration, limit int,
) ([]Arrival, error) {
	rows, err := i.pool.Query(ctx, `
		update channel_inbox set
			leased_until = now() + $2::interval,
			lease_owner  = $1
		where (channel, conversation, event_id) in (
			select channel, conversation, event_id from channel_inbox
			where status = 'pending'
			  and (leased_until is null or leased_until <= now())
			order by at
			for update skip locked
			limit $3
		)
		returning channel, conversation, event_id, payload`,
		owner, lease.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("channel: claim from the inbox: %w", err)
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

// ErrNotClaimed means somebody else settled this ask, or the lease expired and
// another consumer took it. Reported rather than swallowed: a consumer that
// carries on after losing its claim is the second reply this whole mechanism
// exists to prevent.
var ErrNotClaimed = errors.New("channel: this ask is not ours to settle")

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

// settle closes an ask, and only one that is still pending.
//
// Conditioned on the status and checked afterwards: without the condition a
// consumer whose lease expired would overwrite the outcome the consumer that
// took over had already written, and without the check it would not know.
func (i *Inbox) settle(
	ctx context.Context, a Arrival, status, detail, run string, at time.Time,
) error {
	tag, err := i.pool.Exec(ctx, `
		update channel_inbox
		set status = $4, detail = $5, run_id = $6, at = $7,
		    leased_until = null, lease_owner = ''
		where channel = $1 and conversation = $2 and event_id = $3
		  and status = 'pending'`,
		a.Channel, a.Conversation, a.EventID, status, detail, run, at.UTC())
	if err != nil {
		return fmt.Errorf("channel: settle %s: %w", a.EventID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrNotClaimed, a.EventID)
	}
	return nil
}
