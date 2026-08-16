package channel

import (
	"context"
	"fmt"
	"time"
)

/*
A ceiling on one correspondent, because an open channel is an open budget
(NT-005 §12.2).

A conversation anybody can mention the bot in is a way for anybody to open
runs. Not maliciously — a channel with three hundred people and a helpful agent
is enough. The only thing standing there otherwise is the scope ceiling, which
is shared with everything else in the area: the first busy morning spends the
month, and the agents nobody was talking to stop.

It counts runs and not money. Money arrives late — a run's cost is known when
it ends, and by then the next twenty have started — and a run's own ceiling
already bounds each one, so a bound on how many bounds the spend.

Not yet a setting. A number nobody can change is a poor knob and a real rail,
and a setting with no screen would be configuration written straight into the
database, which this platform does not do. The screen comes with the setting.
*/

// Ceiling is how many runs one correspondent may open, and over what.
type Ceiling struct {
	Runs int
	Per  time.Duration
}

// Quota is one ceiling resolved against a clock: how many, and since when.
//
// Resolved by the caller because the clock is the caller's. The store is asked
// a question about an instant it is given, never about `now`, or the window
// would move under a test standing still.
type Quota struct {
	Runs  int
	Since time.Time
}

/*
DefaultCeiling is what an installation gets without saying anything.

Twenty an hour is far above a person asking an agent for things and far below a
channel spending an area's month before lunch. It is deliberately not
generous-by-default: the failure it prevents is expensive and the failure it
causes is somebody waiting, being told why, and asking again.
*/
var DefaultCeiling = Ceiling{Runs: 20, Per: time.Hour}

// WithCeiling sets how much one correspondent may open.
func (c *Consumer) WithCeiling(ceiling Ceiling) *Consumer {
	c.ceiling = ceiling
	return c
}

// WithClock is the time the ceiling counts from. Injected, because a window
// measured by `time.Now` inside the decision is a window no test can stand
// still in.
func (c *Consumer) WithClock(clock func() time.Time) *Consumer {
	c.clock = clock
	return c
}

// reasonCeiling is the code a refusal for the ceiling carries. Its own
// constant because two places have to agree on it: the refusal that records it
// and the query that asks whether this person has already been told.
const reasonCeiling = "ceiling"

/*
overCeiling answers with the refusal, or nil when there is room.

Said once. A limit that answers every message it rejects amplifies the flood it
exists to stop: fifty mentions become fifty replies, the conversation is
unreadable, and it is our bot that made it so. The rest are recorded and not
said, so an operator asked why nothing happened can still count them.
*/
func (c *Consumer) overCeiling(ctx context.Context, a Claimed) (*Refusal, error) {
	if c.ceiling.Runs <= 0 {
		return nil, nil
	}
	since := c.clock().Add(-c.ceiling.Per)

	// Taken, not read. The slot is held before the run is opened, so the next
	// worker to decide counts it — which is the whole difference between a
	// ceiling and a suggestion.
	spent, granted, err := c.inbox.Reserve(ctx, a, Quota{Runs: c.ceiling.Runs, Since: since})
	if err != nil {
		// Not a refusal. A reservation that failed is this side failing, and
		// refusing on it would tell somebody they had asked too much on the
		// evidence of a database that was away.
		return nil, err
	}
	if granted {
		return nil, nil
	}

	who := Correspondent{Channel: a.Channel, Account: a.AskedBy}
	told, err := c.inbox.ToldSince(ctx, who, reasonCeiling, since)
	if err != nil {
		return nil, err
	}
	c.log.Info("an ask was over the correspondent ceiling",
		"channel", a.Channel, "spent", spent, "ceiling", c.ceiling.Runs, "told", told)

	return &Refusal{
		Why: fmt.Sprintf(
			"You have started too many runs here recently — %d in the last %s, which is the limit. "+
				"Try again once the oldest of them falls outside that window.",
			spent, c.ceiling.Per),
		Reason: reasonCeiling,
		Silent: told,
	}, nil
}

// Correspondent is one account on one channel — who a ceiling belongs to.
//
// The channel account and not the person behind it: the binding may not
// resolve, and the flood arrives before it does. One person with two accounts
// gets two ceilings, which is the cost of asking a question that can be
// answered from what arrived.
type Correspondent struct {
	Channel string
	Account string
}

