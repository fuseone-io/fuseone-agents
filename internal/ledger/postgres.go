package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// appendRetries bounds the optimistic retry on a sequence collision. Two
// workers racing for the same run is a bug the scheduler should prevent, so a
// handful of attempts is generous; more would just mask it for longer.
const appendRetries = 5

// Postgres is the production ledger.
//
// It enforces exactly the invariants ledger.Memory does — a shared contract
// suite runs the same assertions against both — because a fake that is more
// permissive than the real store turns green tests into production incidents.
type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

// Append seals a step against the run's head and updates the projection, both
// in one transaction. The projection can never disagree with the ledger,
// because there is no moment at which only one of them has been written.
func (p *Postgres) Append(ctx context.Context, s domain.Step) (domain.Step, error) {
	var last error
	for attempt := range appendRetries {
		sealed, err := p.appendOnce(ctx, s)
		switch {
		case err == nil:
			return sealed, nil
		case errors.Is(err, ErrSeqConflict):
			// Another writer claimed the sequence. Re-read the head and try
			// again: the chain must stay contiguous, not merely conflict-free.
			last = err
			_ = attempt
			continue
		default:
			return domain.Step{}, err
		}
	}
	return domain.Step{}, fmt.Errorf("append after %d attempts: %w", appendRetries, last)
}

func (p *Postgres) Read(ctx context.Context, runID domain.RunID, fromSeq int64) ([]domain.Step, error) {
	if fromSeq < domain.FirstSeq {
		fromSeq = domain.FirstSeq
	}

	var exists bool
	if err := p.pool.QueryRow(ctx,
		`select exists(select 1 from run_steps where run_id = $1)`, string(runID),
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check run: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	rows, err := p.pool.Query(ctx, `
		select `+stepColumns+`
		from run_steps
		where run_id = $1 and seq >= $2
		order by seq`, string(runID), fromSeq)
	if err != nil {
		return nil, fmt.Errorf("read steps: %w", err)
	}
	defer rows.Close()

	return scanSteps(rows)
}

func (p *Postgres) Head(ctx context.Context, runID domain.RunID) (domain.Step, error) {
	rows, err := p.pool.Query(ctx, `
		select `+stepColumns+`
		from run_steps
		where run_id = $1
		order by seq desc
		limit 1`, string(runID))
	if err != nil {
		return domain.Step{}, fmt.Errorf("read head: %w", err)
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

// Runs answers from the projection: one indexed scan, no folding.
func (p *Postgres) Runs(ctx context.Context) ([]domain.RunID, error) {
	rows, err := p.pool.Query(ctx, `select run_id from runs where not simulated order by started_at desc, run_id`)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	var out []domain.RunID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, domain.RunID(id))
	}
	return out, rows.Err()
}

func (p *Postgres) Verify(ctx context.Context, runID domain.RunID) error {
	steps, err := p.Read(ctx, runID, domain.FirstSeq)
	if err != nil {
		return err
	}
	return domain.VerifyChain(steps)
}
