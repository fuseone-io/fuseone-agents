package ledger

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

func step(runID domain.RunID, kind domain.StepKind) domain.Step {
	return domain.Step{
		RunID:      runID,
		Kind:       kind,
		Scope:      domain.Scope{Company: "acme", Area: "cx"},
		AgentID:    "triage",
		VersionID:  "v3",
		OnBehalfOf: "ana",
		At:         time.Now(),
	}
}

func TestAppend_successiveSteps_formVerifiableChain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l := NewMemory()

	for _, k := range []domain.StepKind{
		domain.StepRunStarted, domain.StepPlanned,
		domain.StepGateDecided, domain.StepToolCalled,
	} {
		if _, err := l.Append(ctx, step("run-1", k)); err != nil {
			t.Fatalf("Append(%s): %v", k, err)
		}
	}

	if err := l.Verify(ctx, "run-1"); err != nil {
		t.Errorf("Verify: %v", err)
	}

	steps, err := l.Read(ctx, "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("read %d steps, want 4", len(steps))
	}
	for i, s := range steps {
		if want := int64(i + 1); s.Seq != want {
			t.Errorf("steps[%d].Seq = %d, want %d", i, s.Seq, want)
		}
	}
}

func TestAppend_sameIdempotencyKeyTwice_secondRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l := NewMemory()

	call := step("run-1", domain.StepToolCalled)
	call.IdemKey = "run-1:4:crm.refund:9f2a"

	if _, err := l.Append(ctx, call); err != nil {
		t.Fatalf("first Append: %v", err)
	}

	// A resume after a crash replays the same call. Billing it twice is the
	// most expensive bug this architecture can have.
	if _, err := l.Append(ctx, call); !errors.Is(err, ErrIdemConflict) {
		t.Errorf("second Append = %v, want %v", err, ErrIdemConflict)
	}
}

func TestAppend_concurrentWriters_produceContiguousChainWithNoGaps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l := NewMemory()

	const writers = 32
	var wg sync.WaitGroup
	wg.Add(writers)
	for range writers {
		go func() {
			defer wg.Done()
			_, _ = l.Append(ctx, step("run-1", domain.StepPlanned))
		}()
	}
	wg.Wait()

	steps, err := l.Read(ctx, "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(steps) != writers {
		t.Fatalf("stored %d steps, want %d", len(steps), writers)
	}

	// Whatever the interleaving, the result must be one unbroken chain:
	// contiguous sequence numbers and every hash link intact.
	for i, s := range steps {
		if want := int64(i + 1); s.Seq != want {
			t.Fatalf("steps[%d].Seq = %d, want %d — gap or duplicate", i, s.Seq, want)
		}
	}
	if err := domain.VerifyChain(steps); err != nil {
		t.Errorf("VerifyChain after concurrent append: %v", err)
	}
}

func TestRead_fromMidChain_returnsRemainderOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l := NewMemory()

	for range 5 {
		if _, err := l.Append(ctx, step("run-1", domain.StepPlanned)); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	steps, err := l.Read(ctx, "run-1", 4)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("read %d steps from seq 4, want 2", len(steps))
	}
	if steps[0].Seq != 4 {
		t.Errorf("first step Seq = %d, want 4", steps[0].Seq)
	}
}

func TestRead_unknownRun_returnsNotFound(t *testing.T) {
	t.Parallel()

	_, err := NewMemory().Read(context.Background(), "nope", domain.FirstSeq)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Read = %v, want %v", err, ErrNotFound)
	}
}

func TestAppend_cancelledContext_doesNotWrite(t *testing.T) {
	t.Parallel()
	l := NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := l.Append(ctx, step("run-1", domain.StepPlanned)); !errors.Is(err, context.Canceled) {
		t.Errorf("Append = %v, want %v", err, context.Canceled)
	}
	if _, err := l.Head(context.Background(), "run-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a cancelled append left state behind: %v", err)
	}
}
