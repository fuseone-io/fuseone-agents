package spec

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// Activations is what the activation route reads and does (NT-018 phase 2, release 2c).
type Activations struct {
	publisher *Publisher
	reader    *CandidateReader
}

// WithReconciliation is Publisher.WithReconciliation on the publisher activations go
// through: the agent's schedules and webhooks move with every activation, in its
// transaction.
func (a *Activations) WithReconciliation(schedules ScheduleSync, webhooks WebhookSync) *Activations {
	a.publisher.WithReconciliation(schedules, webhooks)
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

// Activate is Publisher.Activate.
func (a *Activations) Activate(ctx context.Context, act Activation) (Activated, error) {
	return a.publisher.Activate(ctx, act)
}
