package ledger

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// SimulationRuns lists the runs one simulation opened, in the order its cases
// were run.
//
// An empty id matches nothing. Real runs carry no simulation, and a query that
// treated "" as "everything unmarked" would report production as a simulation.
func (p *Postgres) SimulationRuns(ctx context.Context, simulation string) ([]domain.RunID, error) {
	if simulation == "" {
		return nil, nil
	}

	rows, err := p.pool.Query(ctx, `
		select run_id from runs
		where simulation = $1
		order by started_at, run_id`, simulation)
	if err != nil {
		return nil, fmt.Errorf("list simulation runs: %w", err)
	}
	defer rows.Close()

	var out []domain.RunID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan simulation run: %w", err)
		}
		out = append(out, domain.RunID(id))
	}
	return out, rows.Err()
}
