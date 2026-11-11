package ledger

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Opening is a run's first step alone.

Every turn of every pool reads it before resolving anything (NT-016): a judge run must never
be planned as the agent it judges. Read reads the whole run, and the runner already does
that twice a turn; this reads one row by the primary key, however long the run has grown.
*/
func (p *Postgres) Opening(ctx context.Context, runID domain.RunID) (domain.Step, error) {
	rows, err := p.pool.Query(ctx, `
		select `+stepColumns+`
		from run_steps
		where run_id = $1 and seq = $2`, string(runID), domain.FirstSeq)
	if err != nil {
		return domain.Step{}, fmt.Errorf("read opening: %w", err)
	}
	defer rows.Close()

	steps, err := scanSteps(rows)
	if err != nil {
		return domain.Step{}, err
	}
	if len(steps) == 0 {
		return domain.Step{}, ErrNotFound
	}
	return steps[0], nil
}

// Opening answers as the Postgres store does: the first step, or ErrNotFound.
func (m *Memory) Opening(ctx context.Context, runID domain.RunID) (domain.Step, error) {
	if err := ctx.Err(); err != nil {
		return domain.Step{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	steps := m.runs[runID]
	if len(steps) == 0 {
		return domain.Step{}, ErrNotFound
	}
	return steps[0], nil
}
