package ledger

import (
	"context"
	"math"
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
	out.MedianDurationMS = percentileDisc(durations, 0.5)
	out.P95DurationMS = percentileDisc(durations, 0.95)
	return out, nil
}

// Throughput buckets runs by the hour they started, mirroring the SQL side's
// date_trunc. Phase comes from the same projection the tallies use, so an hour
// here and a total there cannot disagree.
func (m *Memory) Throughput(ctx context.Context, filter domain.RunFilter) ([]domain.ThroughputBucket, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	byHour := map[time.Time]*domain.ThroughputBucket{}
	for _, steps := range m.runs {
		if len(steps) == 0 || !matches(steps[0], filter) {
			continue
		}
		at := steps[0].At.UTC().Truncate(time.Hour)
		bucket, ok := byHour[at]
		if !ok {
			bucket = &domain.ThroughputBucket{At: at, ByPhase: map[string]int64{}}
			byHour[at] = bucket
		}
		bucket.ByPhase[phaseOf(steps)]++
		bucket.Total++
	}

	out := make([]domain.ThroughputBucket, 0, len(byHour))
	for _, bucket := range byHour {
		out = append(out, *bucket)
	}
	// Map iteration is unordered and a chart drawn from it would reshuffle
	// between renders.
	slices.SortFunc(out, func(a, b domain.ThroughputBucket) int { return a.At.Compare(b.At) })
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
	case !f.Until.IsZero() && !first.At.Before(f.Until):
		return false
	case f.Search != "" && !matchesSearch(first, f.Search):
		return false
	case len(f.Scopes) > 0 && !withinAny(first.Scope, f.Scopes):
		return false
	}
	return true
}

// withinAny reports whether any of the caller's scopes reaches this run's.
func withinAny(scope domain.Scope, allowed []domain.Scope) bool {
	for _, a := range allowed {
		if a.Contains(scope) {
			return true
		}
	}
	return false
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

// percentileDisc mirrors Postgres percentile_disc: it returns a value some run
// actually had, never an interpolation between two runs that never existed.
//
// This used to average the two middle values on an even count while the SQL
// side took the lower one, so the same runs produced different medians
// depending on which store answered — the exact divergence the contract suite
// exists to catch, and it did not, because every fixture had an odd count.
func percentileDisc(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)

	i := int(math.Ceil(p*float64(len(values)))) - 1
	return values[min(max(i, 0), len(values)-1)]
}
