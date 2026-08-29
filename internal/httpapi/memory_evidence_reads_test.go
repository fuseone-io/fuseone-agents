package httpapi

import (
	"context"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	memstore "github.com/fuseone/agents/internal/memory"
)

func TestCreateMemoryAssertion_readsTheEvidenceRunOnce(t *testing.T) {
	t.Parallel()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	seedFinishedEvidence(t, store, "run-evidence", scope, domain.ScopeLabels(scope), "sha256:answer")
	counted := &countingEvidenceLedger{Memory: store}
	req := memoryCreateRequest()

	resp, err := NewServer(counted, "test").WithMemory(memstore.NewMemory()).
		WithMemoryEvidence(counted, memoryContentFor(store)).
		CreateMemoryAssertion(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("CreateMemoryAssertion: %v", err)
	}
	if _, ok := resp.(openapi.CreateMemoryAssertion200JSONResponse); !ok {
		t.Fatalf("response = %T, want the assertion", resp)
	}
	if got := counted.readCount(); got != 1 {
		t.Fatalf("ledger reads = %d, want one proof answering evidence and agent", got)
	}
}

type countingEvidenceLedger struct {
	*ledger.Memory
	mu    sync.Mutex
	reads int
}

func (s *countingEvidenceLedger) Read(
	ctx context.Context, run domain.RunID, from int64,
) ([]domain.Step, error) {
	s.mu.Lock()
	s.reads++
	s.mu.Unlock()
	return s.Memory.Read(ctx, run, from)
}

func (s *countingEvidenceLedger) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}
