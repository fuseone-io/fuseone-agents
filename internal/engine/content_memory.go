package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/fuseone/agents/internal/domain"
)

// MemoryContent is a ContentStore for development and tests.
//
// Production points at object storage in the customer's own account: prompts,
// tool arguments and tool results routinely carry personal data, and the
// retention rules that apply to them are the customer's, not ours.
type MemoryContent struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMemoryContent() *MemoryContent {
	return &MemoryContent{data: make(map[string][]byte)}
}

func (m *MemoryContent) Put(ctx context.Context, runID domain.RunID, seq int64, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	ref := fmt.Sprintf("mem://%s/%d/%s", runID, seq, hex.EncodeToString(sum[:])[:12])

	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[ref] = append([]byte(nil), data...)
	return ref, nil
}

func (m *MemoryContent) Get(ctx context.Context, ref string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.data[ref]
	if !ok {
		return nil, fmt.Errorf("content: no object at %s", ref)
	}
	return append([]byte(nil), data...), nil
}
