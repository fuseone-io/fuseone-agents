package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

// revisionKind keeps a rule's revision from ever naming another kind of resource.
const revisionKind = "policy"

// RevisionOf names a rule by what it says. Absent and empty lists are the same rule, and
// the order of a list is kept: the stored form keeps it, so a client's reorder is a change
// it can read back.
func RevisionOf(p domain.Policy) domain.Revision {
	encoded, err := json.Marshal(canonical(p))
	if err != nil {
		// As in HashOf: the type is fixed at compile time.
		panic("policy: a rule cannot be encoded: " + err.Error())
	}
	return domain.NewRevision(revisionKind, encoded)
}

func canonical(p domain.Policy) domain.Policy {
	if p.Effects == nil {
		p.Effects = []domain.Effect{}
	}
	if p.Scopes == nil {
		p.Scopes = []domain.Scope{}
	}
	if p.Agents == nil {
		p.Agents = []domain.AgentID{}
	}
	if p.Conditions == nil {
		p.Conditions = []domain.Condition{}
	}
	return p
}

// Written is what a write left behind. Changed is false when the rule already said this,
// in which case nothing was written and the set is the one already in force.
type Written struct {
	Set
	Revision domain.Revision
	Changed  bool
}

/*
beginGuarded opens a write that holds the policy set against every other write.

A lock not on a row, because the rule may not exist yet and there would be no row to hold.
On the whole set rather than one code, because each write also snapshots the whole set,
and one that could not see a neighbour's commit would name a set that never existed.
Writes to rules are rare and a person is usually behind them, so this costs nothing anyone
would notice. A statement of its own, before the comparison: READ COMMITTED gives each
statement a fresh view, so what is compared is what the previous holder committed.
*/
func (s *Store) beginGuarded(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("policy: begin: %w", err)
	}
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended('fuseone:policies', 0))`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("policy: lock the set: %w", err)
	}
	return tx, nil
}

// judged reads the rule as it is now and applies the precondition, under the guard.
func judged(ctx context.Context, tx pgx.Tx, code string, expect domain.IfMatch) (domain.Revision, error) {
	stored, err := scanPolicy(tx.QueryRow(ctx, `select `+policyColumns+` from policies where code = $1`, code))
	exists := true
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		exists = false
	case err != nil:
		return "", fmt.Errorf("policy: read %s: %w", code, err)
	}
	var current domain.Revision
	if exists {
		current = RevisionOf(stored)
	}
	if !expect.Allows(exists, current) {
		return "", &domain.StaleError{Current: current}
	}
	return current, nil
}

// Put writes one policy and takes a new snapshot in the same transaction, unless the rule
// already says exactly this. Then nothing moves: no snapshot, no updated_at, no author.
//
// Together, always. A policy that reached the table without a snapshot would be enforced
// under a hash describing a set it is not in, and the trail would point at the wrong rules
// for every decision until the next write.
func (s *Store) Put(ctx context.Context, p domain.Policy, by domain.UserID, expect domain.IfMatch) (Written, error) {
	scopes, conditions, effects, err := encode(p)
	if err != nil {
		return Written{}, err
	}
	tx, err := s.beginGuarded(ctx)
	if err != nil {
		return Written{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := judged(ctx, tx, p.Code, expect)
	if err != nil {
		return Written{}, err
	}
	revision := RevisionOf(p)
	if current == revision {
		policies, err := readAll(ctx, tx)
		if err != nil {
			return Written{}, err
		}
		return Written{Set: Set{Hash: HashOf(policies), Policies: policies}, Revision: revision}, nil
	}

	// The company and area columns are left at their defaults: in phase 1 every
	// policy is filed at the installation and reach is what narrows it. They
	// exist so multi-company does not need a migration on this table.
	if _, err := tx.Exec(ctx, `
		insert into policies (code, name, owner, reason,
		                      resource, effects, reach, scopes, agents,
		                      conditions, effect, mode, enabled, updated_by)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		on conflict (code) do update set
			name = excluded.name, owner = excluded.owner, reason = excluded.reason,
			resource = excluded.resource, effects = excluded.effects,
			reach = excluded.reach, scopes = excluded.scopes, agents = excluded.agents,
			conditions = excluded.conditions, effect = excluded.effect,
			mode = excluded.mode, enabled = excluded.enabled,
			updated_at = now(), updated_by = excluded.updated_by`,
		p.Code, p.Name, p.Owner, p.Reason,
		p.Resource, effects, string(p.Reach), scopes, agentIDs(p.Agents),
		conditions, string(p.Effect), string(p.Mode), p.Enabled, string(by),
	); err != nil {
		return Written{}, fmt.Errorf("policy: write %s: %w", p.Code, err)
	}

	set, err := s.snapshot(ctx, tx)
	if err != nil {
		return Written{}, err
	}
	return Written{Set: set, Revision: revision, Changed: true}, nil
}

// Delete removes a policy and snapshots what is left.
func (s *Store) Delete(ctx context.Context, code string, expect domain.IfMatch) (Set, error) {
	tx, err := s.beginGuarded(ctx)
	if err != nil {
		return Set{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := judged(ctx, tx, code, expect); err != nil {
		return Set{}, err
	}
	if _, err := tx.Exec(ctx, `delete from policies where code = $1`, code); err != nil {
		return Set{}, fmt.Errorf("policy: delete %s: %w", code, err)
	}
	return s.snapshot(ctx, tx)
}

// snapshot records the current set inside the caller's transaction.
func (s *Store) snapshot(ctx context.Context, tx pgx.Tx) (Set, error) {
	policies, err := readAll(ctx, tx)
	if err != nil {
		return Set{}, err
	}

	hash := HashOf(policies)
	encoded, err := json.Marshal(policies)
	if err != nil {
		return Set{}, fmt.Errorf("policy: encode snapshot: %w", err)
	}

	// The same set twice is the same snapshot. Editing a policy back to what
	// it was returns to the hash it had, which is correct: the rules in force
	// are identical and a decision under either is the same decision.
	if _, err := tx.Exec(ctx, `
		insert into policy_snapshots (policy_hash, policies)
		values ($1, $2)
		on conflict (policy_hash) do nothing`, hash, encoded); err != nil {
		return Set{}, fmt.Errorf("policy: take snapshot: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Set{}, fmt.Errorf("policy: commit: %w", err)
	}
	return Set{Hash: hash, Policies: policies}, nil
}
