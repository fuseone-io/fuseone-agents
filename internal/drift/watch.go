/*
Package drift notices when nobody changed anything and it stopped working.

A provider ships a new model under a name that did not change and behaviour
moves. Inside somebody else's network nobody finds out until it is an
incident — there is no dashboard to watch and no announcement to read — and
the whole argument for keeping a corpus is that the platform says so first.

The reading is two batteries of the *same* version. A version's identifier is
the digest of its own bytes, so a version that did not change is an agent that
did not change: a correction that held last night and does not hold this
morning is the world moving underneath it, not somebody's edit. That is why
this is a different fact from the one the start gate checks, and why it is
worth a different notice.
*/
package drift

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/simulate"
)

// Corpora is which agents are worth watching, declared here by the consumer.
type Corpora interface {
	Watching(ctx context.Context) ([]domain.WatchedCorpus, error)
}

// Batteries is where the runs of one version's simulations are found.
type Batteries interface {
	Batteries(ctx context.Context, agent domain.AgentID, version domain.VersionID, limit int) ([]string, error)
}

// Folds turns a simulation into the report it always was.
type Folds interface {
	Fold(ctx context.Context, simulation string) (simulate.Report, error)
}

// Announcer is where the notice goes (NT-005).
type Announcer interface {
	Announce(ctx context.Context, scope domain.Scope, m channel.Message) error
}

// Watch compares the last two batteries of each watched version.
type Watch struct {
	corpora   Corpora
	batteries Batteries
	folds     Folds
	announcer Announcer
	log       *slog.Logger
	// said is which drift has already been announced, by the battery that
	// found it. A sweep that repeated itself every pass is a sweep somebody
	// silences, and then the next notice is not heard either.
	said map[string]bool
}

func New(
	corpora Corpora, batteries Batteries, folds Folds,
	announcer Announcer, log *slog.Logger,
) *Watch {
	if log == nil {
		log = slog.Default()
	}
	return &Watch{
		corpora: corpora, batteries: batteries, folds: folds,
		announcer: announcer, log: log, said: map[string]bool{},
	}
}

// Sweep reads every watched agent and announces the ones that moved.
func (w *Watch) Sweep(ctx context.Context, now time.Time) (int, error) {
	watching, err := w.corpora.Watching(ctx)
	if err != nil {
		return 0, fmt.Errorf("drift: read the watched corpora: %w", err)
	}

	drifted := 0
	for _, one := range watching {
		found, err := w.compare(ctx, one)
		if err != nil {
			// Logged and left for the next sweep. One agent whose batteries
			// could not be read must not stop the others being read.
			w.log.Error("could not read for drift", "agent", one.Agent, "err", err)
			continue
		}
		if found == nil {
			continue
		}
		drifted++
		w.announce(ctx, one, *found, now)
	}
	return drifted, nil
}

// finding is drift, and the battery that found it — which is what "said
// once" is counted by, because the same drift found again by a later battery
// is a second thing worth saying.
type finding struct {
	battery string
	moved   simulate.Comparison
}

// compare answers with the drift between a version's last two batteries, or
// nothing when there is nothing to say.
func (w *Watch) compare(ctx context.Context, one domain.WatchedCorpus) (*finding, error) {
	found, err := w.batteries.Batteries(ctx, one.Agent, one.Version, 2)
	if err != nil {
		return nil, err
	}
	// One battery is a reading, not a comparison. Announcing on it would fire
	// once for every agent the first time this ever ran.
	if len(found) < 2 {
		return nil, nil
	}
	newest, previous := found[0], found[1]
	if w.said[newest] {
		return nil, nil
	}

	now, err := w.folds.Fold(ctx, newest)
	if err != nil {
		return nil, err
	}
	was, err := w.folds.Fold(ctx, previous)
	if err != nil {
		return nil, err
	}
	// Half a fold is not an answer: every case that has not run yet would
	// read as a correction that stopped holding.
	if now.Running || was.Running {
		return nil, nil
	}

	compared := simulate.Compare(was, now)
	if compared.Regressed == 0 {
		return nil, nil
	}
	return &finding{battery: newest, moved: compared}, nil
}

// announce says it once, and says which correction moved.
//
// Named rather than counted, for the same reason the start gate names them:
// "three corrections moved" sends somebody to find out which, and the whole
// reason a correction has an identifier is so it can be pointed at.
func (w *Watch) announce(ctx context.Context, one domain.WatchedCorpus, found finding, now time.Time) {
	w.said[found.battery] = true
	w.log.Warn("an agent drifted", "agent", one.Agent, "version", one.Version,
		"regressed", found.moved.Regressed, "at", now)

	if w.announcer == nil {
		return
	}
	message := channel.Message{
		Event:  channel.EventDrifted,
		Agent:  one.Agent,
		Scope:  one.Scope,
		Reason: reasonOf(found.moved),
	}
	if err := w.announcer.Announce(ctx, one.Scope, message); err != nil {
		// The notice failing is the failure mode this whole loop exists to
		// prevent, so it is loud. Left unmarked, so the next sweep tries again.
		w.log.Error("drift could not be announced", "agent", one.Agent, "err", err)
		delete(w.said, found.battery)
	}
}

func reasonOf(moved simulate.Comparison) string {
	first := ""
	for _, c := range moved.Cases {
		if c.Regressed() {
			first = c.ID
			break
		}
	}
	if moved.Regressed == 1 {
		return fmt.Sprintf("%s stopped holding, with nothing published since", first)
	}
	return fmt.Sprintf("%d corrections stopped holding, starting with %s, "+
		"with nothing published since", moved.Regressed, first)
}
