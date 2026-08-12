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
}

// object is one stored payload and when it was put there, so the fake can
// answer retention the way the durable store does.
type object struct {
	bytes  []byte
	owner  string
	at     time.Time
	erased bool
}

func NewMemoryContent() *MemoryContent {
	return &MemoryContent{data: make(map[string]object)}
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
	sum := sha256.Sum256(data)
	ref := fmt.Sprintf("%s://%s/%d/%s", kind, owner, seq, hex.EncodeToString(sum[:])[:16])

	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[ref] = object{
		bytes: append([]byte(nil), data...), owner: owner, at: time.Now(),
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
		m.data[ref] = object{owner: held.owner, at: held.at, erased: true}
		count++
	}
	return count, nil
}
