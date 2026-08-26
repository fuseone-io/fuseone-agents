package dedupe

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	if pool == nil {
		return nil
	}
	return &Postgres{pool: pool}
}

func (p *Postgres) Lookup(ctx context.Context, key Key, now time.Time) (Record, bool, error) {
	if err := key.Validate(); err != nil {
		return Record{}, false, err
	}
	if err := validateNow(now); err != nil {
		return Record{}, false, err
	}
	if p == nil || p.pool == nil {
		return Record{}, false, nil
	}
	var rec Record
	err := p.pool.QueryRow(ctx, `
		select status, run_id, seq, expires_at
		from tool_effect_dedupe
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and tool_id = $4 and fingerprint = $5 and expires_at > $6`,
		key.Scope.Company, key.Scope.Area, key.AgentID, key.Tool, key.Fingerprint,
		now.UTC()).Scan(&rec.State, &rec.RunID, &rec.Seq, &rec.ExpiresAt)
	switch err {
	case nil:
		return rec, true, nil
	case pgx.ErrNoRows:
		return Record{}, false, nil
	default:
		return Record{}, false, fmt.Errorf("dedupe: lookup: %w", err)
	}
}

func (p *Postgres) Reserve(
	ctx context.Context, key Key, runID domain.RunID, pendingTTL time.Duration, now time.Time,
) (Record, error) {
	if err := key.Validate(); err != nil {
		return Record{}, err
	}
	if err := validateNow(now); err != nil {
		return Record{}, err
	}
	if runID == "" {
		return Record{}, fmt.Errorf("dedupe reservation needs a run")
	}
	if pendingTTL <= 0 {
		return Record{}, fmt.Errorf("pending ttl must be positive")
	}
	if p == nil || p.pool == nil {
		return Record{State: StateReserved, RunID: runID}, nil
	}
	now = now.UTC()
	pendingUntil := now.Add(pendingTTL)

	var (
		reserved bool
		rec      Record
	)
	if err := p.pool.QueryRow(ctx, `
		with upsert as (
			insert into tool_effect_dedupe (
				company_id, area_id, agent_id, tool_id, fingerprint,
				status, run_id, seq, reserved_at, confirmed_at, expires_at, updated_at
			)
			values ($1, $2, $3, $4, $5, 'pending', $6, 0, $7, null, $8, $7)
			on conflict (company_id, area_id, agent_id, tool_id, fingerprint)
			do update set
				status = 'pending',
				run_id = excluded.run_id,
				seq = 0,
				reserved_at = excluded.reserved_at,
				confirmed_at = null,
				expires_at = excluded.expires_at,
				updated_at = excluded.updated_at
			where tool_effect_dedupe.expires_at <= $7
			returning true as reserved, status, run_id, seq, expires_at
		)
		select reserved, status, run_id, seq, expires_at from upsert
		union all
		select false, status, run_id, seq, expires_at
		from tool_effect_dedupe
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and tool_id = $4 and fingerprint = $5
		  and not exists (select 1 from upsert)`,
		key.Scope.Company, key.Scope.Area, key.AgentID, key.Tool, key.Fingerprint,
		runID, now, pendingUntil).Scan(&reserved, &rec.State, &rec.RunID, &rec.Seq, &rec.ExpiresAt); err != nil {
		return Record{}, fmt.Errorf("dedupe: reserve: %w", err)
	}
	if reserved {
		rec.State = StateReserved
	}
	return rec, nil
}

func (p *Postgres) Confirm(
	ctx context.Context, key Key, runID domain.RunID, seq int64, window time.Duration, now time.Time,
) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if err := validateNow(now); err != nil {
		return err
	}
	if runID == "" || seq <= 0 {
		return fmt.Errorf("dedupe confirmation needs a run and step")
	}
	if window <= 0 {
		return fmt.Errorf("dedupe window must be positive")
	}
	if p == nil || p.pool == nil {
		return nil
	}
	now = now.UTC()
	tag, err := p.pool.Exec(ctx, `
		update tool_effect_dedupe set
			status = 'confirmed',
			seq = $7,
			confirmed_at = $8,
			expires_at = $9,
			updated_at = $8
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and tool_id = $4 and fingerprint = $5
		  and run_id = $6 and status = 'pending' and expires_at > $8`,
		key.Scope.Company, key.Scope.Area, key.AgentID, key.Tool, key.Fingerprint,
		runID, seq, now, now.Add(window))
	if err != nil {
		return fmt.Errorf("dedupe: confirm: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrReservationNotHeld
	}
	return nil
}

func (p *Postgres) Release(ctx context.Context, key Key, runID domain.RunID) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("dedupe release needs a run")
	}
	if p == nil || p.pool == nil {
		return nil
	}
	if _, err := p.pool.Exec(ctx, `
		delete from tool_effect_dedupe
		where company_id = $1 and area_id = $2 and agent_id = $3
		  and tool_id = $4 and fingerprint = $5
		  and run_id = $6 and status = 'pending'`,
		key.Scope.Company, key.Scope.Area, key.AgentID, key.Tool, key.Fingerprint, runID); err != nil {
		return fmt.Errorf("dedupe: release: %w", err)
	}
	return nil
}
