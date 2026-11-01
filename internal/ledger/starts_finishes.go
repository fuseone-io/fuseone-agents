package ledger

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
StartsAndFinishes is, for a batch of runs, only each run's opening and
closing steps, in sequence order.

What the event dispatcher needs and nothing more: what opened a finished run
and the chain it carries, and what it published when it closed (#203). The
dispatcher sweeps every few seconds on every replica; reading each run whole
to find two steps would grow with how long its runs are. Runs that do not
exist are absent from the answer, not an error: the caller decides what a
missing start means.
*/
func (p *Postgres) StartsAndFinishes(
	ctx context.Context, runs []domain.RunID,
) (map[domain.RunID][]domain.Step, error) {
	out := map[domain.RunID][]domain.Step{}
	if len(runs) == 0 {
		return out, nil
	}
	ids := make([]string, len(runs))
	for i, id := range runs {
		ids[i] = string(id)
	}
	rows, err := p.pool.Query(ctx, `
		select `+stepColumns+`
		from run_steps
		where run_id = any($1) and kind in ('run_started', 'run_finished')
		order by run_id, seq`, ids)
	if err != nil {
		return nil, fmt.Errorf("read starts and finishes: %w", err)
	}
	defer rows.Close()

	steps, err := scanSteps(rows)
	if err != nil {
		return nil, err
	}
	for _, s := range steps {
		out[s.RunID] = append(out[s.RunID], s)
	}
	return out, nil
}

// StartsAndFinishes holds the Postgres contract: only the asked runs that
// exist, only their opening and closing steps, in sequence order.
func (m *Memory) StartsAndFinishes(
	ctx context.Context, runs []domain.RunID,
) (map[domain.RunID][]domain.Step, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := map[domain.RunID][]domain.Step{}
	for _, id := range runs {
		for _, s := range m.runs[id] {
			if s.Kind == domain.StepRunStarted || s.Kind == domain.StepRunFinished {
				out[id] = append(out[id], s)
			}
		}
	}
	return out, nil
}
