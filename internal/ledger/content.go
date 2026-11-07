package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// ErrNoContent means the reference points at nothing.
	ErrNoContent = domain.ErrContentAbsent
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
	// limit is how much of one payload is kept. Zero is no limit.
	limit int
}

// NewContent bounds payloads by default.
//
// The limit is here rather than at each call site because a bound somebody has
// to remember to apply is a bound that is not there: six places build one of
// these, and the one that forgets is the one that writes a database dump into
// a row.
func NewContent(pool *pgxpool.Pool) *Content {
	return &Content{pool: pool, limit: domain.DefaultContentLimit}
}

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
	// The digest is always of the whole payload, even when only part of it is
	// kept. That is what keeps the record honest: an auditor holding the
	// original can still prove it is the one the run used.
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ref := fmt.Sprintf("%s://%s/%d/%s", kind, owner, seq, digest[:16])

	stored, truncated := domain.Truncate(data, c.limit)

	// Writing the same reference twice is writing the same bytes: the digest
	// is part of it. Doing nothing on conflict keeps a retry idempotent.
	if _, err := c.pool.Exec(ctx, `
		insert into run_content
			(ref, owner_kind, run_id, seq, digest, bytes, full_bytes, truncated)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (ref) do nothing`,
		ref, kind, owner, seq, digest, stored, len(data), truncated); err != nil {
		return "", fmt.Errorf("ledger: store content for %s: %w", owner, err)
	}
	return ref, nil
}

/*
RestoreFor stores content again over its own tombstone.

PutFor leaves an erased payload erased, because a retried step writes the same
reference and must not undo an erasure that happened in between. A person
recording a correction again is a new act: the reference is the digest, so what
they read from a live run lands on the tombstone of the copy that aged out. It
gets a new retention window, because it was stored again today.
*/
func (c *Content) RestoreFor(
	ctx context.Context, kind, owner string, seq int64, data []byte,
) (string, error) {
	return c.restore(ctx, c.pool, contentOwner{kind, owner, seq}, data)
}

// RestoreForTx is RestoreFor inside a transaction, for content that must not exist
// unless what names it does.
func (c *Content) RestoreForTx(
	ctx context.Context, tx pgx.Tx, kind, owner string, seq int64, data []byte,
) (string, error) {
	return c.restore(ctx, tx, contentOwner{kind, owner, seq}, data)
}

type contentOwner struct {
	kind, owner string
	seq         int64
}

func (c *Content) restore(
	ctx context.Context, conn contentExecutor, to contentOwner, data []byte,
) (string, error) {
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ref := fmt.Sprintf("%s://%s/%d/%s", to.kind, to.owner, to.seq, digest[:16])
	stored, truncated := domain.Truncate(data, c.limit)

	if _, err := conn.Exec(ctx, `
		insert into run_content
			(ref, owner_kind, run_id, seq, digest, bytes, full_bytes, truncated)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (ref) do update
		set bytes = excluded.bytes, truncated = excluded.truncated,
		    erased_at = null, erased_reason = '', created_at = now()
		where run_content.erased_at is not null`,
		ref, to.kind, to.owner, to.seq, digest, stored, len(data), truncated); err != nil {
		return "", fmt.Errorf("ledger: restore content for %s: %w", to.owner, err)
	}
	return ref, nil
}

/*
WithLimit raises or lowers what one payload may occupy.

The day object storage arrives, this is where the number goes up. Zero is
unbounded and is an explicit choice: the default is already a limit, so asking
for none is something a caller says rather than something it forgets.
*/
func (c *Content) WithLimit(bytes int) *Content {
	out := *c
	out.limit = bytes
	return &out
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
Metadata answers what a reference says about itself without reading the bytes.

The reference carries only the first 16 hex of the digest; this is the whole
SHA-256 the store recorded over the whole payload, which is the number a
citation has to be checked against. Re-hashing what Get returns would disagree
with it for anything the limit truncated.

Erased is reported rather than raised: the digest outlives the bytes, so a
caller can still tell a citation whose content was erased from one that was
never true.
*/
func (c *Content) Metadata(ctx context.Context, ref string) (domain.ContentMetadata, error) {
	var out domain.ContentMetadata
	var erased *time.Time
	err := c.pool.QueryRow(ctx,
		`select digest, erased_at from run_content where ref = $1`, ref).Scan(&out.Digest, &erased)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ContentMetadata{}, fmt.Errorf("%w: %s", ErrNoContent, ref)
	}
	if err != nil {
		return domain.ContentMetadata{}, fmt.Errorf("ledger: read content metadata %s: %w", ref, err)
	}
	out.Erased = erased != nil
	return out, nil
}

/*
EraseOwnersTx removes everything these owners' content holds, leaving tombstones.

This is per-subject erasure (NF-09), and it reaches the referenced content
rather than the ledger: the step keeps its reference and its digest, neither
changes, and the hash chain is untouched. That is the whole reason bulky
content was segregated in the first place (AU-04).

One statement however many owners: a request about one person can name thousands of
runs, and everything else that wants their corpus is waiting while this runs.

Erasing what is already erased is not an error. The caller is asking for a
state, not for an event, and a retry after a failure must not refuse.
*/
func (c *Content) EraseOwnersTx(
	ctx context.Context, tx pgx.Tx, kind string, owners []string, reason string,
) (int, error) {
	tag, err := tx.Exec(ctx, `
		update run_content
		set bytes = null, erased_at = now(), erased_reason = $3
		where owner_kind = $1 and run_id = any($2::text[]) and erased_at is null`,
		kind, owners, reason)
	if err != nil {
		return 0, fmt.Errorf("ledger: erase content of %d owners: %w", len(owners), err)
	}
	return int(tag.RowsAffected()), nil
}

/*
EraseRefsTx removes named payloads, leaving the same tombstone.

Per owner is too wide for a corpus: its copies are filed under the agent, and a
request about one person must not take every correction that agent has.
*/
func (c *Content) EraseRefsTx(
	ctx context.Context, tx pgx.Tx, refs []string, reason string,
) (int, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `
		update run_content
		set bytes = null, erased_at = now(), erased_reason = $2
		where ref = any($1::text[]) and erased_at is null`, refs, reason)
	if err != nil {
		return 0, fmt.Errorf("ledger: erase content by reference: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ErasePastTx is retention: everything stored before a moment goes.
//
// The moment is computed by the caller from what the installation configured,
// and never defaulted here. A purge that ran on a default nobody chose would
// be the one operation in this system that destroys data without being asked.
func (c *Content) ErasePastTx(
	ctx context.Context, tx pgx.Tx, before time.Time, reason string,
) (int, error) {
	return eraseBefore(ctx, tx, before, reason)
}

func eraseBefore(
	ctx context.Context, conn contentExecutor, before time.Time, reason string,
) (int, error) {
	tag, err := conn.Exec(ctx, `
		update run_content
		set bytes = null, erased_at = now(), erased_reason = $2
		where created_at < $1 and erased_at is null`, before.UTC(), reason)
	if err != nil {
		return 0, fmt.Errorf("ledger: erase content before %s: %w", before, err)
	}
	return int(tag.RowsAffected()), nil
}

type contentExecutor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
