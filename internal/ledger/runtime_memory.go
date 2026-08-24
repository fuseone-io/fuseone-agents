package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/mcpmetrics"
)

func (m *Memory) RuntimeHealth(ctx context.Context, filter domain.RunFilter) (domain.RuntimeHealth, error) {
	if err := ctx.Err(); err != nil {
		return domain.RuntimeHealth{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	now := m.now()
	since := runtimeWindowSince(filter.Since, now)
	current := filter
	current.Since = time.Time{}
	current.Until = time.Time{}
	current.After = nil
	current.Search = ""

	out := domain.RuntimeHealth{ByPhase: map[string]int64{}}
	failures := map[string]domain.RuntimeFailureBucket{}
	toolFailures := map[string]domain.RuntimeToolFailureBucket{}
	toolFailureRuns := map[string]map[domain.RunID]bool{}
	for id, steps := range m.runs {
		if len(steps) == 0 || !matches(steps[0], current) || isSimulated(steps) {
			continue
		}
		collectRuntimeToolFailures(steps, since, toolFailures, toolFailureRuns)
		summary := summarise(steps)
		if runtimeActivePhase(summary.Phase) ||
			(runtimeTerminalPhase(summary.Phase) && !summary.UpdatedAt.Before(since)) {
			out.ByPhase[summary.Phase]++
		}
		st := m.leases[id]
		if claimable(summary.Phase) {
			switch {
			case !st.leasedUntil.IsZero() && st.leasedUntil.After(now):
				out.Queue.Leased++
			case st.nextAttemptAt.After(now):
				out.Queue.BackingOff++
			default:
				out.Queue.Ready++
				readyAt := st.nextAttemptAt
				if readyAt.IsZero() {
					readyAt = summary.UpdatedAt
				}
				if out.Queue.OldestReadyAt.IsZero() || readyAt.Before(out.Queue.OldestReadyAt) {
					out.Queue.OldestReadyAt = readyAt
				}
				if !st.leasedUntil.IsZero() && !st.leasedUntil.After(now) {
					out.Queue.ExpiredLeases++
				}
			}
		}
		failure := summary.Failure
		failureAt := summary.UpdatedAt
		if failure == nil {
			if current, ok := m.lastFailure[id]; ok {
				failure = &current
				failureAt = m.lastFailureAt[id]
			}
		}
		if failure == nil || failureAt.Before(since) {
			continue
		}
		key := runtimeFailureKey(*failure)
		bucket := failures[key]
		bucket.Code = failure.Code
		bucket.Provider = failure.Provider
		bucket.Status = failure.Status
		bucket.Retryable = failure.Retryable
		bucket.Runs++
		if failureAt.After(bucket.LastAt) {
			bucket.LastAt = failureAt
		}
		failures[key] = bucket
	}

	out.Failures = make([]domain.RuntimeFailureBucket, 0, len(failures))
	for _, one := range failures {
		out.Failures = append(out.Failures, one)
	}
	slices.SortFunc(out.Failures, func(a, b domain.RuntimeFailureBucket) int {
		if a.Runs != b.Runs {
			return int(b.Runs - a.Runs)
		}
		if c := b.LastAt.Compare(a.LastAt); c != 0 {
			return c
		}
		return strings.Compare(a.Code, b.Code)
	})
	out.ToolFailures = make([]domain.RuntimeToolFailureBucket, 0, len(toolFailures))
	for code, one := range toolFailures {
		one.Runs = int64(len(toolFailureRuns[code]))
		out.ToolFailures = append(out.ToolFailures, one)
	}
	slices.SortFunc(out.ToolFailures, func(a, b domain.RuntimeToolFailureBucket) int {
		if a.Calls != b.Calls {
			return int(b.Calls - a.Calls)
		}
		if c := b.LastAt.Compare(a.LastAt); c != 0 {
			return c
		}
		return strings.Compare(a.Code, b.Code)
	})
	return out, nil
}

func collectRuntimeToolFailures(
	steps []domain.Step, since time.Time,
	failures map[string]domain.RuntimeToolFailureBucket,
	runs map[string]map[domain.RunID]bool,
) {
	for _, st := range steps {
		if st.Kind != domain.StepToolReturned || st.At.Before(since) {
			continue
		}
		var returned domain.ToolReturnedPayload
		if err := json.Unmarshal(st.Payload, &returned); err != nil || !returned.Failed {
			continue
		}
		code := returned.ErrorCode
		if code == "" {
			code = mcpmetrics.CodeNoCode
		}
		code = mcpmetrics.Code(code)
		bucket := failures[code]
		bucket.Code = code
		bucket.Calls++
		if st.At.After(bucket.LastAt) {
			bucket.LastAt = st.At
		}
		failures[code] = bucket
		if runs[code] == nil {
			runs[code] = map[domain.RunID]bool{}
		}
		runs[code][st.RunID] = true
	}
}

func runtimeActivePhase(phase string) bool {
	switch phase {
	case "running", "awaiting_tool", "awaiting_approval", "parked", "compensating":
		return true
	default:
		return false
	}
}

func runtimeTerminalPhase(phase string) bool {
	return phase == "finished" || phase == "failed"
}

func runtimeFailureKey(f domain.FailureSummary) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s", f.Code, f.Provider, f.Status, boolKey(f.Retryable))
}

func boolKey(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
