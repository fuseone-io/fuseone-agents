/*
Package budget warns before the ceiling, not at it (PRD FO-05).

A hard limit that says nothing until it stops the work is a limit that is
discovered by a run parking mid-afternoon. Half, four fifths, and all of it:
the first is a note, the second is time to act, and the third is the
explanation for everything that stops next.

Crossings are recorded rather than delivered. This installation has no outbound
channel — no mail, no chat — so what the platform can honestly do is notice,
write it where it survives, and put it where somebody looking at the money will
see it. Wiring a channel to these records is a separate piece of work, named in
docs/NT-002 rather than implied here.
*/
package budget

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Thresholds are the fractions of a ceiling worth saying something about.
//
// Ascending, and only the highest one crossed is announced: a scope that goes
// from nothing to spent in one busy hour gets told it is at its ceiling, not
// told three times in a row.
var Thresholds = []int{50, 80, 100}

// Ceilings is what each scope may spend, declared here by the consumer.
type Ceilings interface {
	List(ctx context.Context) ([]domain.ScopeBudget, error)
}

// Spend is what a scope has actually consumed in a window.
type Spend interface {
	SpentSince(ctx context.Context, scope domain.Scope, since time.Time) (domain.Consumption, error)
}

// Marks remember which threshold a scope has already been told about, so a
// sweep every few minutes does not announce the same crossing all afternoon.
type Marks interface {
	Announced(ctx context.Context) (map[string]domain.BudgetMark, error)
	Announce(ctx context.Context, mark domain.BudgetMark) error
}

// Clock is injectable because which period a sweep falls in decides whether a
// mark is stale, and a test that could not choose the day could not test it.
type Clock interface {
	Now() time.Time
}

type Watcher struct {
	ceilings Ceilings
	spend    Spend
	marks    Marks
	clock    Clock
	log      *slog.Logger
}

func NewWatcher(c Ceilings, s Spend, m Marks, clock Clock, log *slog.Logger) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	return &Watcher{ceilings: c, spend: s, marks: m, clock: clock, log: log}
}

/*
Sweep reads every configured ceiling and announces the crossings.

Returns what it announced so a caller can act on it — a test asserts on it, and
an installation that grows an outbound channel has the list to send.
*/
func (w *Watcher) Sweep(ctx context.Context) ([]domain.BudgetMark, error) {
	ceilings, err := w.ceilings.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("budget: read ceilings: %w", err)
	}
	announced, err := w.marks.Announced(ctx)
	if err != nil {
		return nil, fmt.Errorf("budget: read marks: %w", err)
	}

	now := w.clock.Now()
	var crossed []domain.BudgetMark

	for _, ceiling := range ceilings {
		mark, ok, err := w.check(ctx, ceiling, announced, now)
		if err != nil {
			return crossed, err
		}
		if !ok {
			continue
		}
		if err := w.marks.Announce(ctx, mark); err != nil {
			return crossed, fmt.Errorf("budget: announce %s: %w", mark.Key(), err)
		}
		w.log.Warn("budget threshold crossed",
			"scope", mark.Scope.String(), "at", mark.Threshold,
			"spent", mark.SpentMicros, "ceiling", mark.CeilingMicros)
		crossed = append(crossed, mark)
	}
	return crossed, nil
}

// check is one scope: what it may spend, what it has, and whether that is news.
func (w *Watcher) check(
	ctx context.Context, ceiling domain.ScopeBudget,
	announced map[string]domain.BudgetMark, now time.Time,
) (domain.BudgetMark, bool, error) {
	// Only an amount. The other dimensions are per run rather than per period,
	// and a scope has no meaningful "80% of its steps".
	if !ceiling.Enabled || ceiling.Budget.Micros <= 0 || ceiling.Period == "" {
		return domain.BudgetMark{}, false, nil
	}

	since := ceiling.Period.Since(now)
	spent, err := w.spend.SpentSince(ctx, ceiling.Scope, since)
	if err != nil {
		return domain.BudgetMark{}, false, fmt.Errorf(
			"budget: read spend for %s: %w", ceiling.Scope, err)
	}

	reached := highestCrossed(spent.Micros, ceiling.Budget.Micros)
	if reached == 0 {
		return domain.BudgetMark{}, false, nil
	}

	mark := domain.BudgetMark{
		Scope: ceiling.Scope, Threshold: reached, Since: since,
		SpentMicros: spent.Micros, CeilingMicros: ceiling.Budget.Micros, At: now,
	}
	// Already said, unless the period rolled over or the scope climbed higher.
	// The period is part of the comparison because a new month starts the
	// warnings again, which is the only reading of a monthly budget that works.
	if said, ok := announced[mark.Key()]; ok &&
		said.Since.Equal(since) && said.Threshold >= reached {
		return domain.BudgetMark{}, false, nil
	}
	return mark, true, nil
}

// highestCrossed is the largest threshold this spend has reached, or zero.
func highestCrossed(spent, ceiling int64) int {
	reached := 0
	for _, t := range Thresholds {
		if spent*100 >= ceiling*int64(t) {
			reached = t
		}
	}
	return reached
}
