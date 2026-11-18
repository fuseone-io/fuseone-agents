package admin

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// agentsLosingSeals is whose candidates retention is about to reach: the agents of every
// seal whose bytes age out of the window (NT-018 phase 2).
func (e *Erasures) agentsLosingSeals(ctx context.Context, before time.Time) ([]domain.AgentID, error) {
	agents, err := scanStrings(ctx, e.pool, `
		select distinct s.agent_id
		from agent_seals s
		join run_content sealed on sealed.ref = s.content_ref
		where sealed.created_at < $1 and sealed.erased_at is null`, before.UTC())
	if err != nil {
		return nil, fmt.Errorf("admin: find the seals retention reaches: %w", err)
	}
	return toAgents(agents), nil
}

/*
eraseRunsOfExpiredSeals takes the content of every run derived from a seal that ages out,
however recent the run: its output may repeat the sealed text, and that text is what the
window promised to remove. A seal already erased some other way counts too, so a run that
outlived its seal's erasure is caught by the next sweep.
*/
func (e *Erasures) eraseRunsOfExpiredSeals(ctx context.Context, tx pgx.Tx, before time.Time) (int, error) {
	runs, err := scanStrings(ctx, tx, `
		select r.run_id
		from runs r
		join agent_seals s on s.seal_id = r.seal_id
		join run_content sealed on sealed.ref = s.content_ref
		where (sealed.created_at < $1 or sealed.erased_at is not null)
		  and exists (
			select 1 from run_content held
			where held.owner_kind = 'run' and held.run_id = r.run_id and held.erased_at is null
		  )
		order by r.run_id`, before.UTC())
	if err != nil {
		return 0, fmt.Errorf("admin: find runs of expired seals: %w", err)
	}
	if len(runs) == 0 {
		return 0, nil
	}
	n, err := e.content.EraseOwnersTx(ctx, tx, "run", runs, "retention")
	if err != nil {
		return 0, fmt.Errorf("admin: erase runs of expired seals: %w", err)
	}
	return n, nil
}

// fencedAgents is every agent a sweep reaches, once each.
func fencedAgents(groups ...[]domain.AgentID) []domain.AgentID {
	var out []domain.AgentID
	for _, group := range groups {
		out = append(out, group...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}
