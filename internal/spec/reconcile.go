package spec

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// ScheduleSync reconciles an agent's cron schedules in the caller's transaction.
type ScheduleSync interface {
	SyncIn(ctx context.Context, tx pgx.Tx, agent domain.AgentID, schedules []string, from time.Time) error
}

// WebhookSync reconciles an agent's webhooks in the caller's transaction.
type WebhookSync interface {
	SyncIn(ctx context.Context, tx pgx.Tx, agent domain.AgentID, scope domain.Scope, hooks []domain.WebhookTrigger) error
}

/*
WithReconciliation moves the agent's schedules and webhooks with every version this
publisher makes current — published or activated — in that transaction and under the
agent's row (#416). Reconciled after the commit instead, a slower publication could land
after a later one and put the earlier version's schedules back, or reopen a webhook the
live version withdrew.
*/
func (p *Publisher) WithReconciliation(schedules ScheduleSync, webhooks WebhookSync) *Publisher {
	p.schedules, p.webhooks = schedules, webhooks
	return p
}

// reconcile moves what s declares into the schedules and webhooks, in tx. An error undoes
// the version: a webhook path another agent answers on is refused, not left half live.
func (p *Publisher) reconcile(ctx context.Context, tx pgx.Tx, s Spec) error {
	if p.schedules != nil {
		if err := p.schedules.SyncIn(ctx, tx, s.ID, CronSchedules(s), p.clock.Now()); err != nil {
			return fmt.Errorf("reconcile the schedules of %s: %w", s.ID, err)
		}
	}
	if p.webhooks == nil {
		return nil
	}
	scope := domain.Scope{Company: s.Company, Area: s.Area}
	if err := p.webhooks.SyncIn(ctx, tx, s.ID, scope, Webhooks(s)); err != nil {
		return fmt.Errorf("reconcile the webhooks of %s: %w", s.ID, err)
	}
	return nil
}

// pausedIn is whether agent is paused, as tx leaves it.
func pausedIn(ctx context.Context, tx pgx.Tx, agent domain.AgentID) (bool, error) {
	var paused bool
	if err := tx.QueryRow(ctx, `select paused from agent_state where agent_id = $1`,
		string(agent)).Scan(&paused); err != nil {
		return false, fmt.Errorf("spec: read the pause of %s: %w", agent, err)
	}
	return paused, nil
}
