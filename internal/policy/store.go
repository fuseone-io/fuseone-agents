// Package policy stores authored rules and the snapshots that make a past
// decision reconstructable.
//
// The Gate evaluates against a snapshot rather than against the table, and the
// hash of that snapshot is what every step records. So "under the policy in
// force at the time" stops being a phrase in a document and becomes a row
// somebody can fetch.
package policy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNoSnapshot means nothing was ever taken under that hash.
var ErrNoSnapshot = errors.New("policy: no snapshot with that hash")

// Store is the authored set and its history.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Set is the policies in force at a moment, and the hash that names them.
type Set struct {
	Hash     string
	Policies []domain.Policy
	TakenAt  time.Time
}

// Put writes one policy and takes a new snapshot in the same transaction.
//
// Together, always. A policy that reached the table without a snapshot would
// be enforced under a hash describing a set it is not in, and the trail would
// point at the wrong rules for every decision until the next write.
func (s *Store) Put(ctx context.Context, p domain.Policy, by domain.UserID) (Set, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Set{}, fmt.Errorf("policy: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	scopes, conditions, effects, err := encode(p)
	if err != nil {
		return Set{}, err
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
		return Set{}, fmt.Errorf("policy: write %s: %w", p.Code, err)
	}

	return s.snapshot(ctx, tx)
}

// Delete removes a policy and snapshots what is left.
func (s *Store) Delete(ctx context.Context, code string) (Set, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Set{}, fmt.Errorf("policy: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `delete from policies where code = $1`, code); err != nil {
		return Set{}, fmt.Errorf("policy: delete %s: %w", code, err)
	}
	return s.snapshot(ctx, tx)
}

// Active reads what is in force now, with the hash that names it.
func (s *Store) Active(ctx context.Context) (Set, error) {
	policies, err := readAll(ctx, s.pool)
	if err != nil {
		return Set{}, err
	}
	return Set{Hash: HashOf(policies), Policies: policies}, nil
}

// Snapshot reads what was in force under a hash.
//
// The whole reason the hash exists. A decision recorded two years ago names
// one of these, and reconstructing that decision means reading the rules it
// was made under rather than today's.
func (s *Store) Snapshot(ctx context.Context, hash string) (Set, error) {
	var (
		raw     []byte
		takenAt time.Time
	)
	err := s.pool.QueryRow(ctx,
		`select policies, taken_at from policy_snapshots where policy_hash = $1`, hash,
	).Scan(&raw, &takenAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return Set{}, fmt.Errorf("%w: %s", ErrNoSnapshot, hash)
	}
	if err != nil {
		return Set{}, fmt.Errorf("policy: read snapshot %s: %w", hash, err)
	}

	var policies []domain.Policy
	if err := json.Unmarshal(raw, &policies); err != nil {
		return Set{}, fmt.Errorf("policy: decode snapshot %s: %w", hash, err)
	}
	return Set{Hash: hash, Policies: policies, TakenAt: takenAt.UTC()}, nil
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

// HashOf names a set of policies.
//
// Over the canonical encoding of the ordered set, so the same rules always
// produce the same name whatever order the database returned them in — a hash
// that moved with row order would make every restart look like a policy
// change.
func HashOf(policies []domain.Policy) string {
	encoded, err := json.Marshal(policies)
	if err != nil {
		// The type is fixed at compile time; a failure here is a struct that
		// cannot be marshalled, which is a bug rather than a runtime state.
		panic("policy: policies cannot be encoded: " + err.Error())
	}
	sum := sha256.Sum256(encoded)
	return "pol_" + hex.EncodeToString(sum[:16])
}
