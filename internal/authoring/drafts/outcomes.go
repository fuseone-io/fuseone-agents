package drafts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
ProposalOutcomes is what became of the proposals of the given turns of the caller's own
thread, in the thread's order: each with its turn, kind, validity, decision, and whether
the draft as it is would still accept it (#412). One query of metadata, bounded by the
turns asked, which a prompt takes from its window; never a body or a reason, and never the
rest of the thread. Whether a waiting proposal is current is judged once per turn, since
every proposal of a turn read the same ETag.
*/
func (s *Store) ProposalOutcomes(ctx context.Context, agent domain.AgentID, c Caller, turns []string) ([]Proposal, error) {
	var out []Proposal
	if len(turns) == 0 {
		return out, nil
	}
	err := s.within(ctx, func(tx pgx.Tx) error {
		r, err := s.reachableRow(ctx, tx, agent, c, false)
		if err != nil {
			return err
		}
		if out, err = outcomesOf(ctx, tx, r.draftID, c.By, turns); err != nil {
			return err
		}
		return judgeCurrent(ctx, tx, r, out)
	})
	return out, err
}

func outcomesOf(ctx context.Context, tx pgx.Tx, draftID string, author domain.UserID, turns []string) ([]Proposal, error) {
	rows, err := tx.Query(ctx, `
		select p.proposal_id::text, p.turn_id::text, p.draft_id::text, p.kind, p.valid,
		       p.draft_revision, coalesce(d.decision, '')
		from agent_draft_proposals p
		left join agent_draft_proposal_decisions d on d.proposal_id = p.proposal_id
		where p.draft_id = $1::uuid and p.author = $2 and p.turn_id = any($3::uuid[])
		order by p.seq`, draftID, string(author), turns)
	if err != nil {
		return nil, fmt.Errorf("drafts: read the outcomes: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Proposal, error) {
		var p Proposal
		var read string
		err := row.Scan(&p.ID, &p.Turn, &p.Draft, &p.Kind, &p.Valid, &read, &p.Decision)
		p.Read = domain.Revision(read)
		return p, err
	})
}

// judgeCurrent sets Current on each waiting valid proposal, judging each turn once.
func judgeCurrent(ctx context.Context, tx pgx.Tx, r row, out []Proposal) error {
	judged := map[string]bool{}
	for i, p := range out {
		if !p.Valid || p.Decision != "" {
			continue
		}
		current, seen := judged[p.Turn]
		if !seen {
			var err error
			if current, err = waitsOnThisDraft(ctx, tx, stored{Proposal: p}, r); err != nil {
				return err
			}
			judged[p.Turn] = current
		}
		out[i].Current = current
	}
	return nil
}
