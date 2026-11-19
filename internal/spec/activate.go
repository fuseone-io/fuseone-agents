package spec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// ErrCandidateScopeMoved is an activation of a version sealed in a scope the agent no
// longer lives in. Nothing was written.
var ErrCandidateScopeMoved = errors.New("spec: the agent moved out of the version's scope")

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
	return sealedDefinition(ctx, tx, a, *claimed)
}

/*
sealedDefinition is the candidate as a readable seal holds it, read in the activation's
own transaction: a read through the pool, while this transaction holds the locks every
other activation waits for with a connection of its own, could wait for a connection
nobody returns.

The seals of the version in its scope are walked newest first, by key, one row a query,
and the query itself skips any whose bytes were erased or never kept. No window is fixed:
the oldest kept seal behind any number of erased ones is found, and no query reads more
than one seal's bytes while the locks are held. Kept bytes that are not the claimed ones
are a row written around every writer, refused as a collision; bytes that do not parse to
the version, its agent and scope are passed over.
*/
func sealedDefinition(ctx context.Context, tx pgx.Tx, a Activation, claimed string) (Activated, error) {
	var after *sealKey
	for {
		seal, found, err := nextKeptSeal(ctx, tx, a, after)
		if err != nil || !found {
			return Activated{}, firstOrUnreadable(err, a)
		}
		if digestOf(seal.bytes) != claimed {
			return Activated{}, fmt.Errorf("%w: seal %s of %s@%s holds other bytes",
				ErrVersionCollision, seal.id, a.Agent, a.Version)
		}
		sealed, err := parseSealed(seal.id, seal.bytes, SealExpectation{Agent: a.Agent, Version: a.Version, Scope: a.Scope})
		if err == nil {
			return Activated{Spec: sealed, SealID: seal.id, Created: true}, nil
		}
		after = &seal.sealKey
	}
}

// sealKey is where a seal stands in the walk: newest first, the id breaking ties.
type sealKey struct {
	at time.Time
	id string
}

type keptSeal struct {
	sealKey
	bytes []byte
}

// nextKeptSeal is the newest seal of the version in its scope after the key, whose bytes
// are still kept.
func nextKeptSeal(ctx context.Context, tx pgx.Tx, a Activation, after *sealKey) (keptSeal, bool, error) {
	var at *time.Time
	var id *string
	if after != nil {
		at, id = &after.at, &after.id
	}
	var out keptSeal
	err := tx.QueryRow(ctx, `
		select s.seal_id::text, s.sealed_at, c.bytes
		from agent_seals s
		join run_content c on c.ref = s.content_ref and c.erased_at is null
		where s.agent_id = $1 and s.version_id = $2 and s.company_id = $3 and s.area_id = $4
		  and ($5::timestamptz is null or (s.sealed_at, s.seal_id) < ($5, $6::uuid))
		order by s.sealed_at desc, s.seal_id desc
		limit 1`,
		string(a.Agent), string(a.Version), string(a.Scope.Company), string(a.Scope.Area), at, id).
		Scan(&out.id, &out.at, &out.bytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return keptSeal{}, false, nil
	}
	if err != nil {
		return keptSeal{}, false, fmt.Errorf("spec: seals of %s@%s: %w", a.Agent, a.Version, err)
	}
	return out, true, nil
}

func firstOrUnreadable(err error, a Activation) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("candidate %s/%s: %w", a.Agent, a.Version, domain.ErrCandidateUnreadable)
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
