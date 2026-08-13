package ledger_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
Paging the run list, against both implementations.

The console shows a count taken over the whole set beside a list that stops at
the limit. Today that means a screen reporting twelve hundred runs and offering
fifty, with the other eleven hundred and fifty unreachable — a number that
tells you something exists and a list that will not show it.

What follows asserts the property a page has to have to be worth anything:
walk the pages and you have walked the list, each run once. The in-memory
ledger has to hold it too, or every suite built on the fake certifies a
behaviour production does not have.
*/
func TestListRuns_walkedInPages_yieldsEveryRunExactlyOnce(t *testing.T) {
	for _, impl := range implementations(t) {
		t.Run(impl.name, func(t *testing.T) {
			store := impl.open(t)
			// Several share a start, because ties are where a cursor built
			// from time alone loses a row or repeats one.
			for i := range 23 {
				startRun(t, store, fmt.Sprintf("run-%02d", i),
					noon.Add(time.Duration(i/3)*time.Minute))
			}

			whole, err := store.ListRuns(t.Context(), domain.RunFilter{}, "", 100)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(whole) != 23 {
				t.Fatalf("seeded 23 runs, the unpaged list has %d", len(whole))
			}

			var walked []domain.RunSummary
			filter := domain.RunFilter{}
			for range 12 {
				page, err := store.ListRuns(t.Context(), filter, "", 5)
				if err != nil {
					t.Fatalf("list page: %v", err)
				}
				walked = append(walked, page...)
				if len(page) < 5 {
					break
				}
				filter.After = domain.RunCursorAt(page[len(page)-1])
			}

			if len(walked) != len(whole) {
				t.Fatalf("walking the pages gave %d runs; the list holds %d", len(walked), len(whole))
			}
			for i := range whole {
				if walked[i].RunID != whole[i].RunID {
					t.Fatalf("run %d: paged %s, whole %s", i, walked[i].RunID, whole[i].RunID)
				}
			}
		})
	}
}

// A cursor is a position in an ordering, not a promise about the filter. The
// same cursor under a narrower filter must not reach past it.
func TestListRuns_cursorUnderANarrowerFilter_staysNarrow(t *testing.T) {
	for _, impl := range implementations(t) {
		t.Run(impl.name, func(t *testing.T) {
			store := impl.open(t)
			for i := range 10 {
				startRunIn(t, store, fmt.Sprintf("run-%02d", i),
					noon.Add(time.Duration(i)*time.Minute),
					domain.Scope{Company: "acme", Area: domain.AreaID([]string{"finance", "support"}[i%2])})
			}

			page, err := store.ListRuns(t.Context(), domain.RunFilter{}, "", 3)
			if err != nil {
				t.Fatalf("list: %v", err)
			}

			narrowed := domain.RunFilter{
				After:  domain.RunCursorAt(page[len(page)-1]),
				Scopes: []domain.Scope{{Company: "acme", Area: "finance"}},
			}
			rest, err := store.ListRuns(t.Context(), narrowed, "", 100)
			if err != nil {
				t.Fatalf("list narrowed: %v", err)
			}
			for _, r := range rest {
				if r.Scope.Area != "finance" {
					t.Fatalf("a cursor carried a run from %s past the scope filter", r.Scope.Area)
				}
			}
		})
	}
}

var noon = time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

func startRun(t *testing.T, store Store, id string, at time.Time) {
	t.Helper()
	startRunIn(t, store, id, at, domain.Scope{Company: "acme", Area: "cx"})
}

func startRunIn(t *testing.T, store Store, id string, at time.Time, scope domain.Scope) {
	t.Helper()
	s := step(domain.RunID(id), domain.StepRunStarted)
	s.At, s.Scope = at, scope
	if _, err := store.Append(t.Context(), s); err != nil {
		t.Fatalf("start %s: %v", id, err)
	}
}
