package channel

import (
	"context"
	"fmt"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Cards that still offer an answer to a question that has one.

A card is a message the platform posted with buttons on it. Once somebody
decides — in a conversation, in a private message, or in the console — every
other copy is still offering to answer, and pressing one of them is refused
with a conflict. That refusal is correct and the card is wrong: it says a run
is waiting when it is not.

Read as state rather than pushed from the decision, because most decisions
never pass through the channel at all. Somebody deciding in the console touches
no code here, so a hook on the inbound path would close the cards of Slack
decisions and leave every console decision's cards live — exactly the hole this
exists to fill.
*/

// Card is one posted message that still offers an answer, and what happened to
// the question it asked.
type Card struct {
	Announcement
	// PlacedIn is where the message actually is, which is where an edit goes.
	PlacedIn string
	Ref      string
	AgentID  domain.AgentID
	Scope    domain.Scope
	Tool     string
	Reason   string
	Outcome  Outcome
	// DecidedBy is who answered, when anybody did.
	DecidedBy string
}

// Outcome is what became of the question a card asked.
type Outcome string

const (
	OutcomeApproved Outcome = "approved"
	OutcomeRefused  Outcome = "refused"
	// OutcomeMovedOn is a card whose step is over with no decision recorded
	// against it: the run was abandoned, it failed, or it stopped again
	// somewhere else. Saying "refused" there would put a decision in somebody's
	// mouth that nobody made.
	OutcomeMovedOn Outcome = "moved_on"
)

// Cards is what was posted about a decision, and whether it still asks
// anything. Declared here by the consumer.
type Cards interface {
	Stale(ctx context.Context, limit int) ([]Card, error)
	Closed(ctx context.Context, c Card, at time.Time) error
}

/*
Stale lists cards whose question has been answered or has gone away.

A card is stale when its run is no longer waiting on the step the buttons name
— decided, abandoned, finished, or stopped again somewhere later. That last one
matters: a run parked twice is announced twice now, and the first card must
stop offering to answer a step the second one replaced.

The sentinel rows and the ones a driver could not place are excluded here
rather than skipped by the caller. A row with no reference names no message,
and one with no conversation is the shape that means a run was said everywhere.
*/
const openCardsSQL = `
		select d.run_id, d.event, d.at_seq, d.channel, d.conversation,
		       coalesce(nullif(d.placed_in, ''), d.conversation), d.ref,
		       r.agent_id, r.company_id, r.area_id,
		       coalesce(decided.approved, false), coalesce(decided.by, ''),
		       decided.approved is not null
		from channel_deliveries d
		join runs r on r.run_id = d.run_id
		left join lateral (
		    select (s.payload->>'approved')::boolean as approved,
		           s.payload->>'by' as by
		    from run_steps s
		    where s.run_id = d.run_id and s.opened_at = r.started_at
		      and s.kind = 'approval_decided' and s.seq > d.at_seq
		    order by s.seq asc
		    limit 1
		) decided on true
		where d.event = 'parked' and d.at_seq > 0 and d.closed_at is null
		  and d.ref <> '' and d.conversation <> ''
		  and (r.phase <> 'awaiting_approval'
		       or coalesce(r.pending_at_seq, 0) <> d.at_seq)
		order by r.updated_at desc
		limit $1`

func (p *Postgres) Stale(ctx context.Context, limit int) ([]Card, error) {
	rows, err := p.pool.Query(ctx, openCardsSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("channel: read open cards: %w", err)
	}
	defer rows.Close()

	var out []Card
	for rows.Next() {
		var c Card
		var event, company, area string
		var approved, wasDecided bool
		if err := rows.Scan(&c.RunID, &event, &c.AtSeq, &c.Channel, &c.Conversation,
			&c.PlacedIn, &c.Ref, &c.AgentID, &company, &area,
			&approved, &c.DecidedBy, &wasDecided); err != nil {
			return nil, err
		}
		c.Event = Event(event)
		c.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		c.Outcome = outcomeOf(approved, wasDecided)
		out = append(out, c)
	}
	return out, rows.Err()
}

func outcomeOf(approved, decided bool) Outcome {
	switch {
	case !decided:
		return OutcomeMovedOn
	case approved:
		return OutcomeApproved
	default:
		return OutcomeRefused
	}
}

// Closed records that a card no longer offers an answer.
//
// Written after the message was rewritten, and also after a refusal nothing can
// fix: a message somebody deleted can never be closed, and leaving it open
// means every sweep spends its budget rediscovering that.
func (p *Postgres) Closed(ctx context.Context, c Card, at time.Time) error {
	_, err := p.pool.Exec(ctx, `
		update channel_deliveries set closed_at = $6
		where run_id = $1 and event = $2 and channel = $3
		  and conversation = $4 and at_seq = $5`,
		string(c.RunID), string(c.Event), c.Channel, c.Conversation, c.AtSeq, at.UTC())
	if err != nil {
		return fmt.Errorf("channel: close card for %s: %w", c.RunID, err)
	}
	return nil
}
