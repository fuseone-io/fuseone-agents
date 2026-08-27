package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/memory"
)

/*
A reconciliation walks to the end of both tables and adds up what it did.

Pages rather than one read, because a sweep that read everything would hold a
transaction open across the whole table. Two cursors rather than one, because
the two tables have different populations and one finishing says nothing about
the other — a single cursor could only ever describe one of them.
*/
func TestReconcile_walksBothTablesToTheEnd(t *testing.T) {
	t.Parallel()
	sweeps := &countingSweeps{assertions: 3, suggestions: 2}

	got, pending, err := memory.Reconcile(context.Background(), sweeps, nil,
		time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// One more page than there were rows to repair: the walk ends when a page
	// says there is no next one, and asking is how it finds that out.
	if got.Pages != 4 || pending.Pages != 3 {
		t.Errorf("pages = %d and %d, want each table walked to its own end",
			got.Pages, pending.Pages)
	}
	if got.Repaired != 3 || pending.Repaired != 2 {
		t.Errorf("repaired = %d and %d, want every page added up", got.Repaired, pending.Repaired)
	}
}

// Every page is dated by the moment the job started, not by a clock read per
// page. Rows from one run filed under times that drift apart would be a trail
// nobody can read as one act.
func TestReconcile_everyPageCarriesTheSameMoment(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	sweeps := &countingSweeps{assertions: 4}

	if _, _, err := memory.Reconcile(context.Background(), sweeps, nil, started); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	for _, at := range sweeps.moments {
		if !at.Equal(started) {
			t.Fatalf("a page was dated %s, want every one at %s", at, started)
		}
	}
}

/*
A cursor that does not advance stops the job instead of being retried for ever.

The sweep hands back where the next page starts. Being given the same place back
means nothing moved, and the loop would ask for it again — quietly, at full
speed, until the deadline. The exit code is what makes that visible.
*/
func TestReconcile_cursorThatDoesNotAdvance_stops(t *testing.T) {
	t.Parallel()
	sweeps := &countingSweeps{assertions: 2, stuck: true}

	_, _, err := memory.Reconcile(context.Background(), sweeps, nil, time.Now())
	if !errors.Is(err, memory.ErrNoProgress) {
		t.Fatalf("Reconcile = %v, want the job stopped rather than looping", err)
	}
}

/*
Rows that need a person are reported, not treated as a failure.

Neither improves with a retry: a conflicted identity is two rows somebody has to
choose between, and an unproved citation is evidence the ledger will not vouch
for. Failing the job would fail a release, repeat the same reading until the
backoff limit, and resolve nothing — while the operator reads an exit code
instead of a count.
*/
func TestReconcile_rowsNeedingAPerson_areReportedAndNotAFailure(t *testing.T) {
	t.Parallel()
	sweeps := &countingSweeps{assertions: 2, conflicted: 1, unproved: 2}

	got, _, err := memory.Reconcile(context.Background(), sweeps, nil, time.Now())
	if err != nil {
		t.Fatalf("Reconcile = %v, want rows needing a person reported rather than raised", err)
	}
	if got.Conflicted != 2 || got.Unproved != 4 {
		t.Errorf("conflicted = %d and unproved = %d, want both counted across pages",
			got.Conflicted, got.Unproved)
	}
	if !got.NeedsReview() {
		t.Error("the totals do not ask for a person, so nothing would warn")
	}
}

func TestReconcile_nothingToRepair_asksForNobody(t *testing.T) {
	t.Parallel()
	got, pending, err := memory.Reconcile(context.Background(),
		&countingSweeps{}, nil, time.Now())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got.NeedsReview() || pending.NeedsReview() {
		t.Error("a clean sweep asked for a person")
	}
}

// A failure of the store is the job's failure. It is the one thing here a retry
// can fix, and the one thing an operator cannot see from a count.
func TestReconcile_storeFailure_stopsTheJob(t *testing.T) {
	t.Parallel()
	_, _, err := memory.Reconcile(context.Background(),
		&countingSweeps{fail: errors.New("dial tcp: connection refused")}, nil, time.Now())
	if err == nil {
		t.Fatal("Reconcile succeeded while the store was refusing")
	}
}

/*
countingSweeps answers a fixed number of pages, and can misbehave on purpose.

A stub rather than the in-memory store because what is under test is the loop:
how many pages it asks for, what moment it dates them with, and what it does
when the answer stops making sense. The store's own behaviour is pinned by the
contract tests both stores run.
*/
type countingSweeps struct {
	assertions, suggestions int
	conflicted, unproved    int
	stuck                   bool
	fail                    error

	moments []time.Time
	seenA   int
	seenS   int
}

func (s *countingSweeps) Hydrate(
	_ context.Context, _ *memory.Resolver, page memory.HydratePage,
) (memory.HydrateResult, error) {
	s.seenA++
	return s.page(page, s.seenA, s.assertions)
}

func (s *countingSweeps) HydrateSuggestions(
	_ context.Context, _ *memory.Resolver, page memory.HydratePage,
) (memory.HydrateResult, error) {
	s.seenS++
	return s.page(page, s.seenS, s.suggestions)
}

func (s *countingSweeps) page(
	page memory.HydratePage, nth, pages int,
) (memory.HydrateResult, error) {
	if s.fail != nil {
		return memory.HydrateResult{}, s.fail
	}
	s.moments = append(s.moments, page.Now)
	if nth > pages {
		// The last page of a walk: fewer rows than the limit, so the sweep says
		// there is no next one.
		return memory.HydrateResult{}, nil
	}
	out := memory.HydrateResult{
		Scanned: 1, Repaired: 1,
		Conflicted: s.conflicted, Unproved: s.unproved,
		Cursor: "mem_" + string(rune('a'+nth)),
	}
	if s.stuck {
		out.Cursor = "mem_stuck"
	}
	return out, nil
}
