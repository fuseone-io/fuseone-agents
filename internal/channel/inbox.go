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
	// Message is what the channel calls the message somebody typed. Separate
	// from EventID because they are separate things: one names a delivery and
	// repeats on retry, the other names what was said and is what a thread is
	// keyed by.
	Message string

	/*
		The ask as the platform understands it, read by the door.

		The door is the only layer that knows a vendor's shape and it has
		already read the message by the time it writes the row. Leaving the
		consumer to parse the payload again would give Slack's format a second
		reader, and a second reader of a format is where the two readings start
		to disagree.
	*/
	AskedBy string
	Text    string
	// Thread is where an answer belongs: the parent when the ask came inside
	// one, the message itself when it started one.
	Thread string

	// Payload is what actually arrived. It is what the digest is of, and what
	// an auditor reads when they want the thing itself rather than what we
	// made of it.
	Payload []byte
}

/*
Claimed is an ask this consumer holds, and the proof that it does.

The owner travels with the ask rather than being remembered by the caller,
because settling is where it matters and a caller that has to pass it
separately is a caller that will pass the wrong one. A lease with a holder that
nothing checks at the end is a lease in name: worker-1's lease lapses, worker-2
takes the ask, and worker-1 — still running, still holding what it read —
settles it and replies. The row was pending, so it succeeded.

That is the duplicated attention the claim exists to prevent, arriving through
the claim itself.
*/
type Claimed struct {
	Arrival
	Owner string
	// Detail is what was decided about this ask, for a debt being delivered.
	// Empty while the ask is still pending: there is nothing decided yet.
	Detail string
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
		insert into channel_inbox
			(channel, conversation, event_id, message, asked_by, text, thread, payload, digest)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		on conflict (channel, conversation, event_id) do nothing`,
		a.Channel, a.Conversation, a.EventID, a.Message,
		a.AskedBy, a.Text, a.Thread, a.Payload,
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
) ([]Claimed, error) {
	return i.claim(ctx, owner, lease, limit, "status = 'pending'")
}

// claim takes rows matching a state, leases them, and hands them back.
//
// The state is a literal in the two callers above and never a caller's string:
// this builds SQL, and a predicate that could arrive from anywhere is an
// injection with a comment explaining that it cannot be.
func (i *Inbox) claim(
	ctx context.Context, owner string, lease time.Duration, limit int, state string,
) ([]Claimed, error) {
	rows, err := i.pool.Query(ctx, `
		update channel_inbox set
			leased_until = now() + $2::interval,
			lease_owner  = $1
		where (channel, conversation, event_id) in (
			select channel, conversation, event_id from channel_inbox
			where `+state+`
			  and (leased_until is null or leased_until <= now())
			order by at
			for update skip locked
			limit $3
		)
		returning channel, conversation, event_id, message,
		          asked_by, text, thread, payload, detail`,
		owner, lease.String(), limit)
	if err != nil {
		return nil, fmt.Errorf("channel: claim from the inbox: %w", err)
	}
	defer rows.Close()

	var out []Claimed
	for rows.Next() {
		var a Arrival
		var detail string
		if err := rows.Scan(&a.Channel, &a.Conversation, &a.EventID, &a.Message,
			&a.AskedBy, &a.Text, &a.Thread, &a.Payload, &detail); err != nil {
			return nil, err
		}
		out = append(out, Claimed{Arrival: a, Owner: owner, Detail: detail})
	}
	return out, rows.Err()
}

// Opened records which run an ask became.
func (i *Inbox) Opened(ctx context.Context, c Claimed, run string, at time.Time) error {
	return i.settle(ctx, c, settled{Status: "opened", Run: run}, at)
}

// ErrNotClaimed means this ask is somebody else's now: already settled, or
// reclaimed after this consumer's lease lapsed.
//
// Reported rather than swallowed. A consumer that carries on after losing its
// claim is the second reply this whole mechanism exists to prevent, and it is
// the only warning it will get.
var ErrNotClaimed = errors.New("channel: this ask is not ours to settle")

/*
Refused records that an ask became nothing, and why.

Kept rather than deleted. "Somebody mentioned an agent that cannot be started
here" is exactly what an operator needs when the person says nothing happened,
and a row removed on refusal makes that conversation unanswerable — the ask is
gone and the platform has no memory of ever having been asked.
*/
func (i *Inbox) Refused(ctx context.Context, c Claimed, r Refusal, at time.Time) error {
	return i.settle(ctx, c, settled{
		Status: "refused", Detail: r.Why, Reason: r.Reason,
		// A silent refusal is recorded as already said. It is not owed to
		// anybody, so it is never claimed, and the row still answers an
		// operator asking what happened.
		Answered: r.Silent,
	}, at)
}

/*
Refusal is an ask that became nothing.

Three parts because three readers. The sentence is for the person who asked and
names what would have worked. The reason is for whoever is counting later, and
a sentence with somebody's agent name in it counts nothing. And Silent is for
the conversation itself: the second refusal of the same kind inside the same
window is recorded and not said, because a limit that answers every message it
rejects amplifies the flood it exists to stop.
*/
type Refusal struct {
	Why    string
	Reason string
	Silent bool
}

// Answered marks a refusal as said.
//
// Owner-checked like every other settle: a consumer that lost the debt must not
// record somebody else's delivery. And conditioned on the row being a refusal,
// because Claimed serves both an ask and a debt — the table defends its own
// shape rather than trusting every caller to have kept the two apart.
func (i *Inbox) Answered(ctx context.Context, c Claimed, at time.Time) error {
	tag, err := i.pool.Exec(ctx, `
		update channel_inbox
		set answered_at = $5, leased_until = null, lease_owner = ''
		where channel = $1 and conversation = $2 and event_id = $3
		  and lease_owner = $4 and status = 'refused' and answered_at is null`,
		c.Channel, c.Conversation, c.EventID, c.Owner, at.UTC())
	if err != nil {
		return fmt.Errorf("channel: mark %s answered: %w", c.EventID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrNotClaimed, c.EventID)
	}
	return nil
}

// settled is what a settle writes. A struct because the row has more parts
// than a parameter list should carry, and because two of them are easy to
// swap: `detail` and `reason` are both strings, and a caller that transposed
// them would tell the person a code and the operator a sentence.
type settled struct {
	Status   string
	Detail   string
	Reason   string
	Run      string
	Answered bool
}

/*
settle closes an ask, and only one this consumer still holds.

Pending *and* ours. Status alone is not enough: a lease that lapsed leaves the
row pending, so the consumer that lost it would close the ask the consumer that
took over is working on — and then reply. Both conditions, and the row count
checked afterwards, because a settle that quietly did nothing is a consumer
that believes it finished.
*/
func (i *Inbox) settle(ctx context.Context, c Claimed, s settled, at time.Time) error {
	var answered *time.Time
	if s.Answered {
		when := at.UTC()
		answered = &when
	}
	tag, err := i.pool.Exec(ctx, `
		update channel_inbox
		set status = $5, detail = $6, run_id = $7, at = $8, reason = $9,
		    answered_at = $10, leased_until = null, lease_owner = ''
		where channel = $1 and conversation = $2 and event_id = $3
		  and status = 'pending' and lease_owner = $4`,
		c.Channel, c.Conversation, c.EventID, c.Owner,
		s.Status, s.Detail, s.Run, at.UTC(), s.Reason, answered)
	if err != nil {
		return fmt.Errorf("channel: settle %s: %w", c.EventID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrNotClaimed, c.EventID)
	}
	return nil
}
