package simulate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/fuseone/agents/internal/domain"
)

// Runs is the read side a report needs, declared here by the consumer.
type Runs interface {
	SimulationRuns(ctx context.Context, simulation string) ([]domain.RunID, error)
	Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error)
}

// ErrBusy means the queue is full and this simulation was not accepted.
var ErrBusy = errors.New("simulate: another simulation is already queued")

/*
Service runs simulations away from the request that asked for one.

A case set is minutes of model calls, and an HTTP handler is the wrong place to
hold that open — the same reason starting a run returns before the run does.
So the work goes on a queue with a fixed depth, and the loop that drains it is
owned by whoever started the process.

The depth is small and Submit refuses when it is reached. Simulations cost real
money at a real provider, and an unbounded queue behind an HTTP handler is how
an afternoon of them gets started by accident.
*/
type Service struct {
	exec *Executor
	runs Runs
	jobs chan Job
	log  *slog.Logger

	// expected is how many cases each simulation was given. It is the only
	// thing the ledger cannot answer: a case that never opened a run leaves
	// nothing behind, and a report that counted only what ran would report
	// eighteen of eighteen for a set of twenty.
	mu       sync.Mutex
	expected map[string]int
	running  map[string]bool
}

func NewService(exec *Executor, runs Runs, queue int) *Service {
	if queue <= 0 {
		queue = 1
	}
	return &Service{
		exec: exec, runs: runs, jobs: make(chan Job, queue), log: slog.Default(),
		expected: make(map[string]int), running: make(map[string]bool),
	}
}

// WithLogger replaces the logger the loop reports failures on.
func (s *Service) WithLogger(log *slog.Logger) *Service {
	if log != nil {
		s.log = log
	}
	return s
}

// Submit accepts a job for the loop to run.
//
// It records what was accepted before it queues, so a report read in the
// moment between accepting and starting already says how many cases are
// coming rather than looking like an empty simulation.
func (s *Service) Submit(job Job) error {
	s.mu.Lock()
	s.expected[job.ID] = len(job.Cases)
	s.running[job.ID] = true
	s.mu.Unlock()

	select {
	case s.jobs <- job:
		return nil
	default:
		s.mu.Lock()
		delete(s.expected, job.ID)
		delete(s.running, job.ID)
		s.mu.Unlock()
		return ErrBusy
	}
}

// Run drains the queue until ctx is cancelled. Its caller owns it.
func (s *Service) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case job := <-s.jobs:
			s.execute(ctx, job)
		}
	}
}

func (s *Service) execute(ctx context.Context, job Job) {
	defer func() {
		s.mu.Lock()
		s.running[job.ID] = false
		s.mu.Unlock()
	}()

	report, err := s.exec.Run(ctx, job)
	if err != nil {
		s.log.Error("simulation failed", "simulation", job.ID, "agent", job.Agent, "err", err)
		return
	}
	// Logged rather than stored. A case that could not open a run leaves
	// nothing in the ledger to fold, and the report says so by counting fewer
	// rows than it expected — but the reason is only useful to an operator,
	// and inventing a place to keep it would be a second store of something
	// the ledger already covers for every case that did run.
	for i, c := range report.Cases {
		if c.Error != "" {
			s.log.Warn("simulated case did not settle",
				"simulation", job.ID, "case", i+1, "run", c.RunID, "err", c.Error)
		}
	}
}

// Report folds the simulation's runs back into rows.
//
// Read from the ledger rather than from whatever the executor was holding, so
// a report outlives the process that produced it and reads the same from any
// of them.
func (s *Service) Report(ctx context.Context, id string) (Report, error) {
	ids, err := s.runs.SimulationRuns(ctx, id)
	if err != nil {
		return Report{}, fmt.Errorf("simulate: runs of %s: %w", id, err)
	}

	report := Report{ID: id, Cases: make([]Case, 0, len(ids))}
	for _, runID := range ids {
		steps, err := s.runs.Read(ctx, runID, domain.FirstSeq)
		if err != nil {
			return Report{}, fmt.Errorf("simulate: read %s: %w", runID, err)
		}
		if report.Agent == "" && len(steps) > 0 {
			report.Agent, report.Version = steps[0].AgentID, steps[0].VersionID
		}
		folded, err := Fold(steps)
		if err != nil {
			return Report{}, err
		}
		report.Cases = append(report.Cases, folded)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	report.Running = s.running[id]
	if n, ok := s.expected[id]; ok {
		report.Expected = n
	} else {
		// Nothing in this process started it — a restart, or another node.
		// What ran is what there is, and claiming to expect more would show a
		// finished simulation as permanently incomplete.
		report.Expected = len(report.Cases)
	}
	return report, nil
}
