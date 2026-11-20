package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrTooLarge is a payload that must be kept whole and does not fit the store's limit.
var ErrTooLarge = errors.New("ledger: content too large to keep whole")

/*
PutExactForTx keeps data whole, in tx, never over a tombstone (NT-018 phase 2).

For bytes a digest is cut from, a seal's: a prefix would be other bytes under the same
name, so a payload past the store's limit is refused rather than truncated. It lives and
dies with tx, so a seal that fails to commit leaves no content behind. And it never
restores: the reference names its owner and its digest, so the only row it can meet is the
one a retry wrote, or that row's tombstone, which may be a subject's erasure and stays.
*/
func (c *Content) PutExactForTx(
	ctx context.Context, tx pgx.Tx, kind, owner string, seq int64, data []byte,
) (string, error) {
	if c.limit > 0 && len(data) > c.limit {
		return "", fmt.Errorf("%w: %d bytes for %s, limit %d", ErrTooLarge, len(data), owner, c.limit)
	}
	return keepExactTx(ctx, tx, kind, owner, seq, data)
}

// contentRef names data kept for owner at seq: its kind, its owner, its place, and the
// first 16 hex of the digest of the whole payload, returned whole beside it.
func contentRef(kind, owner string, seq int64, data []byte) (ref, digest string) {
	sum := sha256.Sum256(data)
	digest = hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s://%s/%d/%s", kind, owner, seq, digest[:16]), digest
}

// keepExactTx keeps data whole in tx under its reference, and refuses a tombstone.
func keepExactTx(ctx context.Context, tx pgx.Tx, kind, owner string, seq int64, data []byte) (string, error) {
	ref, digest := contentRef(kind, owner, seq, data)
	if _, err := tx.Exec(ctx, `
		insert into run_content
			(ref, owner_kind, run_id, seq, digest, bytes, full_bytes, truncated)
		values ($1, $2, $3, $4, $5, $6, $7, false)
		on conflict (ref) do nothing`,
		ref, kind, owner, seq, digest, data, len(data)); err != nil {
		return "", fmt.Errorf("ledger: keep content for %s: %w", owner, err)
	}
	var erased *time.Time
	if err := tx.QueryRow(ctx, `select erased_at from run_content where ref = $1`, ref).Scan(&erased); err != nil {
		return "", fmt.Errorf("ledger: read back content for %s: %w", owner, err)
	}
	if erased != nil {
		return "", fmt.Errorf("%w: %s", ErrErased, ref)
	}
	return ref, nil
}
