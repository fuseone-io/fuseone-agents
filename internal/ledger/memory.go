// Package ledger provides storage for the append-only run ledger.
//
// The interface it satisfies is declared by its consumer, in package engine.
// Go interfaces are structural, so nothing here imports engine.
package ledger

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

var (
	// ErrSeqConflict means another writer already claimed that sequence
	// number. It is the in-memory equivalent of the primary key violation on
	// run_steps, and it is how single-writer is enforced (PRD NF-15).
	ErrSeqConflict = errors.New("ledger: sequence already claimed by another writer")
	// ErrIdemConflict means the same idempotency key was already recorded.
	ErrIdemConflict = errors.New("ledger: idempotency key already used")
	ErrNotFound     = errors.New("ledger: run not found")
)

// Memory is an in-memory ledger used by tests and by single-node development.
//
// It enforces the same invariants as the Postgres implementation on purpose:
// a test that passes here must not fail in production because the fake was
// more permissive than the real thing.
type Memory struct {
	mu    sync.RWMutex
	runs  map[domain.RunID][]domain.Step
	idems map[string]struct{}

	// Worker coordination, mirroring the columns migration 0003 adds.
	leases        map[domain.RunID]leaseState
	owners        map[domain.RunID]string
	lastError     map[domain.RunID]string
	lastFailure   map[domain.RunID]domain.FailureSummary
	lastFailureAt map[domain.RunID]time.Time

	// Clock is injectable so lease expiry is testable without sleeping.
	Clock func() time.Time
}

func NewMemory() *Memory {
	return &Memory{
		runs:          make(map[domain.RunID][]domain.Step),
		idems:         make(map[string]struct{}),
		leases:        make(map[domain.RunID]leaseState),
		owners:        make(map[domain.RunID]string),
		lastError:     make(map[domain.RunID]string),
		lastFailure:   make(map[domain.RunID]domain.FailureSummary),
		lastFailureAt: make(map[domain.RunID]time.Time),
	}
}

// Append seals s against the run's current head and stores it.
//
// The caller supplies the step without Seq, PrevHash or Hash: those are the
// ledger's to assign, because only the ledger knows the head at commit time.
func (m *Memory) Append(ctx context.Context, s domain.Step) (domain.Step, error) {
	if err := ctx.Err(); err != nil {
		return domain.Step{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if s.IdemKey != "" {
		if _, used := m.idems[s.IdemKey]; used {
			return domain.Step{}, ErrIdemConflict
		}
	}

	steps := m.runs[s.RunID]
	var prev *domain.Step
	if n := len(steps); n > 0 {
		prev = &steps[n-1]
	}

	sealed, err := domain.NewStep(prev, s)
	if err != nil {
		return domain.Step{}, err
	}

	m.runs[s.RunID] = append(steps, sealed)
	if sealed.IdemKey != "" {
		m.idems[sealed.IdemKey] = struct{}{}
	}
	if sealed.Kind != domain.StepParked {
		delete(m.lastFailure, sealed.RunID)
		delete(m.lastFailureAt, sealed.RunID)
	}
	return sealed, nil
}

func (m *Memory) now() time.Time {
	if m.Clock != nil {
		return m.Clock()
	}
	return time.Now()
}

// Read returns the run's steps from fromSeq onward, in order.
func (m *Memory) Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	steps, ok := m.runs[runID]
	if !ok {
		return nil, ErrNotFound
	}
	if fromSeq <= domain.FirstSeq {
		return append([]domain.Step(nil), steps...), nil
	}

	idx := int(fromSeq - domain.FirstSeq)
	if idx >= len(steps) {
		return nil, nil
	}
	return append([]domain.Step(nil), steps[idx:]...), nil
}

// Head returns the last sealed step of a run.
func (m *Memory) Head(ctx context.Context, runID domain.RunID) (domain.Step, error) {
	if err := ctx.Err(); err != nil {
		return domain.Step{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	steps := m.runs[runID]
	if len(steps) == 0 {
		return domain.Step{}, ErrNotFound
	}
	return steps[len(steps)-1], nil
}

// Runs lists every run this ledger holds, newest first by opening step.
//
// The Postgres implementation will answer this from a materialised projection
// rather than by scanning steps; folding every run to build a list is fine for
// a development ledger and will not survive real volume.
func (m *Memory) Runs(ctx context.Context) ([]domain.RunID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	type entry struct {
		id domain.RunID
		at time.Time
	}
	entries := make([]entry, 0, len(m.runs))
	for id, steps := range m.runs {
		if len(steps) > 0 {
			entries = append(entries, entry{id, steps[0].At})
		}
	}
	slices.SortFunc(entries, func(a, b entry) int {
		if c := b.at.Compare(a.at); c != 0 {
			return c
		}
		return strings.Compare(string(a.id), string(b.id))
	})

	out := make([]domain.RunID, len(entries))
	for i, e := range entries {
		out[i] = e.id
	}
	return out, nil
}

// Verify walks the whole chain for a run (PRD NF-05, AU-12).
func (m *Memory) Verify(ctx context.Context, runID domain.RunID) error {
	steps, err := m.Read(ctx, runID, domain.FirstSeq)
	if err != nil {
		return err
	}
	return domain.VerifyChain(steps)
}
