package spec

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// ErrAgentNotFound is an agent that does not exist, or one the caller may not
// write where it lives: the two answer the same, so a refusal never says the
// agent is there.
var ErrAgentNotFound = errors.New("spec: no such agent")

/*
Guard is what a write to an agent must find under the agent's row lock
(NT-017 phase 1, NT-018 1a).

Judged in the transaction that writes, after the event graph's lock and then
the agent's row, in publication's order. First the scope the agent lives in
now, then the precondition: an agent the caller may not reach is one that
does not exist, so a tag never tells it that an agent it cannot see changed.
*/
type Guard struct {
	// Allow answers whether the caller may write an agent living in the scope
	// it is handed: the agent's current one, read under the lock. Nil judges
	// no scope, for a caller with nobody behind it; the HTTP API always sets
	// it.
	Allow func(current domain.Scope) bool
	// Expect is the caller's If-Match. The zero value asks nothing.
	Expect domain.IfMatch
	// Absent is If-None-Match: *, for a publication creating the agent.
	Absent bool
}

// lockedAgent is what the guard judges: whether the agent has a version, the
// scope its current one lives in, and its tag.
type lockedAgent struct {
	exists   bool
	scope    domain.Scope
	revision domain.Revision
}

// judge applies the guard to the agent as the lock found it.
func (g Guard) judge(a lockedAgent) error {
	if a.exists && g.Allow != nil && !g.Allow(a.scope) {
		return ErrAgentNotFound
	}
	if g.Absent && a.exists {
		return &domain.StaleError{Current: a.revision}
	}
	if !g.Expect.Allows(a.exists, a.revision) {
		return &domain.StaleError{Current: a.revision}
	}
	return nil
}

/*
lockAgent locks the agent's state row, then reads what it is now.

Lock first, judge second. The row is created if the agent has none, as
refuseStage does, so the lock always lands on one: a read of an absent row
locks nothing. Every write that changes the current version or the
retirement writes this row, so once the lock is granted, the reads after it
see what they committed, under read committed. The current version is chosen
as every reader chooses it (Registry.Versions): the one somebody made
current, else the newest published.
*/
func lockAgent(ctx context.Context, tx pgx.Tx, agent domain.AgentID) (lockedAgent, error) {
	if _, err := tx.Exec(ctx, `
		insert into agent_state (agent_id, paused, changed_at) values ($1, true, now())
		on conflict (agent_id) do nothing`, string(agent)); err != nil {
		return lockedAgent{}, fmt.Errorf("spec: %s: ensure state: %w", agent, err)
	}
	var retired bool
	if err := tx.QueryRow(ctx, `
		select retired_at is not null from agent_state where agent_id = $1 for update`,
		string(agent)).Scan(&retired); err != nil {
		return lockedAgent{}, fmt.Errorf("spec: %s: lock state: %w", agent, err)
	}
	var version, company, area string
	err := tx.QueryRow(ctx, `
		select s.version_id, s.company_id, s.area_id
		from agent_specs s
		left join agent_state st on st.agent_id = s.agent_id
		where s.agent_id = $1
		order by (st.current_version = s.version_id) desc nulls last, s.published_at desc
		limit 1`, string(agent)).Scan(&version, &company, &area)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedAgent{}, nil
	}
	if err != nil {
		return lockedAgent{}, fmt.Errorf("spec: %s: read the current version: %w", agent, err)
	}
	return lockedAgent{
		exists:   true,
		scope:    domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)},
		revision: domain.AgentRevision(domain.VersionID(version), retired),
	}, nil
}

// guardAgent locks the agent and judges the guard, returning what it found.
func guardAgent(ctx context.Context, tx pgx.Tx, agent domain.AgentID, g Guard) (lockedAgent, error) {
	locked, err := lockAgent(ctx, tx, agent)
	if err != nil {
		return lockedAgent{}, err
	}
	if err := g.judge(locked); err != nil {
		return lockedAgent{}, fmt.Errorf("spec: %s: %w", agent, err)
	}
	return locked, nil
}
