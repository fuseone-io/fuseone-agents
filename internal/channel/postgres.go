package channel

import (
	"context"
	"fmt"
	"time"

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

// Unreported lists runs in a state worth announcing that no conversation has
// been told about.
//
// Simulated runs are excluded. Nobody is waiting on one, and an approval
// request for a rehearsal would teach people to ignore the channel.
func (p *Postgres) Unreported(ctx context.Context, since time.Time, limit int) ([]Report, error) {
	rows, err := p.pool.Query(ctx, `
		select runs.run_id, runs.agent_id, runs.company_id, runs.area_id,
		       `+phases+` as event, runs.updated_at,
		       coalesce(runs.pending_tool, ''), coalesce(runs.pending_reason, '')
		from runs
		where not runs.simulated
		  and runs.updated_at >= $1
		  and `+phases+` is not null
		  and not exists (
		      select 1 from channel_deliveries d
		      where d.run_id = runs.run_id and d.event = `+phases+`)
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
			&event, &r.At, &r.Tool, &r.Reason); err != nil {
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
		insert into channel_deliveries (run_id, event, conversation, ref, posted_at)
		values ($1, $2, $3, $4, $5)
		on conflict (run_id, event, conversation) do nothing`,
		string(d.RunID), string(d.Event), d.Conversation, d.Ref, d.PostedAt.UTC())
	if err != nil {
		return fmt.Errorf("channel: record delivery: %w", err)
	}
	return nil
}

// Delivered answers whether this has already been said here.
func (p *Postgres) Delivered(
	ctx context.Context, run domain.RunID, e Event, conversation string,
) (bool, error) {
	var exists bool
	err := p.pool.QueryRow(ctx, `
		select exists(select 1 from channel_deliveries
		              where run_id = $1 and event = $2 and conversation = $3)`,
		string(run), string(e), conversation).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("channel: read delivery: %w", err)
	}
	return exists, nil
}
