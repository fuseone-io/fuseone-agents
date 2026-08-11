package ledger

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// Stats aggregates over every run held in memory.
//
// It reads the same projection the Postgres query does — phase from
// projectPhase, the opening and closing instants from the steps that carry
// them — so a figure the console shows cannot differ between the two.
func (m *Memory) Stats(ctx context.Context, filter domain.RunFilter) (domain.RunStats, error) {
	if err := ctx.Err(); err != nil {
		return domain.RunStats{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	out := domain.RunStats{ByPhase: map[string]int64{}}
	var durations []int64

	for _, steps := range m.runs {
		if len(steps) == 0 || !matches(steps[0], filter) {
			continue
		}

		out.Total++
		out.ByPhase[phaseOf(steps)]++

		if started, ended, ok := span(steps); ok {
			durations = append(durations, ended.Sub(started).Milliseconds())
		}
	}

	out.Ended = int64(len(durations))
	out.MedianDurationMS = median(durations)
	return out, nil
}

func matches(first domain.Step, f domain.RunFilter) bool {
	switch {
	case f.Scope.Company != "" && first.Scope.Company != f.Scope.Company:
		return false
	case f.Scope.Area != "" && first.Scope.Area != f.Scope.Area:
		return false
	case f.AgentID != "" && first.AgentID != f.AgentID:
		return false
	case !f.Since.IsZero() && first.At.Before(f.Since):
		return false
	case f.Search != "" && !matchesSearch(first, f.Search):
		return false
	}
	return true
}

// matchesSearch mirrors the SQL side's ILIKE over the same two identifiers.
func matchesSearch(first domain.Step, q string) bool {
	q = strings.ToLower(q)
	return strings.Contains(strings.ToLower(string(first.RunID)), q) ||
		strings.Contains(strings.ToLower(string(first.AgentID)), q)
}

// span returns when a run opened and closed. A run still in flight has no
// duration to report — measuring it against now would put a moving number in a
// column of settled ones.
func span(steps []domain.Step) (started, ended time.Time, ok bool) {
	for _, s := range steps {
		switch s.Kind {
		case domain.StepRunStarted:
			started = s.At
		case domain.StepRunFinished:
			ended = s.At
		}
	}
	return started, ended, !started.IsZero() && !ended.IsZero()
}

// median takes the lower of the two middle values on an even count, which is
// what percentile_cont does not do — so the SQL side interpolates and this one
// must match it.
func median(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)

	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}
