package spec

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Taking an agent out of circulation.

There is no delete, anywhere, and there will not be one. A run is pinned to a
version and that version is the only correct explanation of what the run did;
removing it would leave the ledger pointing at a text nobody can read, which
is the opposite of what this platform exists to do.

Retiring is the honest alternative and it is a different act: the agent leaves
every listing, cannot be started, and keeps all of it — its versions, its
runs, and a line in the trail saying who retired it.
*/

// Retire takes an agent out of circulation and stops it in the same act.
//
// Both, and not just the flag: an agent the screen says is gone that a
// schedule still fires is worse than one nobody retired, because now nobody
// is looking for it.
func (s *State) Retire(ctx context.Context, agent domain.AgentID, by domain.UserID) error {
	_, err := s.pool.Exec(ctx, `
		insert into agent_state (agent_id, paused, retired_at, retired_by, changed_at, changed_by)
		values ($1, true, now(), $2, now(), $2)
		on conflict (agent_id) do update set
			paused = true,
			retired_at = now(), retired_by = excluded.retired_by,
			changed_at = now(), changed_by = excluded.changed_by`,
		string(agent), string(by))
	if err != nil {
		return fmt.Errorf("spec: retire %s: %w", agent, err)
	}
	return nil
}

// Restore brings an agent back, stopped.
//
// Stopped and never running: bringing an agent back and deciding it should
// act are two decisions, and doing both at once starts one nobody looked at.
func (s *State) Restore(ctx context.Context, agent domain.AgentID, by domain.UserID) error {
	_, err := s.pool.Exec(ctx, `
		update agent_state set
			retired_at = null, retired_by = '',
			paused = true, changed_at = now(), changed_by = $2
		where agent_id = $1`, string(agent), string(by))
	if err != nil {
		return fmt.Errorf("spec: restore %s: %w", agent, err)
	}
	return nil
}

// Retired reports which agents are out of circulation.
//
// A set, like the pauses and the stages beside it: every screen that lists
// agents needs this for all of them, and asking once is the difference
// between one query and one per row.
func (s *State) Retired(ctx context.Context) (map[domain.AgentID]bool, error) {
	rows, err := s.pool.Query(ctx,
		`select agent_id from agent_state where retired_at is not null`)
	if err != nil {
		return nil, fmt.Errorf("spec: read retired agents: %w", err)
	}
	defer rows.Close()

	out := map[domain.AgentID]bool{}
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return nil, fmt.Errorf("spec: scan retired agent: %w", err)
		}
		out[domain.AgentID(agent)] = true
	}
	return out, rows.Err()
}
