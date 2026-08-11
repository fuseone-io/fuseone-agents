package trigger

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/fuseone/agents/internal/domain"
)

// The scheduler fires schedules. What makes it safe is not a lock.
//
// Every due moment has one name — the agent and the instant, nothing else —
// and that name is the idempotency key. Two workers waking in the same minute
// compute the same key and the ledger accepts one of them. No lease to expire,
// no clock to agree on, no lock to leak: the guarantee is the same unique index
// that already protects every other effect on the platform.

// Schedules is where the next due moment of each trigger is kept.
type Schedules interface {
	Due(ctx context.Context, at time.Time) ([]Due, error)
	Advance(ctx context.Context, agent domain.AgentID, schedule string, next time.Time) error
	Sync(ctx context.Context, agent domain.AgentID, schedules []string, from time.Time) error
}

// Due is one trigger whose moment has arrived.
type Due struct {
	Agent domain.AgentID
	// Schedule is the cron expression, kept so a changed schedule is a
	// different row rather than a silent rewrite of the old one's history.
	Schedule string
	// At is the moment that came due — not the moment it is being handled.
	// The key is derived from this, so a late worker still opens the run that
	// moment named rather than a new one.
	At time.Time
}

// Grace is how late a moment may be handled and still fire.
//
// A moment that passed while nothing was running is not replayed. Firing every
// slot missed during an outage would empty an hour of backlog into a system
// that has just come back up, and firing the newest of them at start-up would
// mean deploying the platform runs every scheduled agent at once.
const Grace = 2 * time.Minute

// Scheduler opens runs when schedules come due.
type Scheduler struct {
	schedules Schedules
	opener    *Opener
	clock     Clock
	log       *slog.Logger
}

func NewScheduler(schedules Schedules, opener *Opener, clock Clock, log *slog.Logger) *Scheduler {
	return &Scheduler{schedules: schedules, opener: opener, clock: clock, log: log}
}

// Tick fires everything due, and reports how many runs it opened.
//
// Errors on one trigger do not stop the others: a schedule pointing at an
// agent somebody unpublished must not keep every other schedule from running.
func (s *Scheduler) Tick(ctx context.Context) (opened int, err error) {
	now := s.clock.Now()

	due, err := s.schedules.Due(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("trigger: read due schedules: %w", err)
	}

	for _, d := range due {
		next, parseErr := NextAfter(d.Schedule, now)
		if parseErr != nil {
			// A schedule that cannot be parsed cannot be advanced either, so
			// it would be read as due on every tick forever.
			s.log.Error("unparseable schedule; it will not fire",
				"agent", d.Agent, "schedule", d.Schedule, "err", parseErr)
			continue
		}
		if advErr := s.schedules.Advance(ctx, d.Agent, d.Schedule, next); advErr != nil {
			s.log.Error("could not advance a schedule", "agent", d.Agent, "err", advErr)
			continue
		}

		if now.Sub(d.At) > Grace {
			s.log.Warn("schedule missed its moment and was not replayed",
				"agent", d.Agent, "moment", d.At, "late", now.Sub(d.At))
			continue
		}

		result, openErr := s.opener.Open(ctx, Request{
			Agent:   d.Agent,
			IdemKey: KeyFor(d.Agent, d.At),
			Trigger: "cron",
		})
		if openErr != nil {
			s.log.Error("could not open a scheduled run", "agent", d.Agent, "err", openErr)
			continue
		}
		if result.Created {
			opened++
			s.log.Info("scheduled run opened", "agent", d.Agent, "run", result.RunID, "moment", d.At)
		}
	}
	return opened, nil
}

// KeyFor names a due moment. Deterministic in the agent and the instant, so
// every worker that sees the same moment computes the same key.
func KeyFor(agent domain.AgentID, at time.Time) string {
	return fmt.Sprintf("cron:%s:%d", agent, at.UTC().Unix())
}

// parser accepts the five-field form and the descriptors, and nothing else.
// Seconds are deliberately absent: a schedule finer than a minute is a queue,
// and this is not one.
var parser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// NextAfter is when a schedule next comes due, strictly after t.
func NextAfter(schedule string, t time.Time) (time.Time, error) {
	parsed, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("trigger: schedule %q: %w", schedule, err)
	}
	return parsed.Next(t).UTC(), nil
}

// ValidSchedule reports whether a specification's schedule can ever fire. Used
// at publication time, so a typo is refused by the author rather than found by
// an operator wondering why nothing runs.
func ValidSchedule(schedule string) error {
	_, err := parser.Parse(schedule)
	return err
}
