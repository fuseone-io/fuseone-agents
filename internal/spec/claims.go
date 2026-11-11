package spec

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
)

/*
Claiming a version's identity (NT-018, release C1).

A version id is the first 48 bits of the SHA-256 of a definition's bytes, and
agent_specs is insert-or-nothing on it. Two definitions with one id would be
answered with whichever was published first, so a publication could judge one
text and make the other current. A claim binds the id to the full digest the
first time it is published, and every later write is compared against it.

The full digest comes from Parse and only from Parse: a Spec rebuilt or
re-serialised is other bytes, and recomputing would claim bytes that never
existed. A Spec that does not carry its own digest does not publish.
*/

var (
	// ErrDigestMissing is a Spec that does not carry the digest of the bytes it
	// was parsed from, or carries one its version was not taken from.
	ErrDigestMissing = errors.New("spec: the definition does not carry the digest of its bytes")
	// ErrVersionCollision is another definition already holding this version id.
	ErrVersionCollision = errors.New("spec: another definition already holds this version id")
	// ErrDigestUnknown is a version id whose bytes were never kept: published
	// before claims, or by a binary that does not claim. Nothing can show that
	// new bytes are the same, so the id is not published again. Changing the
	// definition almost always gives another id; one that lands on another
	// claim is refused the same way.
	ErrDigestUnknown = errors.New("spec: this version id was published without a digest")
)

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// carriesDigest reports whether s holds a full digest its version was cut from.
func carriesDigest(s Spec) bool {
	return sha256Hex.MatchString(s.Digest) && string(s.Version) == "v"+s.Digest[:12]
}

// claimer is a transaction, as far as claiming needs one.
type claimer interface {
	execer
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

/*
claimVersion binds s's version id to its digest, or refuses.

It runs under the event graph lock, which every writer of agent_specs takes,
the previous binary's included. That is what makes the check for a row nobody
claimed race-free: no other writer can insert one between the check and the
claim.
*/
func claimVersion(ctx context.Context, q claimer, s Spec) error {
	if !carriesDigest(s) {
		return fmt.Errorf("%w: %s@%s", ErrDigestMissing, s.ID, s.Version)
	}
	claimed, found, err := claimOf(ctx, q, s)
	if err != nil {
		return err
	}
	if !found {
		if claimed, err = claimNew(ctx, q, s); err != nil {
			return err
		}
	}
	switch {
	case claimed == nil:
		return fmt.Errorf("%w: %s@%s", ErrDigestUnknown, s.ID, s.Version)
	case *claimed != s.Digest:
		return fmt.Errorf("%w: %s@%s", ErrVersionCollision, s.ID, s.Version)
	}
	return nil
}

func claimOf(ctx context.Context, q claimer, s Spec) (digest *string, found bool, err error) {
	err = q.QueryRow(ctx, `
		select digest from agent_version_digests where agent_id = $1 and version_id = $2`,
		string(s.ID), string(s.Version)).Scan(&digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("spec: read the claim of %s@%s: %w", s.ID, s.Version, err)
	}
	return digest, true, nil
}

// claimNew claims an unclaimed id, unless a row already holds it. That row was
// written without a digest, and claiming these bytes over it would record,
// for good, that it holds them.
func claimNew(ctx context.Context, q claimer, s Spec) (*string, error) {
	var orphan bool
	if err := q.QueryRow(ctx, `
		select exists (select 1 from agent_specs where agent_id = $1 and version_id = $2)`,
		string(s.ID), string(s.Version)).Scan(&orphan); err != nil {
		return nil, fmt.Errorf("spec: look for %s@%s: %w", s.ID, s.Version, err)
	}
	if orphan {
		return nil, nil
	}
	if _, err := q.Exec(ctx, `
		insert into agent_version_digests (agent_id, version_id, digest) values ($1, $2, $3)
		on conflict (agent_id, version_id) do nothing`,
		string(s.ID), string(s.Version), s.Digest); err != nil {
		return nil, fmt.Errorf("spec: claim %s@%s: %w", s.ID, s.Version, err)
	}
	digest, _, err := claimOf(ctx, q, s)
	return digest, err
}

// Proven reports whether s's bytes are the ones its version id is claimed for.
//
// What reconciles anything from a file, such as its schedules and webhook
// paths, asks this first. A file the registry refused is not what the
// installation runs, and its triggers must not be either.
func Proven(ctx context.Context, q claimer, s Spec) (bool, error) {
	if !carriesDigest(s) {
		return false, nil
	}
	claimed, found, err := claimOf(ctx, q, s)
	if err != nil || !found || claimed == nil {
		return false, err
	}
	return *claimed == s.Digest, nil
}
