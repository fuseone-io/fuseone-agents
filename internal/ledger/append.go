package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/domain"
)

/*
Sealing one step into the chain.

The whole of it is one transaction: the head is read, the step is hashed onto
it, and the run’s projection moves — because a chain with a gap and a
projection that disagrees with the trail are the two ways this record stops
being worth keeping (PRD AU-01, AU-02).
*/
func (p *Postgres) appendOnce(ctx context.Context, s domain.Step) (domain.Step, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.Step{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialise appends to this run, and only this run.
	//
	// Locking the head row is not enough: it does not exist before the first
	// step, and a waiter that blocks on it still resolves its `order by seq
	// desc limit 1` against the pre-lock snapshot, so it computes the same
	// next sequence the winner just used. Under real contention that turns
	// single-writer into a retry storm. An advisory lock covers the run as a
	// whole, exists before the first row, and is released by the commit.
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, string(s.RunID),
	); err != nil {
		return domain.Step{}, fmt.Errorf("lock run: %w", err)
	}

	prev, err := headTx(ctx, tx, s.RunID)
	if err != nil {
		return domain.Step{}, err
	}

	sealed, err := domain.NewStep(prev, s)
	if err != nil {
		return domain.Step{}, err
	}

	if err := insertStep(ctx, tx, sealed); err != nil {
		return domain.Step{}, err
	}
	if err := upsertRun(ctx, tx, sealed); err != nil {
		return domain.Step{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Step{}, fmt.Errorf("commit: %w", err)
	}
	return sealed, nil
}

// headTx reads the run's last step and locks it, serialising concurrent
// appends to the same run. The primary key remains the real guarantee: an
// empty run has no row to lock, so two writers can still collide on seq 1.
func headTx(ctx context.Context, tx pgx.Tx, runID domain.RunID) (*domain.Step, error) {
	rows, err := tx.Query(ctx, `
		select `+stepColumns+`
		from run_steps
		where run_id = $1
		order by seq desc
		limit 1
		for update`, string(runID))
	if err != nil {
		return nil, fmt.Errorf("read head: %w", err)
	}
	defer rows.Close()

	steps, err := scanSteps(rows)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, nil
	}
	return &steps[0], nil
}

func insertStep(ctx context.Context, tx pgx.Tx, s domain.Step) error {
	_, err := tx.Exec(ctx, `
		insert into run_steps (
			run_id, seq, kind, company_id, area_id, agent_id, version_id,
			on_behalf_of, payload, labels,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			cost_micros, idem_key, policy_hash, at, prev_hash, hash
		) values (
			$1,$2,$3,$4,$5,$6,$7,$8,coalesce($9,'{}'::jsonb),$10,
			$11,$12,$13,$14,$15,$16,$17,$18,$19,$20
		)`,
		string(s.RunID), s.Seq, string(s.Kind),
		string(s.Scope.Company), string(s.Scope.Area),
		string(s.AgentID), string(s.VersionID), string(s.OnBehalfOf),
		nilIfEmpty(s.Payload), labelsOrEmpty(s.Labels),
		s.Cost.InputTokens, s.Cost.OutputTokens,
		s.Cost.CacheReadTokens, s.Cost.CacheWriteTokens, s.Cost.Micros,
		s.IdemKey, s.PolicyHash, s.At, s.PrevHash, s.Hash,
	)
	if err == nil {
		return nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "run_steps_idem_key_uniq":
			return ErrIdemConflict
		default:
			return ErrSeqConflict
		}
	}
	return fmt.Errorf("insert step: %w", err)
}

