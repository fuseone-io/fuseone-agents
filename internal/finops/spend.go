/*
Package finops projects what each planning call cost.

The ledger stays the source of truth. This reads planning steps forward from a
cursor and writes one row per call, so an aggregate can group by model or agent
without folding an append-only chain — and so a projection that turns out wrong
is rebuilt by reprocessing rather than by rewriting something that must not be
rewritten.
*/
package finops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

type Spend struct{ pool *pgxpool.Pool }

func NewSpend(pool *pgxpool.Pool) *Spend { return &Spend{pool: pool} }

// cursor is where the last pass stopped, in the order the index provides.
type cursor struct {
	at    time.Time
	runID string
	seq   int64
}

/*
Project reads the next planning calls and records what they cost.

One transaction: the cursor is taken under a row lock, the rows are written,
and the cursor moves last. A pass that dies mid-batch leaves the cursor where
it was and repeats the work, which the primary key absorbs — the direction that
repeats rather than the one that skips, because a skipped call is money the
aggregate silently never counted.
*/
func (s *Spend) Project(ctx context.Context, limit int) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("finops: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	written, last, ok, err := projectBatch(ctx, tx, projectionLimit(limit))
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	if err := writeCursor(ctx, tx, last); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("finops: commit: %w", err)
	}
	return written, nil
}

func projectionLimit(limit int) int {
	if limit > 0 {
		return limit
	}
	return 200
}

func projectBatch(ctx context.Context, tx pgx.Tx, limit int) (int, cursor, bool, error) {
	from, err := readCursor(ctx, tx)
	if err != nil {
		return 0, cursor{}, false, err
	}

	steps, err := plannedAfter(ctx, tx, from, limit)
	if err != nil {
		return 0, cursor{}, false, err
	}
	if len(steps) == 0 {
		return 0, cursor{}, false, nil
	}

	written := 0
	for _, step := range steps {
		ok, err := writeSpend(ctx, tx, step)
		if err != nil {
			return 0, cursor{}, false, err
		}
		if ok {
			written++
		}
	}

	// Over everything read, not everything written: a step skipped for naming
	// no model is still a step this pass has seen, and leaving it behind the
	// cursor would make every future pass read it again for ever.
	last := steps[len(steps)-1]
	return written, cursor{at: last.at, runID: last.runID, seq: last.seq}, true, nil
}

type plannedStep struct {
	at        time.Time
	runID     string
	seq       int64
	agentID   string
	companyID string
	areaID    string
	simulated bool
	cost      domain.Cost
	payload   []byte
}

func plannedAfter(ctx context.Context, tx pgx.Tx, from cursor, limit int) ([]plannedStep, error) {
	rows, err := tx.Query(ctx, `
		select s.at, s.run_id, s.seq, s.agent_id, s.company_id, s.area_id,
		       r.simulated, s.cost_micros, s.input_tokens, s.output_tokens,
		       s.cache_read_tokens, s.cache_write_tokens, s.payload
		from run_steps s
		join runs r on r.run_id = s.run_id
		where s.kind = 'planned'
		  and (s.at, s.run_id, s.seq) > ($1, $2, $3)
		order by s.at, s.run_id, s.seq
		limit $4`, from.at, from.runID, from.seq, limit)
	if err != nil {
		return nil, fmt.Errorf("finops: read planning steps: %w", err)
	}
	defer rows.Close()

	var out []plannedStep
	for rows.Next() {
		var s plannedStep
		if err := rows.Scan(&s.at, &s.runID, &s.seq, &s.agentID, &s.companyID, &s.areaID,
			&s.simulated, &s.cost.Micros, &s.cost.InputTokens, &s.cost.OutputTokens,
			&s.cost.CacheReadTokens, &s.cost.CacheWriteTokens, &s.payload); err != nil {
			return nil, fmt.Errorf("finops: scan planning step: %w", err)
		}
		s.at = s.at.UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

/*
writeSpend records one call, and reports whether it recorded anything.

A step that names no model is skipped rather than projected or fatal. Steps
written before the pair existed carry none, and attributing them would put
spend against a model the step never named; failing on them would stop the
sweep at the oldest row in the installation and never reach today's.
*/
func writeSpend(ctx context.Context, tx pgx.Tx, s plannedStep) (bool, error) {
	if s.simulated {
		return false, nil
	}
	var p domain.PlannedPayload
	if err := json.Unmarshal(s.payload, &p); err != nil {
		// Unreadable rather than absent, and equally not this sweep's to fix.
		return false, nil
	}
	// Both halves or neither. The projection aggregates by the pair, and a row
	// with a model under an empty provider is a financial fact with half a
	// subject — it would group into a bucket nobody can act on.
	if p.Provider == "" || p.Model == "" {
		return false, nil
	}

	status := ""
	if p.Price != nil {
		status = string(p.Price.Status)
	}
	_, err := tx.Exec(ctx, `
		insert into planning_spend (
			run_id, seq, provider, model, agent_id, company_id, area_id, day,
			cost_micros, input_tokens, output_tokens,
			cache_read_tokens, cache_write_tokens, price_status)
		values ($1,$2,$3,$4,$5,$6,$7,($8 at time zone 'UTC')::date,$9,$10,$11,$12,$13,$14)
		on conflict (run_id, seq) do nothing`,
		s.runID, s.seq, p.Provider, p.Model, s.agentID, s.companyID, s.areaID, s.at,
		s.cost.Micros, s.cost.InputTokens, s.cost.OutputTokens,
		s.cost.CacheReadTokens, s.cost.CacheWriteTokens, status)
	if err != nil {
		return false, fmt.Errorf("finops: record planning spend: %w", err)
	}
	return true, nil
}

func readCursor(ctx context.Context, tx pgx.Tx) (cursor, error) {
	var c cursor
	err := tx.QueryRow(ctx, `
		select scanned_at, scanned_run_id, scanned_seq
		from planning_spend_cursor
		where id = true
		for update`).Scan(&c.at, &c.runID, &c.seq)
	if err == nil {
		c.at = c.at.UTC()
		return c, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return cursor{}, fmt.Errorf("finops: read spend cursor: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		insert into planning_spend_cursor (id, scanned_at, started_at) values (true, $1, $1)
		on conflict (id) do nothing`, now); err != nil {
		return cursor{}, fmt.Errorf("finops: initialise spend cursor: %w", err)
	}
	return cursor{at: now}, nil
}

func writeCursor(ctx context.Context, tx pgx.Tx, c cursor) error {
	if _, err := tx.Exec(ctx, `
		update planning_spend_cursor
		   set scanned_at = $1, scanned_run_id = $2, scanned_seq = $3
		 where id = true`, c.at.UTC(), c.runID, c.seq); err != nil {
		return fmt.Errorf("finops: advance spend cursor: %w", err)
	}
	return nil
}
