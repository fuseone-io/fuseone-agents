package ledger

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// ListRuns builds each summary from the run's own steps.
//
// The in-memory ledger has no materialised projection to read, so it derives
// the same figures the SQL side stores. Deriving them here rather than keeping
// a second copy is what makes the contract suite meaningful: if the two ever
// disagreed, this is where it would show.
func (m *Memory) ListRuns(ctx context.Context, filter domain.RunFilter, phase string, limit int) ([]domain.RunSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []domain.RunSummary
	for _, steps := range m.runs {
		if len(steps) == 0 || !matches(steps[0], filter) {
			continue
		}
		summary := summarise(steps)
		if phase != "" && summary.Phase != phase {
			continue
		}
		if !filter.Until.IsZero() && summary.StartedAt.After(filter.Until) {
			continue
		}
		out = append(out, summary)
	}

	// Newest first, then by id, so a page is stable when two runs share an
	// instant — which they do, because a test can open several in one tick.
	slices.SortFunc(out, func(a, b domain.RunSummary) int {
		if c := b.StartedAt.Compare(a.StartedAt); c != 0 {
			return c
		}
		return strings.Compare(string(b.RunID), string(a.RunID))
	})

	if n := limitOrDefault(limit); len(out) > n {
		out = out[:n]
	}
	return out, nil
}

func (m *Memory) CostRollup(ctx context.Context, filter domain.RunFilter, groupBy string) ([]domain.CostBucket, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if filter.Until.IsZero() {
		return nil, fmt.Errorf("ledger: a cost rollup needs an upper bound")
	}
	if _, err := rollupColumn(groupBy); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	totals := map[string]domain.CostBucket{}
	for _, steps := range m.runs {
		if len(steps) == 0 || !matches(steps[0], filter) {
			continue
		}
		summary := summarise(steps)
		if summary.StartedAt.After(filter.Until) {
			continue
		}

		key := rollupKey(summary, groupBy)
		bucket := totals[key]
		bucket.Key = key
		bucket.Runs++
		bucket.Cost = bucket.Cost.Add(summary.Cost)
		totals[key] = bucket
	}

	out := make([]domain.CostBucket, 0, len(totals))
	for _, b := range totals {
		out = append(out, b)
	}
	slices.SortFunc(out, func(a, b domain.CostBucket) int { return strings.Compare(a.Key, b.Key) })
	return out, nil
}

func rollupKey(s domain.RunSummary, groupBy string) string {
	switch groupBy {
	case "area":
		return string(s.Scope.Area)
	case "company":
		return string(s.Scope.Company)
	case "day":
		return s.StartedAt.UTC().Format("2006-01-02")
	default:
		return string(s.AgentID)
	}
}

// summarise folds a run into the shape the projection stores. It mirrors
// upsertRun's arithmetic, which is the pairing the contract suite checks.
func summarise(steps []domain.Step) domain.RunSummary {
	who := identityOf(steps)
	s := domain.RunSummary{
		RunID: steps[0].RunID, Scope: who.Scope, AgentID: who.AgentID,
		VersionID: who.VersionID, OnBehalfOf: who.OnBehalfOf,
		Phase: phaseOf(steps), StartedAt: steps[0].At,
	}

	var labels domain.Labels
	for _, step := range steps {
		s.Seq = step.Seq
		s.UpdatedAt = step.At
		s.Cost = s.Cost.Add(step.Cost)
		s.ReservedMicros += reservationDelta(step)
		s.ToolCalls += toolCallDelta(step)
		labels = labels.Union(step.Labels)

		// The pending approval is set by the request and cleared by anything
		// that follows: a decided run must not stay in the inbox for ever.
		if step.Kind == domain.StepApprovalRequested {
			var p domain.ApprovalRequestedPayload
			decodePayload(step, &p)
			s.PendingApproval = &domain.PendingApprovalSummary{
				Tool: p.Tool, Rule: p.Rule, Reason: p.Reason, AtSeq: step.Seq,
			}
		} else if step.Kind != domain.StepRunStarted {
			s.PendingApproval = nil
		}

		if step.Kind == domain.StepRunFinished {
			s.EndedAt = step.At
		}
	}
	s.Labels = labels
	return s
}
