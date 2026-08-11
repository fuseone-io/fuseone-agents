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
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ref := fmt.Sprintf("run://%s/%d/%s", runID, seq, digest[:16])

	// Writing the same reference twice is writing the same bytes: the digest
	// is part of it. Doing nothing on conflict keeps a retry idempotent.
	if _, err := c.pool.Exec(ctx, `
		insert into run_content (ref, run_id, seq, digest, bytes)
		values ($1, $2, $3, $4, $5)
		on conflict (ref) do nothing`,
		ref, string(runID), seq, digest, data); err != nil {
		return "", fmt.Errorf("ledger: store content for %s: %w", runID, err)
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
