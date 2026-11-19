package spec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// Activations is what the activation route reads and does (NT-018 phase 2, release 2c).
type Activations struct {
	publisher *Publisher
	reader    *CandidateReader
	schedules ScheduleSync
	webhooks  WebhookSync
}

// ScheduleSync reconciles an agent's cron schedules in the caller's transaction.
type ScheduleSync interface {
	SyncIn(ctx context.Context, tx pgx.Tx, agent domain.AgentID, schedules []string, from time.Time) error
}

// WebhookSync reconciles an agent's webhooks in the caller's transaction.
type WebhookSync interface {
	SyncIn(ctx context.Context, tx pgx.Tx, agent domain.AgentID, scope domain.Scope, hooks []domain.WebhookTrigger) error
}

/*
WithReconciliation moves the agent's schedules and webhooks with every activation, in its
transaction and under the agent's row. Reconciled after the commit instead, a slower
activation could land after a later one and put the earlier version's schedules back, or
reopen a webhook the live version withdrew.
*/
func (a *Activations) WithReconciliation(schedules ScheduleSync, webhooks WebhookSync) *Activations {
	a.schedules, a.webhooks = schedules, webhooks
	return a
}

// NewActivations activates through publisher, reading candidates with reader.
func NewActivations(publisher *Publisher, reader *CandidateReader) *Activations {
	return &Activations{publisher: publisher, reader: reader}
}

/*
Scopes is where the version lives, read before any lock (NT-018, activation step 2): the
scope of its published row and of each of its seals, without repeats. They name the same
bytes, and the bytes name their scope, so there is one; the route authorises every one it
is given all the same. None is a version the agent does not have.
*/
func (a *Activations) Scopes(ctx context.Context, agent domain.AgentID, version domain.VersionID) ([]domain.Scope, error) {
	rows, err := a.publisher.pool.Query(ctx, `
		select company_id, area_id from agent_specs where agent_id = $1 and version_id = $2
		union
		select company_id, area_id from agent_seals where agent_id = $1 and version_id = $2`,
		string(agent), string(version))
	if err != nil {
		return nil, fmt.Errorf("spec: scopes of %s@%s: %w", agent, version, err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Scope, error) {
		var company, area string
		err := row.Scan(&company, &area)
		return domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}, err
	})
}

/*
Definition is the version's definition as stored, read before any lock for the checks a
publication runs (activation step 4): the published row when there is one, else the
newest readable seal in scope. No readable seal is not an error here: the activation
decides that under its locks, and answers it in its own order.
*/
func (a *Activations) Definition(
	ctx context.Context, agent domain.AgentID, version domain.VersionID, scope domain.Scope,
) (Spec, bool, error) {
	row, err := a.publisher.registry.Get(ctx, agent, version)
	if err == nil {
		return row, true, nil
	}
	if !errors.Is(err, ErrNotPublished) {
		return Spec{}, false, err
	}
	candidate, err := a.reader.Read(ctx, agent, version, []domain.Scope{scope})
	if errors.Is(err, ErrCandidateNotFound) || errors.Is(err, domain.ErrCandidateUnreadable) {
		return Spec{}, false, nil
	}
	if err != nil {
		return Spec{}, false, err
	}
	sealed, err := Parse("sealed.agent.md", []byte(candidate.Definition))
	if err != nil {
		return Spec{}, false, nil
	}
	return sealed, true, nil
}

// Activate is Publisher.Activate, reconciling what moves with the version inside it.
func (a *Activations) Activate(ctx context.Context, act Activation) (Activated, error) {
	act.Within = a.reconcile
	return a.publisher.Activate(ctx, act)
}

func (a *Activations) reconcile(ctx context.Context, tx pgx.Tx, s Spec) error {
	if a.schedules != nil {
		if err := a.schedules.SyncIn(ctx, tx, s.ID, CronSchedules(s), a.publisher.clock.Now()); err != nil {
			return fmt.Errorf("reconcile the schedules of %s: %w", s.ID, err)
		}
	}
	if a.webhooks == nil {
		return nil
	}
	scope := domain.Scope{Company: s.Company, Area: s.Area}
	if err := a.webhooks.SyncIn(ctx, tx, s.ID, scope, Webhooks(s)); err != nil {
		return fmt.Errorf("reconcile the webhooks of %s: %w", s.ID, err)
	}
	return nil
}
