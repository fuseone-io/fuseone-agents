package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

/*
Reading what has happened and remembering what was said.

The question the sweep asks is "what has not been reported", not "what changed
in the last five minutes". A window would drop a run that parked while the
process was away, and the run it drops is the one somebody is waiting on. The
window here only bounds the first sweep after a conversation is configured, so
that turning one on does not replay a year into it.
*/
type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

// phases maps what the projection calls a run's state to what a person would
// want to be told about it.
const phases = `
	case runs.phase
		when 'awaiting_approval' then 'parked'
		when 'parked'            then 'parked'
		when 'failed'            then 'failed'
		when 'finished'          then 'finished'
	end`

/*
announcementSeq is which stop this announcement is about.

A run stops as many times as it needs to, and each stop is its own question.
Only one of the two ways of stopping leaves a sequence on the projection:
pending_at_seq is written for an approval request and cleared by everything
else, so a budget park and a retry that stopped helping have none. Read from
that column alone, every such stop is filed under zero and the second one is
announced to nobody.

So the stop itself answers where nothing was asked: a parked run appends
nothing, and last_seq is the step that stopped it. Failing and finishing happen
once, and carry zero.

Written once and used by both the report and the anti-join that retires it. Two
copies would let a run be announced under one sequence and retired under
another, which is silence that looks like success.
*/
const announcementSeq = `
	case runs.phase
		when 'awaiting_approval' then coalesce(runs.pending_at_seq, runs.last_seq)
		when 'parked'            then runs.last_seq
		else 0
	end`

/*
nextAttempt is how long a run that could not be announced waits.

Doubling, from a floor, to a ceiling — and the whole schedule sits inside the
24-hour window, so a run is retried many times before it leaves and nothing here
is ever "give up". Read as: this run may be tried again once its last attempt is
older than that.

Two schedules, because two failures are not alike. A destination refusing is an
incident and somebody is probably fixing it, so the first retry is a minute
away. Nothing configured to hear the run at all is an installation part-way
through being set up: the same run coming back every thirty seconds writes
2,880 attempts a day into a table nobody is reading, and the fix is a person
doing something, not a network recovering.

Written here rather than as a column, because it is a policy and a column is a
value: changing it would leave every row already scheduled by the old one.
*/
const nextAttempt = `
	tried.last_seen + case when tried.unconfigured
		then least(interval '6 hours',
		           interval '15 minutes' * power(2, least(coalesce(tried.attempts, 1) - 1, 5)))
		else least(interval '2 hours',
		           interval '1 minute' * power(2, least(coalesce(tried.attempts, 1) - 1, 7)))
	end`

