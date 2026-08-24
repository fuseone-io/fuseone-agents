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
		select runs.run_id, runs.agent_id, runs.company_id, runs.area_id,
		       `+phases+` as event, runs.updated_at,
		       coalesce(runs.pending_tool, ''), coalesce(runs.pending_reason, ''),
		       coalesce(runs.pending_at_seq, 0)
		from runs
		where not runs.simulated
		  and runs.updated_at >= $1
		  and `+phases+` is not null
		  and not exists (
		      select 1 from channel_deliveries d
		      where d.run_id = runs.run_id and d.event = `+phases+`
		        and d.channel = '' and d.conversation = '')
		order by runs.updated_at desc
		limit $2`, since.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("channel: unreported runs: %w", err)
	}
	defer rows.Close()

	var out []Report
	for rows.Next() {
		var r Report
		var company, area, event string
		if err := rows.Scan(&r.RunID, &r.AgentID, &company, &area,
			&event, &r.At, &r.Tool, &r.Reason, &r.AtSeq); err != nil {
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
	_, err := p.pool.Exec(ctx, `
		insert into channel_deliveries (run_id, event, channel, conversation, ref, posted_at)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (run_id, event, channel, conversation) do nothing`,
		string(d.RunID), string(d.Event), d.Channel, d.Conversation, d.Ref, d.PostedAt.UTC())
	if err != nil {
		return fmt.Errorf("channel: record delivery: %w", err)
	}
	return nil
}

// RecordFailure remembers a failed attempt to tell one conversation. Repeated
// sweeps update the same fact rather than creating a retry log.
func (p *Postgres) RecordFailure(ctx context.Context, f DeliveryFailure) error {
	if f.SeenAt.IsZero() {
		f.SeenAt = time.Now()
	}
	_, err := p.pool.Exec(ctx, `
		insert into channel_delivery_failures (
			run_id, event, channel, conversation, code,
			company_id, area_id, agent_id, attempts, first_seen, last_seen)
		values ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9, $9)
		on conflict (run_id, event, channel, conversation, code)
		do update set
			attempts = channel_delivery_failures.attempts + 1,
			first_seen = least(channel_delivery_failures.first_seen, excluded.first_seen),
			last_seen = greatest(channel_delivery_failures.last_seen, excluded.last_seen)`,
		string(f.RunID), string(f.Event), f.Channel, f.Conversation,
		MetricCode(f.Code), string(f.Scope.Company), string(f.Scope.Area),
		string(f.AgentID), f.SeenAt.UTC())
	if err != nil {
		return fmt.Errorf("channel: record delivery failure: %w", err)
	}
	return nil
}

// Delivered answers whether this has already been said here.
//
// Here is a conversation *on a connection*: two workspaces are two namespaces,
// and an id that means one channel in Slack may mean another somewhere else.
func (p *Postgres) Delivered(
	ctx context.Context, run domain.RunID, e Event, channel, conversation string,
) (bool, error) {
	var exists bool
	err := p.pool.QueryRow(ctx, `
		select exists(select 1 from channel_deliveries
		              where run_id = $1 and event = $2
		                and channel = $3 and conversation = $4)`,
		string(run), string(e), channel, conversation).Scan(&exists)
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
// is never both — which is what keeps the two apart now that a channel column
// exists and old rows carry an empty one.
func (p *Postgres) Reported(ctx context.Context, run domain.RunID, e Event, at time.Time) error {
	_, err := p.pool.Exec(ctx, `
		insert into channel_deliveries (run_id, event, channel, conversation, ref, posted_at)
		values ($1, $2, '', '', '', $3)
		on conflict (run_id, event, channel, conversation) do nothing`,
		string(run), string(e), at.UTC())
	if err != nil {
		return fmt.Errorf("channel: mark %s reported: %w", run, err)
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
