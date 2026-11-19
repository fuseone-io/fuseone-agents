package spec

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// ErrCandidateScopeMoved is an activation of a version sealed in a scope the agent no
// longer lives in. Nothing was written.
var ErrCandidateScopeMoved = errors.New("spec: the agent moved out of the version's scope")

// CandidateSource reads a candidate's sealed text: CandidateReader in production.
type CandidateSource interface {
	Read(ctx context.Context, agent domain.AgentID, version domain.VersionID, visible []domain.Scope) (Candidate, error)
}

// Activation is one version to make current (NT-018 phase 2, release 2c).
type Activation struct {
	Agent   domain.AgentID
	Version domain.VersionID
	// Scope is the version's own scope, which the caller was authorised on before any
	// lock: the published row's, or its seals'. Both name the same bytes, and the bytes
	// name their scope.
	Scope domain.Scope
	By    domain.UserID
	// Guard is judged under the agent's row lock, as a publication's is.
	Guard Guard
	// Seals is where a candidate's bytes are read from.
	Seals CandidateSource
	// Accept judges the definition after the graph, before anything is written: the
	// workbench's never block.
	Accept func(Spec) error
}

// Activated is an activation that committed.
type Activated struct {
	Spec Spec
	// SealID is the seal the candidate's bytes were read from; empty on the published
	// path, which reads none.
	SealID string
	// Created is whether the version's row was written by this activation.
	Created bool
	Judged  Published
}

/*
Activate makes a.Version the agent's current version, in one transaction, in the order
NT-018 gives (steps 4 to 14; the caller has done 1 to 3 and holds the content fence).

Under the event graph's lock and the agent's row, as a publication takes them: the guard
(404 before 412), the scope the version was sealed in against the one the agent lives in
now, the autonomous stage. Then the definition, chosen by the claim and nothing read
before the locks: the published row when there is one, so an erased seal never blocks a
version already published; else a readable seal whose bytes hash to the claim. That
definition alone is judged, accepted and written, with the activation that wrote it.
*/
func (p *Publisher) Activate(ctx context.Context, a Activation) (Activated, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Activated{}, fmt.Errorf("spec: activate %s@%s: begin: %w", a.Agent, a.Version, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockEventGraph(ctx, tx); err != nil {
		return Activated{}, fmt.Errorf("spec: activate %s@%s: %w", a.Agent, a.Version, err)
	}
	if err := guardActivation(ctx, tx, a); err != nil {
		return Activated{}, err
	}
	out, err := p.definitionOf(ctx, tx, a)
	if err != nil {
		return Activated{}, err
	}
	if out.Judged, err = p.judge(ctx, tx, out.Spec); err != nil {
		return Activated{}, err
	}
	if err := a.Accept(out.Spec); err != nil {
		return Activated{}, err
	}
	if err := writeActivation(ctx, tx, a, out); err != nil {
		return Activated{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Activated{}, fmt.Errorf("spec: activate %s@%s: commit: %w", a.Agent, a.Version, err)
	}
	return out, nil
}

// guardActivation is everything judged about the agent under its row lock: who may
// write and what they saw, where it lives, and its stage.
func guardActivation(ctx context.Context, tx pgx.Tx, a Activation) error {
	found, err := guardAgent(ctx, tx, a.Agent, a.Guard)
	if err != nil {
		return err
	}
	if found.Exists && found.Scope != a.Scope {
		return fmt.Errorf("%w: %s lives in %s/%s, the version in %s/%s", ErrCandidateScopeMoved,
			a.Agent, found.Scope.Company, found.Scope.Area, a.Scope.Company, a.Scope.Area)
	}
	return refuseStage(ctx, tx, a.Agent, []domain.Stage{domain.StageAutonomous})
}

// definitionOf decides which row defines the version, by its claim, and loads it.
func (p *Publisher) definitionOf(ctx context.Context, tx pgx.Tx, a Activation) (Activated, error) {
	claimed, found, err := claimOf(ctx, tx, Spec{ID: a.Agent, Version: a.Version})
	switch {
	case err != nil:
		return Activated{}, err
	case !found:
		return Activated{}, fmt.Errorf("%w: %s@%s", ErrCandidateNotFound, a.Agent, a.Version)
	case claimed == nil:
		return Activated{}, fmt.Errorf("%w: %s@%s", ErrDigestUnknown, a.Agent, a.Version)
	}
	published, err := PublishedIn(ctx, tx, a.Agent, a.Version)
	if err != nil {
		return Activated{}, err
	}
	if published {
		row, err := p.registry.within(tx).Get(ctx, a.Agent, a.Version)
		return Activated{Spec: row}, err
	}
	return sealedDefinition(ctx, a, *claimed)
}

// sealedDefinition is the candidate as a readable seal holds it, refused unless its
// bytes are exactly the ones the claim names.
func sealedDefinition(ctx context.Context, a Activation, claimed string) (Activated, error) {
	candidate, err := a.Seals.Read(ctx, a.Agent, a.Version, []domain.Scope{a.Scope})
	if err != nil {
		return Activated{}, err
	}
	data := []byte(candidate.Definition)
	if digestOf(data) != claimed {
		return Activated{}, fmt.Errorf("%w: seal %s of %s@%s holds other bytes",
			ErrVersionCollision, candidate.SealID, a.Agent, a.Version)
	}
	sealed, err := Parse("sealed.agent.md", data)
	if err != nil {
		return Activated{}, unreadable(candidate.SealID, "its bytes do not parse")
	}
	return Activated{Spec: sealed, SealID: candidate.SealID, Created: true}, nil
}

// writeActivation writes a candidate's row, makes the version current and records the
// activation, all in tx.
func writeActivation(ctx context.Context, tx pgx.Tx, a Activation, out Activated) error {
	path := "published"
	if out.Created {
		path = "candidate"
		if err := publishOn(ctx, tx, out.Spec, a.By, out.Spec.Company); err != nil {
			return err
		}
	}
	if err := makeCurrentOn(ctx, tx, a.Agent, a.Version); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		insert into agent_activations (agent_id, version_id, seal_id, path, activated_by)
		values ($1, $2, nullif($3, '')::uuid, $4, $5)`,
		string(a.Agent), string(a.Version), out.SealID, path, string(a.By)); err != nil {
		return fmt.Errorf("spec: record the activation of %s@%s: %w", a.Agent, a.Version, err)
	}
	return nil
}
