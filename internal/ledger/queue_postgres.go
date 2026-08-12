package ledger

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// claimablePhases are the phases a worker may pick up unattended.
//
// awaiting_approval and finished are excluded for obvious reasons. parked is
// excluded for a less obvious one: parking means a human has to do something —
// raise a ceiling, fix an upstream — and a worker that resumed it anyway would
// turn the supervision policy into an infinite retry with extra steps.
var claimablePhaseNames = []string{"running", "awaiting_tool"}

// claimablePhases renders the same set as a SQL literal. Deriving it from the
// list is what keeps the in-memory queue and the SQL one from drifting apart
// while the contract suite reports both as passing.
var claimablePhases = "('" + strings.Join(claimablePhaseNames, "', '") + "')"

func claimable(phase string) bool {
	return slices.Contains(claimablePhaseNames, phase)
}

// Claim leases the run that has been waiting longest.
//
// SKIP LOCKED is what lets a pool share one queue without coordinating: a row
// another worker is already claiming is passed over rather than waited on.
func (p *Postgres) Claim(ctx context.Context, owner string, lease time.Duration) (domain.Claim, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Claim{}, fmt.Errorf("begin claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var c domain.Claim
	var runID, company, area, agentID, versionID, onBehalf string

	// An expired lease is claimable again: a worker that died stops renewing,
	// and no separate reaper is needed to notice.
	err = tx.QueryRow(ctx, `
		select run_id, company_id, area_id, agent_id, version_id, on_behalf_of, attempts
		from runs
		where phase in `+claimablePhases+`
		  -- A worker must never claim a simulated run. It would execute the
		  -- dry run's proposals with the real tool layer, against real
		  -- systems, on the strength of a case somebody uploaded to find out
		  -- what would happen.
		  and not simulated
		  and next_attempt_at <= now()
		  and (leased_until is null or leased_until <= now())
		order by next_attempt_at
		for update skip locked
		limit 1`,
	).Scan(&runID, &company, &area, &agentID, &versionID, &onBehalf, &c.Attempts)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Claim{}, domain.ErrNoClaimableRun
	}
	if err != nil {
		return domain.Claim{}, fmt.Errorf("select claimable: %w", err)
	}

	leasedUntil := time.Now().Add(lease)
	if _, err := tx.Exec(ctx, `
		update runs
		set leased_until = $2, lease_owner = $3
		where run_id = $1`, runID, leasedUntil, owner); err != nil {
		return domain.Claim{}, fmt.Errorf("write lease: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Claim{}, fmt.Errorf("commit claim: %w", err)
	}

	c.RunID = domain.RunID(runID)
	c.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	c.AgentID = domain.AgentID(agentID)
	c.VersionID = domain.VersionID(versionID)
	c.OnBehalfOf = domain.UserID(onBehalf)
	c.LeasedUntil = leasedUntil
	return c, nil
}

// Release ends a lease and records what happened.
//
// Progress resets the attempt count. The counter measures *consecutive*
// failures, so a run that fails twice and then advances starts clean; without
// the reset a long-running agent would eventually park itself over unrelated
// hiccups spread across hours.
func (p *Postgres) Release(ctx context.Context, runID domain.RunID, outcome domain.ClaimOutcome) error {
	nextAttempt := outcome.NextAttemptAt
	if nextAttempt.IsZero() {
		nextAttempt = time.Now()
	}

	_, err := p.pool.Exec(ctx, `
		update runs set
			leased_until    = null,
			lease_owner     = null,
			attempts        = case when $3 then attempts + 1 else 0 end,
			next_attempt_at = $2,
			last_error      = nullif($4, ''),
			phase           = case when $5 then 'parked' else phase end
		where run_id = $1`,
		string(runID), nextAttempt, outcome.Failed(), outcome.Reason(), outcome.Parked)
	if err != nil {
		return fmt.Errorf("release lease: %w", err)
	}
	return nil
}
