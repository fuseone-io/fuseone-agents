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

// ErrNoPolicy means nobody wrote a rule under that code.
var ErrNoPolicy = errors.New("policy: no policy with that code")

// Store is the authored set and its history.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Set is the policies in force at a moment, and the hash that names them.
type Set struct {
	Hash     string
	Policies []domain.Policy
	TakenAt  time.Time
}

// Active reads what is in force now, with the hash that names it — and takes
// a snapshot if that hash has none.
//
// A read that writes, deliberately. Put always snapshots, but anything else
// that touches the table — a migration, a support script, somebody with psql —
// leaves a set whose hash names nothing. The Gate would then seal every
// decision to a name an auditor cannot fetch, which is the single promise this
// package exists to keep, and it would fail silently until somebody went
// looking years later.
func (s *Store) Active(ctx context.Context) (Set, error) {
	policies, err := readAll(ctx, s.pool)
	if err != nil {
		return Set{}, err
	}
	hash := HashOf(policies)

	var exists bool
	if err := s.pool.QueryRow(ctx,
		`select exists(select 1 from policy_snapshots where policy_hash = $1)`, hash,
	).Scan(&exists); err != nil {
		return Set{}, fmt.Errorf("policy: check snapshot %s: %w", hash, err)
	}
	if !exists {
		encoded, err := json.Marshal(policies)
		if err != nil {
			return Set{}, fmt.Errorf("policy: encode snapshot: %w", err)
		}
		if _, err := s.pool.Exec(ctx, `
			insert into policy_snapshots (policy_hash, policies) values ($1, $2)
			on conflict (policy_hash) do nothing`, hash, encoded); err != nil {
			return Set{}, fmt.Errorf("policy: take snapshot: %w", err)
		}
	}
	return Set{Hash: hash, Policies: policies}, nil
}

// Named reads one rule, for a client asking about that rule.
//
// Not Active with a filter. That reads the whole set and may write a snapshot,
// which is right for what names the set and wrong for a read that a refresh
// repeats once per policy.
func (s *Store) Named(ctx context.Context, code string) (domain.Policy, error) {
	p, err := scanPolicy(s.pool.QueryRow(ctx,
		`select `+policyColumns+` from policies where code = $1`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Policy{}, fmt.Errorf("%w: %s", ErrNoPolicy, code)
	}
	if err != nil {
		return domain.Policy{}, fmt.Errorf("policy: read %s: %w", code, err)
	}
	return p, nil
}

// Exec runs a statement against the policy tables. It exists for tests that
// have to simulate a write which bypassed Put, which is the case Active
// defends against.
func (s *Store) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := s.pool.Exec(ctx, sql, args...)
	return tag.RowsAffected(), err
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
