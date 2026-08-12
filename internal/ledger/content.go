package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// ErrNoContent means the reference points at nothing.
	ErrNoContent = errors.New("ledger: no content at that reference")
	// ErrErased is the domain's, so a caller matching it handles this store
	// and the in-memory one with the same line.
	ErrErased = domain.ErrContentErased
)

// Content is the durable claim check.
//
// A reference is run-scoped rather than a bare content hash: retention and
// per-subject erasure both work per run, and two runs holding identical bytes
// have to be purgeable independently.
type Content struct {
	pool *pgxpool.Pool
}

func NewContent(pool *pgxpool.Pool) *Content { return &Content{pool: pool} }

func (c *Content) Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error) {
	return c.PutFor(ctx, "run", string(runID), seq, data)
}

/*
PutFor stores content belonging to something other than a run.

Simulation cases are the other owner: real customer records, held outside the
ledger under the installation's retention like every other bulky payload
(AU-04). A table of their own would be a second place for personal data to
accumulate, with its own retention nobody remembers to set.

The owner is part of the reference, so identical bytes under two owners are two
rows. Retention and per-subject erasure work per owner (AU-11, NF-09), and a
purged run must not take a case set with it.
*/
func (c *Content) PutFor(
	ctx context.Context, kind, owner string, seq int64, data []byte,
) (string, error) {
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ref := fmt.Sprintf("%s://%s/%d/%s", kind, owner, seq, digest[:16])

	// Writing the same reference twice is writing the same bytes: the digest
	// is part of it. Doing nothing on conflict keeps a retry idempotent.
	if _, err := c.pool.Exec(ctx, `
		insert into run_content (ref, owner_kind, run_id, seq, digest, bytes)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (ref) do nothing`,
		ref, kind, owner, seq, digest, data); err != nil {
		return "", fmt.Errorf("ledger: store content for %s: %w", owner, err)
	}
	return ref, nil
}

func (c *Content) Get(ctx context.Context, ref string) ([]byte, error) {
	var data []byte
	var erased *time.Time
	err := c.pool.QueryRow(ctx,
		`select bytes, erased_at from run_content where ref = $1`, ref).Scan(&data, &erased)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNoContent, ref)
	}
	if erased != nil {
		return nil, fmt.Errorf("%w: %s", ErrErased, ref)
	}
	if err != nil {
		return nil, fmt.Errorf("ledger: read content %s: %w", ref, err)
	}
	return data, nil
}

/*
Erase removes everything one owner's content holds, leaving a tombstone.

This is per-subject erasure (NF-09), and it reaches the referenced content
rather than the ledger: the step keeps its reference and its digest, neither
changes, and the hash chain is untouched. That is the whole reason bulky
content was segregated in the first place (AU-04).

Erasing what is already erased is not an error. The caller is asking for a
state, not for an event, and a retry after a failure must not refuse.
*/
func (c *Content) Erase(ctx context.Context, owner string, reason string) (int, error) {
	tag, err := c.pool.Exec(ctx, `
		update run_content
		set bytes = null, erased_at = now(), erased_reason = $2
		where run_id = $1 and erased_at is null`, owner, reason)
	if err != nil {
		return 0, fmt.Errorf("ledger: erase content of %s: %w", owner, err)
	}
	return int(tag.RowsAffected()), nil
}

// ErasePast is retention: everything stored before a moment goes.
//
// The moment is computed by the caller from what the installation configured,
// and never defaulted here. A purge that ran on a default nobody chose would
// be the one operation in this system that destroys data without being asked.
func (c *Content) ErasePast(ctx context.Context, before time.Time, reason string) (int, error) {
	tag, err := c.pool.Exec(ctx, `
		update run_content
		set bytes = null, erased_at = now(), erased_reason = $2
		where created_at < $1 and erased_at is null`, before.UTC(), reason)
	if err != nil {
		return 0, fmt.Errorf("ledger: erase content before %s: %w", before, err)
	}
	return int(tag.RowsAffected()), nil
}