/*
Reserve takes a slot against the correspondent's ceiling, or says it is full.

One transaction and one lock, because the alternative is a count that was true
when it was read. The lock is per correspondent rather than per table: two
people asking at once are not competing for anything, and serialising them
would make a busy conversation queue behind itself.

What counts as spent is a run already opened inside the window, plus a slot
another worker is holding right now. Holding, not merely claimed: an ask being
looked at has not been granted anything yet, and counting it would refuse a
person because somebody was thinking about their earlier message.

A reservation goes with the lease. A worker that dies stops renewing, the ask
becomes claimable, and the slot comes back with it — one expiry for both, so
there is no second thing to get wrong.
*/
func (i *Inbox) Reserve(ctx context.Context, a Claimed, q Quota) (int, bool, error) {
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("channel: reserve a slot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialised on this correspondent alone, for the length of this
	// transaction. Everybody else in the conversation decides in parallel.
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`,
		a.Channel+"/"+a.AskedBy); err != nil {
		return 0, false, fmt.Errorf("channel: hold the ceiling for %s: %w", a.AskedBy, err)
	}

	var spent int
	err = tx.QueryRow(ctx, `
		select count(*) from channel_inbox
		where channel = $1 and asked_by = $2
		  and not (conversation = $4 and event_id = $5)
		  and (
		        (status = 'opened' and at > $3)
		     or (status = 'pending' and reserved_at is not null and leased_until > now())
		      )`,
		a.Channel, a.AskedBy, q.Since.UTC(), a.Conversation, a.EventID).Scan(&spent)
	if err != nil {
		return 0, false, fmt.Errorf("channel: count what %s has spent: %w", a.AskedBy, err)
	}
	if spent >= q.Runs {
		return spent, false, nil
	}

	// Ours to take, and only while we still hold the ask. A consumer whose
	// lease lapsed must not reserve against a ceiling on behalf of the
	// consumer that replaced it.
	tag, err := tx.Exec(ctx, `
		update channel_inbox set reserved_at = now()
		where channel = $1 and conversation = $2 and event_id = $3
		  and lease_owner = $4 and status = 'pending'`,
		a.Channel, a.Conversation, a.EventID, a.Owner)
	if err != nil {
		return spent, false, fmt.Errorf("channel: take a slot for %s: %w", a.EventID, err)
	}
	if tag.RowsAffected() == 0 {
		return spent, false, fmt.Errorf("%w: %s", ErrNotClaimed, a.EventID)
	}
	if err := tx.Commit(ctx); err != nil {
		return spent, false, fmt.Errorf("channel: reserve a slot: %w", err)
	}
	return spent, true, nil
}

// ToldSince reports whether this correspondent has already been refused for
// this reason inside the window, said or still owed.
//
// Owed counts. Two refusals recorded before the answering sweep runs would
// otherwise both be owed, and the flood would be answered twice — which is the
// same failure, only quieter.
func (i *Inbox) ToldSince(
	ctx context.Context, who Correspondent, reason string, since time.Time,
) (bool, error) {
	var told bool
	err := i.pool.QueryRow(ctx, `
		select exists (
			select 1 from channel_inbox
			where channel = $1 and asked_by = $2 and status = 'refused'
			  and reason = $3 and at > $4
		)`, who.Channel, who.Account, reason, since.UTC()).Scan(&told)
	if err != nil {
		return false, fmt.Errorf("channel: read what %s was told: %w", who.Account, err)
	}
	return told, nil
}

/*
Owed claims refusals that have been recorded and not yet said.

Saying it is work of its own, and this is what makes it survivable. Recorded
and delivered in one step, the two rules pull against each other: record first
and a driver failure closes the ask with nobody told; deliver first and the
message goes out before ownership is proven, so a worker whose lease lapsed
posts a refusal the worker that replaced it posts again.

Separated, each is simple. The holder of the ask records the refusal — proving
ownership before anything is said — and whoever picks the debt up afterwards
delivers it, once, under a claim of its own.
*/
func (i *Inbox) Owed(
	ctx context.Context, owner string, lease time.Duration, limit int,
) ([]Claimed, error) {
	return i.claim(ctx, owner, lease, limit,
		"status = 'refused' and answered_at is null")
}
