package drift

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

/*
Running the corpus on a clock.

The corpus was only ever run when somebody asked, which makes it a thing that
catches what an author broke and nothing else. Drift is the other half: the
model moved and nobody did anything, so nobody is looking.

Every pass spends real money at a real provider, which is the whole design
constraint here. It runs only where there is a corpus, only when the last
battery is older than the interval, and it opens simulated runs — the same
ones a person's battery opens, claimed by the same pool, so there is no second
kind of work to keep in step.
*/

// LastBatteries is when a version's corpus last ran, declared by the consumer.
type LastBatteries interface {
	LastBatteryAt(ctx context.Context, agent domain.AgentID, version domain.VersionID) (time.Time, bool, error)
}

// Occurrences reads the cases of a corpus, bytes and all.
type Occurrences interface {
	Occurrences(ctx context.Context, agent domain.AgentID) ([]simulate.Occurrence, error)
}

// Default is how often a corpus runs when nobody has said otherwise.
//
// Daily. Often enough that a model that moved is caught within a working day,
// rare enough that a corpus of fifty cases across ten agents is a nightly
// cost somebody can predict rather than a surprise on an invoice.
const Default = 24 * time.Hour

// Rerun opens a battery for each watched corpus that is due for one.
type Rerun struct {
	corpora     Corpora
	last        LastBatteries
	occurrences Occurrences
	opener      simulate.Opener
	every       time.Duration
	log         *slog.Logger
}

func NewRerun(
	corpora Corpora, last LastBatteries, occurrences Occurrences,
	opener simulate.Opener, log *slog.Logger,
) *Rerun {
	if log == nil {
		log = slog.Default()
	}
	return &Rerun{
		corpora: corpora, last: last, occurrences: occurrences,
		opener: opener, every: Default, log: log,
	}
}

// Every sets how often a corpus is re-run. Zero or less keeps the default:
// an interval of zero would mean a battery on every pass of the loop, which
// is a way to spend a month's model budget in an afternoon.
func (r *Rerun) Every(interval time.Duration) *Rerun {
	if interval > 0 {
		r.every = interval
	}
	return r
}

// Sweep opens a battery for every corpus that is due one.
func (r *Rerun) Sweep(ctx context.Context, now time.Time) (int, error) {
	watching, err := r.corpora.Watching(ctx)
	if err != nil {
		return 0, fmt.Errorf("drift: read the watched corpora: %w", err)
	}

	opened := 0
	for _, one := range watching {
		due, err := r.due(ctx, one, now)
		if err != nil {
			// Logged and left for the next pass. One corpus that could not be
			// read must not stop the others running.
			r.log.Error("could not tell whether a corpus is due",
				"agent", one.Agent, "err", err)
			continue
		}
		if !due {
			continue
		}
		if err := r.run(ctx, one, now); err != nil {
			r.log.Error("could not run a corpus", "agent", one.Agent, "err", err)
			continue
		}
		opened++
	}
	return opened, nil
}

// due reports whether this corpus has gone longer than the interval. A corpus
// nobody has ever run is due: otherwise drift is undetectable for every agent
// published before this existed.
func (r *Rerun) due(ctx context.Context, one domain.WatchedCorpus, now time.Time) (bool, error) {
	at, ran, err := r.last.LastBatteryAt(ctx, one.Agent, one.Version)
	if err != nil {
		return false, err
	}
	return !ran || now.Sub(at) >= r.every, nil
}

// run opens one simulated run per case.
func (r *Rerun) run(ctx context.Context, one domain.WatchedCorpus, now time.Time) error {
	cases, err := r.occurrences.Occurrences(ctx, one.Agent)
	if err != nil {
		return fmt.Errorf("read the corpus of %s: %w", one.Agent, err)
	}
	if len(cases) == 0 {
		return nil
	}

	opened, err := simulate.Open(ctx, r.opener, simulate.Batch{
		ID:    batteryID(one, now, r.every),
		Agent: one.Agent,
		// The platform asked, and the trail says so. A battery attributed to
		// the last person who touched the agent would put a cost on somebody
		// who was asleep.
		By:    "platform",
		Cases: cases,
	})
	if err != nil {
		return err
	}
	for _, reason := range opened.Failed {
		r.log.Warn("a case of a scheduled battery did not open",
			"agent", one.Agent, "reason", reason)
	}
	return nil
}

/*
batteryID names the battery after the window it belongs to.

Derived rather than random, for the reason the scheduler's key is: every
worker runs this loop, they do not coordinate, and two that sweep in the same
window ask for exactly the same runs — of which the ledger accepts one. A
random identifier would make every worker's battery a separate one, and an
installation with three workers would pay three times.
*/
func batteryID(one domain.WatchedCorpus, now time.Time, every time.Duration) string {
	window := now.UTC().Truncate(every).Unix()
	return fmt.Sprintf("drift_%s_%s_%d", one.Agent, shortened(one.Version), window)
}

func shortened(v domain.VersionID) string {
	if len(v) > 9 {
		return string(v[:9])
	}
	return string(v)
}
