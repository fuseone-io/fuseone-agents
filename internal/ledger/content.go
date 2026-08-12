package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// ErrNoContent means the reference points at nothing.
var ErrNoContent = errors.New("ledger: no content at that reference")

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
	err := c.pool.QueryRow(ctx, `select bytes from run_content where ref = $1`, ref).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNoContent, ref)
	}
	if err != nil {
		return nil, fmt.Errorf("ledger: read content %s: %w", ref, err)
	}
	return data, nil
}