// upsertRun folds the new step into the projection.
//
// The arithmetic mirrors engine.State.Apply. Keeping it in SQL rather than
// reading the row, folding in Go and writing it back avoids a read-modify-write
// race, at the cost of the two having to be changed together — which is why
// the contract suite checks the projection against a Go fold.
func upsertRun(ctx context.Context, tx pgx.Tx, s domain.Step) error {
	phase, endedAt := projectPhase(s)

	// Read from the step rather than passed beside it. The ledger is the
	// record; a projection that learned this from somewhere else could
	// disagree with the thing it projects.
	simulated, simulation := false, ""
	if s.Kind == domain.StepRunStarted {
		var started domain.RunStartedPayload
		if err := json.Unmarshal(s.Payload, &started); err == nil {
			simulated, simulation = started.Simulated, started.Simulation
		}
	}

	_, err := tx.Exec(ctx, `
		insert into runs (
			run_id, company_id, area_id, agent_id, version_id, on_behalf_of,
			phase, last_seq, cost_micros, total_tokens, reserved_micros, tool_calls,
			labels, pending_tool, pending_rule, pending_reason, pending_at_seq,
			started_at, ended_at, updated_at,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			simulated, simulation
		) values (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$18,
			$22,$23,$24,$25,$26,$27
		)
		on conflict (run_id) do update set
			agent_id        = case when runs.agent_id = '' then excluded.agent_id else runs.agent_id end,
			version_id      = case when runs.version_id = '' then excluded.version_id else runs.version_id end,
			on_behalf_of    = case when runs.on_behalf_of = '' then excluded.on_behalf_of else runs.on_behalf_of end,
			phase           = coalesce($20, runs.phase),
			last_seq        = excluded.last_seq,
			cost_micros     = runs.cost_micros + excluded.cost_micros,
			total_tokens    = runs.total_tokens + excluded.total_tokens,
			input_tokens       = runs.input_tokens + excluded.input_tokens,
			output_tokens      = runs.output_tokens + excluded.output_tokens,
			cache_read_tokens  = runs.cache_read_tokens + excluded.cache_read_tokens,
			cache_write_tokens = runs.cache_write_tokens + excluded.cache_write_tokens,
			reserved_micros = runs.reserved_micros + $21,
			tool_calls      = runs.tool_calls + excluded.tool_calls,
			labels          = (
				select coalesce(array_agg(distinct l order by l), '{}')
				from unnest(runs.labels || excluded.labels) as l
			),
			pending_tool    = $14,
			pending_rule    = $15,
			pending_reason  = $16,
			pending_at_seq  = $17,
			ended_at        = coalesce(excluded.ended_at, runs.ended_at),
			updated_at      = excluded.updated_at`,
		string(s.RunID), string(s.Scope.Company), string(s.Scope.Area),
		string(s.AgentID), string(s.VersionID), string(s.OnBehalfOf),
		phaseOrRunning(phase), s.Seq,
		s.Cost.Micros, s.Cost.TotalTokens(), reservationDelta(s), toolCallDelta(s),
		labelsOrEmpty(s.Labels),
		pendingTool(s), pendingRule(s), pendingReason(s), pendingAtSeq(s),
		s.At, endedAt,
		phase, reservationDelta(s),
		s.Cost.InputTokens, s.Cost.OutputTokens, s.Cost.CacheReadTokens, s.Cost.CacheWriteTokens,
		simulated, simulation,
	)
	if err != nil {
		return fmt.Errorf("upsert run projection: %w", err)
	}
	return nil
}

// labelsOrEmpty keeps a nil label set out of a not-null column. pgx encodes a
// nil slice as SQL NULL, and "no labels" is an empty set, not an unknown one.
func labelsOrEmpty(l domain.Labels) []string {
	if len(l) == 0 {
		return []string{}
	}
	return []string(l)
}

func nilIfEmpty(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

func projectPhase(s domain.Step) (phase *string, endedAt *time.Time) {
	set := func(v string) *string { return &v }
	switch s.Kind {
	case domain.StepRunStarted:
		return set("running"), nil
	case domain.StepToolCalled:
		return set("awaiting_tool"), nil
	case domain.StepToolReturned, domain.StepApprovalDecided:
		return set("running"), nil
	case domain.StepApprovalRequested:
		return set("awaiting_approval"), nil
	case domain.StepParked:
		return set("parked"), nil
	case domain.StepResumed:
		return set("running"), nil
	case domain.StepAbandoned:
		// A person ended the run. Whether it is over depends on whether
		// anything still has to be taken back first.
		var p domain.AbandonedPayload
		if err := json.Unmarshal(s.Payload, &p); err == nil && p.Compensate {
			return set("compensating"), nil
		}
		at := s.At
		return set("failed"), &at
	case domain.StepFailed:
		at := s.At
		return set("failed"), &at
	case domain.StepRunFinished:
		at := s.At
		return set("finished"), &at
	}
	return nil, nil
}

func phaseOrRunning(p *string) string {
	if p == nil {
		return "running"
	}
	return *p
}
