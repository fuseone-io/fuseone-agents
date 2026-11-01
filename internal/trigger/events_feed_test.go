package trigger_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
)

/*
A sweep reads the finished runs after a durable cursor, page after page, and
moves the cursor only once a page is dispatched (#203). It used to read the
thirty newest finished runs and remember nothing: the thirty-first to finish
between two sweeps was never dispatched.
*/

// finishSources finishes n runs of agent-a, whose event agent-b listens to.
func finishSources(t *testing.T, store *ledger.Memory, from, n int) {
	t.Helper()
	for i := from; i < from+n; i++ {
		id := domain.RunID(fmt.Sprintf("run-%03d", i))
		startRun(t, store, id, "agent-a", nil)
		finishRun(t, store, id)
	}
}

func TestSweep_moreFinishedRunsThanAPage_dispatchesEveryOne(t *testing.T) {
	t.Parallel()

	cases := map[string]int{"one past a page": 31, "a burst of pages": 75}
	for name, n := range cases {
		store := ledger.NewMemory()
		finishSources(t, store, 0, n)

		opened, err := chainDispatcher(store).Sweep(context.Background(), 30)
		if err != nil {
			t.Fatalf("%s: Sweep: %v", name, err)
		}
		if opened != n {
			t.Errorf("%s: opened %d runs for %d finished, want every one", name, opened, n)
		}
	}
}

func TestSweep_runsFinishingBetweenSweeps_areDispatchedByTheNext(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	finishSources(t, store, 0, 31)
	first, err := chainDispatcher(store).Sweep(context.Background(), 30)
	if err != nil {
		t.Fatalf("first Sweep: %v", err)
	}
	finishSources(t, store, 31, 5)
	second, err := chainDispatcher(store).Sweep(context.Background(), 30)
	if err != nil {
		t.Fatalf("second Sweep: %v", err)
	}
	third, err := chainDispatcher(store).Sweep(context.Background(), 30)
	if err != nil {
		t.Fatalf("third Sweep: %v", err)
	}

	if first != 31 || second != 5 || third != 0 {
		t.Fatalf("opened %d, %d, %d; want 31, then the 5 that finished since, then none", first, second, third)
	}
}

// failingEmits answers an error for every version until healed.
type failingEmits struct {
	wiring
	failing *bool
}

func (f failingEmits) EmitsOf(ctx context.Context, agent domain.AgentID, version domain.VersionID) ([]domain.AgentEvent, error) {
	if *f.failing {
		return nil, errors.New("registry unavailable")
	}
	return f.wiring.EmitsOf(ctx, agent, version)
}

// A page that fails leaves the cursor where it was: the next sweep reads the
// same runs again rather than moving past what it never dispatched.
func TestSweep_aPageThatFails_leavesTheCursorWhereItWas(t *testing.T) {
	t.Parallel()

	store := ledger.NewMemory()
	finishSources(t, store, 0, 3)
	failing := true
	subs := failingEmits{wiring: pingPong, failing: &failing}

	if _, err := dispatcherWith(store, subs).Sweep(context.Background(), 30); err == nil {
		t.Fatal("Sweep succeeded while the registry failed")
	}
	if cursor, _ := store.EventCursor(context.Background()); cursor != (domain.FinishCursor{}) {
		t.Fatalf("cursor moved to %+v past a page that failed", cursor)
	}
	failing = false
	opened, err := dispatcherWith(store, subs).Sweep(context.Background(), 30)
	if err != nil || opened != 3 {
		t.Fatalf("after healing: opened %d, %v; want the 3 runs dispatched", opened, err)
	}
}