/*
Unreported lists runs in a state worth announcing that has not been said
everywhere it should be.

"Everywhere" is not a question this projection can answer: a run is announced
to every conversation that speaks for its scope, and the map from scope to
conversations is configuration this query knows nothing about. So a single
delivery row cannot clear a run — read that way, a conversation the bot had
been removed from was never retried, silently, which is the exact failure the
sweep exists to prevent.

What clears it is the reporter saying so, once it has been to every
conversation without a failure. Dedup per conversation still happens at the
post, so a retry after a partial success repeats nothing.

Simulated runs are excluded. Nobody is waiting on one, and an approval request
for a rehearsal would teach people to ignore the channel.
*/
func (p *Postgres) Unreported(ctx context.Context, since time.Time, limit int) ([]Report, error) {
	rows, err := p.pool.Query(ctx, `
		select runs.run_id, runs.agent_id, runs.version_id, runs.company_id, runs.area_id,
		       `+phases+` as event, runs.updated_at,
		       coalesce(runs.pending_tool, ''), coalesce(runs.pending_reason, ''),
		       `+announcementSeq+`,
		       runs.phase = 'awaiting_approval'
		from runs
		-- What has already been attempted, so a page that failed goes to the
		-- back. Ordered by recency alone, more stopped runs than fit in one
		-- sweep meant the same page came back forever: one destination
		-- refusing every message keeps every run in it pending, and the runs
		-- below the cut were never tried anywhere — not even in the
		-- conversations that were answering — until they left the window a day
		-- later, unannounced.
		-- The most recent failure, and only it.
		--
		-- Aggregated over every failure ever recorded about this announcement,
		-- the schedule read a history rather than a state: six old attempts at
		-- "nothing is configured" and one refusal two minutes ago produced six
		-- attempts on the incident schedule — a run waiting half an hour to be
		-- retried because of a problem somebody had already fixed. What decides
		-- how long to wait is what went wrong last time, and how many times
		-- that has gone wrong.
		--
		-- "That" is one cause, and a cause is one row of this table: a
		-- destination and a normalised operational code, about one
		-- announcement. The count belongs to the cause and is never reset by
		-- another one happening in between — a destination that refused six
		-- times, went unreachable for some other reason, and now refuses again
		-- is refusing for the seventh time. Nothing about the interruption
		-- makes the first six untrue. The ceilings below are what keeps an
		-- inherited count from doubling a run out of the window.
		left join lateral (
		    select f.last_seen,
		           f.attempts,
		           f.code = '`+CodeNowhereToSayIt+`' as unconfigured
		    from channel_delivery_failures f
		    -- Per question, not per run. Two approvals in one run are both
		    -- "parked", so matching the event alone put a run's second
		    -- question at the back of the queue because its first could not be
		    -- delivered — behind runs nobody had ever tried. The identity of
		    -- an announcement is the step it is about, which is what both
		    -- delivery tables are keyed by.
		    where f.run_id = runs.run_id and f.event = `+phases+`
		      and f.at_seq = `+announcementSeq+`
		    order by f.last_seen desc, f.attempts desc
		    limit 1
		) tried on true
		where not runs.simulated
		  and runs.updated_at >= $1
		  and `+phases+` is not null
		  and not exists (
		      select 1 from channel_deliveries d
		      where d.run_id = runs.run_id and d.event = `+phases+`
		        and d.channel = '' and d.conversation = ''
		        and d.at_seq = `+announcementSeq+`)
		  -- Waited long enough. Compared against now, not against the attempt
		  -- itself: an attempt is always older than itself plus a wait, which
		  -- is a clause that reads like a schedule and lets everything through.
		  and (tried.last_seen is null or `+nextAttempt+` <= now())
		order by coalesce(tried.last_seen, to_timestamp(0)) asc, runs.updated_at desc
		limit $2`, since.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("channel: unreported runs: %w", err)
	}
	defer rows.Close()

	var out []Report
	for rows.Next() {
		var r Report
		var company, area, event string
		if err := rows.Scan(&r.RunID, &r.AgentID, &r.Version, &company, &area,
			&event, &r.At, &r.Tool, &r.Reason, &r.AtSeq, &r.AwaitingDecision); err != nil {
			return nil, err
		}
		r.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		r.Event, r.At = Event(event), r.At.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// Record remembers that a message left.
//
// Conflict means a second sweep raced the first and both posted. The message
// is already out; refusing here would make the sweep retry it forever.
func (p *Postgres) Record(ctx context.Context, d Delivery) error {
	if d.Channel == "" || d.Conversation == "" {
		/*
			A delivery names a place, or it is not a delivery.

			Refused for either half. Both empty is the shape that means "said
			everywhere", and storing one retires the run from the sweep while
			every real conversation loses the message. Only the conversation
			empty is the half a direct message will produce — the connection is
			known long before the person's account is — and stored it claims
			somebody was told, suppressing the retry that would have told them.
		*/
		return fmt.Errorf("%w: %s", ErrUnaddressed, d.RunID)
	}
	_, err := p.pool.Exec(ctx, `
		insert into channel_deliveries
			(run_id, event, channel, conversation, at_seq, ref, placed_in, posted_at)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (run_id, event, channel, conversation, at_seq) do nothing`,
		string(d.RunID), string(d.Event), d.Channel, d.Conversation,
		d.AtSeq, d.Ref, d.Placed, d.PostedAt.UTC())
	if err != nil {
		return fmt.Errorf("channel: record delivery: %w", err)
	}
	return nil
}

// RecordFailure remembers a failed attempt to tell one conversation. Repeated
// sweeps update the same fact rather than creating a retry log.
func (p *Postgres) RecordFailure(ctx context.Context, f DeliveryFailure) error {
	return p.RecordFailures(ctx, []DeliveryFailure{f})
}

// RecordFailures remembers failed attempts in a single batch. During a channel
// incident the reporter can owe dozens of conversations per sweep; sending the
// writes together keeps the failure signal durable without turning the sweep
// into one database round trip per conversation.
func (p *Postgres) RecordFailures(ctx context.Context, failures []DeliveryFailure) error {
	if len(failures) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range failures {
		if f.SeenAt.IsZero() {
			f.SeenAt = time.Now()
		}
		batch.Queue(recordFailureSQL,
			string(f.RunID), string(f.Event), f.Channel, f.Conversation, f.AtSeq,
			f.ScopeWide, MetricCode(f.Code), string(f.Scope.Company),
			string(f.Scope.Area), string(f.AgentID), f.SeenAt.UTC())
	}
	results := p.pool.SendBatch(ctx, batch)
	for range failures {
		if _, err := results.Exec(); err != nil {
			_ = results.Close()
			return fmt.Errorf("channel: record delivery failure: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("channel: record delivery failure: %w", err)
	}
	return nil
}

const recordFailureSQL = `
	insert into channel_delivery_failures (
		run_id, event, channel, conversation, at_seq, scope_wide, code,
		company_id, area_id, agent_id, attempts, first_seen, last_seen)
	values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, $11, $11)
	on conflict (run_id, event, channel, conversation, at_seq, code)
	do update set
		attempts = channel_delivery_failures.attempts + 1,
		scope_wide = channel_delivery_failures.scope_wide or excluded.scope_wide,
		first_seen = least(channel_delivery_failures.first_seen, excluded.first_seen),
		last_seen = greatest(channel_delivery_failures.last_seen, excluded.last_seen)`

// Delivered answers whether this has already been said here.
//
// Here is a conversation *on a connection*: two workspaces are two namespaces,
// and an id that means one channel in Slack may mean another somewhere else.
func (p *Postgres) Delivered(ctx context.Context, a Announcement) (bool, error) {
	var exists bool
	err := p.pool.QueryRow(ctx, `
		select exists(select 1 from channel_deliveries
		              where run_id = $1 and event = $2
		                and channel = $3 and conversation = $4 and at_seq = $5)`,
		string(a.RunID), string(a.Event), a.Channel, a.Conversation, a.AtSeq).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("channel: read delivery: %w", err)
	}
	return exists, nil
}

// Reported marks one run's event as said everywhere it should be said.
//
// Written by the reporter, which is the only component that knows what
// everywhere means for a run's scope, and only when it reached all of them
// without a failure. An empty channel *and* an empty conversation is what "all
// of them" is filed under: a delivery belongs to a conversation on a
// connection, and this belongs to neither. Both empty, because a real delivery
// is never both — Record refuses to write one, so nothing else can reach this
// shape by accident.
//
// Filed against the step as well as the run. A run stops as many times as it
// asks, and a sentinel naming only the run answered the second question with
// the first one's silence.
func (p *Postgres) Reported(ctx context.Context, r Report, at time.Time) error {
	_, err := p.pool.Exec(ctx, `
		insert into channel_deliveries (run_id, event, channel, conversation, at_seq, ref, posted_at)
		values ($1, $2, '', '', $3, '', $4)
		on conflict (run_id, event, channel, conversation, at_seq) do nothing`,
		string(r.RunID), string(r.Event), r.AtSeq, at.UTC())
	if err != nil {
		return fmt.Errorf("channel: mark %s reported: %w", r.RunID, err)
	}
	return nil
}

// FinishedOutcome reads the payload that names a run's closing answer.
//
// The bytes of the answer are not here; run_finished carries a reference into
// the content store. This method reads only the ledger fact needed to resolve
// that reference when a channel is owed the final reply.
func (p *Postgres) FinishedOutcome(ctx context.Context, run domain.RunID) (domain.RunFinishedPayload, error) {
	var raw []byte
	err := p.pool.QueryRow(ctx, `
		select payload from run_steps
		where run_id = $1 and kind = 'run_finished'
		order by seq desc
		limit 1`, string(run)).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RunFinishedPayload{}, fmt.Errorf("channel: %s has no finished step", run)
	}
	if err != nil {
		return domain.RunFinishedPayload{}, fmt.Errorf("channel: read finished step for %s: %w", run, err)
	}

	var out domain.RunFinishedPayload
	if err := json.Unmarshal(raw, &out); err != nil {
		return domain.RunFinishedPayload{}, fmt.Errorf("channel: decode finished step for %s: %w", run, err)
	}
	return out, nil
}

/*
AboutRun answers which run the platform posted a message about, or nothing.

This is [NT-005 §2.1]'s boundary of resolution, and it turns out to already
exist in the table: **the platform resolves references to what it put there.**
A message somebody replies to in a thread is resolvable exactly when this
installation is the one that posted it, and `channel_deliveries` is the record
of every message it did.

Anything else — a third-party bot's alert, "that problem from yesterday" — does
not resolve, and must not pretend to. It becomes an ask with no subject,
tainted, and the Gate treats it as what it is: untrusted input asking for an
effect. An agent that needs a specific alert can go and search for one, which
is a tool call somebody can audit rather than a guess the edge made silently.

[NT-005 §2.1]: ../../docs/NT-005-interaction-channels.md
*/
func (p *Postgres) AboutRun(
	ctx context.Context, channel, conversation, ref string,
) (domain.RunID, bool, error) {
	if channel == "" || conversation == "" || ref == "" {
		return "", false, nil
	}

	var run string
	err := p.pool.QueryRow(ctx, `
		select run_id from channel_deliveries
		where channel = $1 and conversation = $2 and ref = $3
		order by posted_at desc
		limit 1`, channel, conversation, ref).Scan(&run)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("channel: resolve %s in %s/%s: %w", ref, channel, conversation, err)
	}
	return domain.RunID(run), true, nil
}
