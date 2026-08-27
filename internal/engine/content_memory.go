package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// MemoryContent is a ContentStore for development and tests.
//
// Production points at object storage in the customer's own account: prompts,
// tool arguments and tool results routinely carry personal data, and the
// retention rules that apply to them are the customer's, not ours.
type MemoryContent struct {
	mu   sync.RWMutex
	data map[string]object
	// limit is how much of one payload is kept, matching what the durable
	// store bounds. A fake that accepted what production truncates would let
	// every suite that uses it certify behaviour the real thing lacks.
	limit int
}

// WithLimit bounds what one payload may occupy. Zero is no limit.
func (m *MemoryContent) WithLimit(bytes int) *MemoryContent {
	out := NewMemoryContent()
	out.limit = bytes

	m.mu.RLock()
	defer m.mu.RUnlock()
	for ref, held := range m.data {
		out.data[ref] = held
	}
	return out
}

// object is one stored payload and when it was put there, so the fake can
// answer retention the way the durable store does.
type object struct {
	bytes []byte
	// digest is of the whole payload, kept beside the truncated bytes for the
	// same reason the durable store keeps it in a column: what the record says
	// it is must not change because only part of it was retained.
	digest string
	owner  string
	at     time.Time
	erased bool
}

// NewMemoryContent bounds payloads by the same default the durable store uses.
func NewMemoryContent() *MemoryContent {
	return &MemoryContent{
		data:  make(map[string]object),
		limit: domain.DefaultContentLimit,
	}
}

func (m *MemoryContent) Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error) {
	return m.PutFor(ctx, "run", string(runID), seq, data)
}

// PutFor stores content belonging to something other than a run — a set of
// simulation cases, so far.
//
// The reference is built exactly as the durable store builds it, owner and
// all. A fake that filed two owners under one reference would let a test pass
// on a store that cannot purge a run without taking a case set with it.
func (m *MemoryContent) PutFor(
	ctx context.Context, kind, owner string, seq int64, data []byte,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// The digest is of the whole payload even when only part of it is kept,
	// exactly as the durable store does it.
	sum := sha256.Sum256(data)
	ref := fmt.Sprintf("%s://%s/%d/%s", kind, owner, seq, hex.EncodeToString(sum[:])[:16])

	// The same rule the durable store applies, from the same place. Two copies
	// of it is two rules, and the one that drifts is this one — which is the
	// copy every test that avoids a database trusts.
	stored, _ := domain.Truncate(data, m.limit)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[ref] = object{
		bytes:  append([]byte(nil), stored...),
		digest: hex.EncodeToString(sum[:]),
		owner:  owner,
		at:     time.Now(),
	}
	return ref, nil
}

func (m *MemoryContent) Get(ctx context.Context, ref string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	held, ok := m.data[ref]
	if !ok {
		return nil, fmt.Errorf("content: no object at %s", ref)
	}
	if held.erased {
		// The same distinction the durable store makes: erased and
		// never-stored are different facts, and a fake that conflated them
		// would let a screen ship that cannot tell an auditor which happened.
		return nil, fmt.Errorf("%w: %s", domain.ErrContentErased, ref)
	}
	return append([]byte(nil), held.bytes...), nil
}

/*
Metadata answers what a reference says about itself without reading the bytes.

The whole digest, not the reference's 16-hex prefix, and reported even after
erasure — the same two facts the durable store answers, because a resolver that
worked against the fake and not against Postgres would be a resolver nobody
tested.
*/
func (m *MemoryContent) Metadata(ctx context.Context, ref string) (domain.ContentMetadata, error) {
	if err := ctx.Err(); err != nil {
		return domain.ContentMetadata{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	held, ok := m.data[ref]
	if !ok {
		return domain.ContentMetadata{}, fmt.Errorf("content: no object at %s", ref)
	}
	return domain.ContentMetadata{Digest: held.digest, Erased: held.erased}, nil
}

// Erase removes one owner's content, leaving a tombstone.
func (m *MemoryContent) Erase(ctx context.Context, owner string, reason string) (int, error) {
	return m.erase(ctx, func(o object) bool { return o.owner == owner })
}

// ErasePast is retention: everything stored before a moment goes.
func (m *MemoryContent) ErasePast(ctx context.Context, before time.Time, reason string) (int, error) {
	return m.erase(ctx, func(o object) bool { return o.at.Before(before) })
}

func (m *MemoryContent) erase(ctx context.Context, matches func(object) bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for ref, held := range m.data {
		if held.erased || !matches(held) {
			continue
		}
		// The bytes go; the digest stays, as it does in the durable store where
		// erasure sets a timestamp and leaves the row's digest alone. The step
		// that referenced this content still carries that number, so a reader
		// can tell "the bytes were erased" from "this citation was never true".
		m.data[ref] = object{
			digest: held.digest, owner: held.owner, at: held.at, erased: true,
		}
		count++
	}
	return count, nil
}
