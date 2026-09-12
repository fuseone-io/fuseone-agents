package connectortools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

type PostgresGraviteeAttempts struct {
	pool *pgxpool.Pool
}

func NewPostgresGraviteeAttempts(pool *pgxpool.Pool) *PostgresGraviteeAttempts {
	return &PostgresGraviteeAttempts{pool: pool}
}

func (p *PostgresGraviteeAttempts) Get(
	ctx context.Context, idemKey string,
) (GraviteeAttempt, error) {
	if p == nil || p.pool == nil {
		return GraviteeAttempt{}, ErrGraviteeAttemptNotFound
	}
	attempt, err := scanGraviteeAttempt(p.pool.QueryRow(ctx, graviteeAttemptSelect+`
		where attempt_kind = $1 and idem_key = $2`, graviteeAttemptKind, idemKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return GraviteeAttempt{}, ErrGraviteeAttemptNotFound
	}
	if err != nil {
		return GraviteeAttempt{}, fmt.Errorf("connector: read Gravitee attempt: %w", err)
	}
	return attempt, nil
}

func (p *PostgresGraviteeAttempts) ClaimDue(
	ctx context.Context, owner string, now time.Time, lease time.Duration, limit int,
) ([]GraviteeAttempt, error) {
	if p == nil || p.pool == nil || owner == "" || lease <= 0 || limit <= 0 || limit > 100 {
		return nil, errors.New("connector: invalid Gravitee attempt claim")
	}
	rows, err := p.pool.Query(ctx, `
		with due as (
			select idem_key as claimed_id from governed_external_attempts
			where attempt_kind = $5 and not settled and next_check_at <= $2
			  and (claimed_until is null or claimed_until <= $2)
			order by next_check_at, created_at, idem_key
			for update skip locked
			limit $4
		)
		update governed_external_attempts a
		set claimed_by = $1, claimed_until = $3, updated_at = $2
		from due where a.idem_key = due.claimed_id
		returning `+graviteeAttemptColumns,
		owner, now.UTC(), now.Add(lease).UTC(), limit, graviteeAttemptKind)
	if err != nil {
		return nil, fmt.Errorf("connector: claim Gravitee attempts: %w", err)
	}
	defer rows.Close()
	var out []GraviteeAttempt
	for rows.Next() {
		attempt, err := scanGraviteeAttempt(rows)
		if err != nil {
			return nil, fmt.Errorf("connector: scan Gravitee attempt: %w", err)
		}
		out = append(out, attempt)
	}
	return out, rows.Err()
}

func (p *PostgresGraviteeAttempts) Arm(
	ctx context.Context, idemKey, claimedBy string, nextCheckAt, at time.Time,
) (GraviteeAttempt, error) {
	if p == nil || p.pool == nil || idemKey == "" || nextCheckAt.IsZero() || at.IsZero() {
		return GraviteeAttempt{}, ErrGraviteeAttemptNotFound
	}
	row := p.pool.QueryRow(ctx, `
		update governed_external_attempts
		set status = 'pending', next_check_at = $3,
		    claimed_by = '', claimed_until = null, updated_at = $4
		where idem_key = $1 and attempt_kind = $5 and status = 'prepared' and not settled
		  and (($2 <> '' and claimed_by = $2) or
		       ($2 = '' and (claimed_by = '' or claimed_until <= $4)))
		returning `+graviteeAttemptColumns,
		idemKey, claimedBy, nextCheckAt.UTC(), at.UTC(), graviteeAttemptKind)
	attempt, err := scanGraviteeAttempt(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return GraviteeAttempt{}, ErrGraviteeAttemptClaim
	}
	if err != nil {
		return GraviteeAttempt{}, fmt.Errorf("connector: arm Gravitee attempt: %w", err)
	}
	return attempt, nil
}

func (p *PostgresGraviteeAttempts) Resolve(
	ctx context.Context, in GraviteeAttemptResolution,
) (GraviteeAttempt, error) {
	if p == nil || p.pool == nil {
		return GraviteeAttempt{}, ErrGraviteeAttemptNotFound
	}
	if err := validateGraviteeResolution(in); err != nil {
		return GraviteeAttempt{}, err
	}
	var next any
	if in.Status == GraviteeAttemptPrepared || in.Status == GraviteeAttemptPending ||
		in.Status == GraviteeAttemptManual {
		next = in.NextCheckAt.UTC()
	} else if in.Status == GraviteeAttemptConfirmed || in.Status == GraviteeAttemptTerminal {
		// Final remote knowledge remains due until its normal return or a late
		// reconciliation is sealed in the immutable run record.
		next = in.NextCheckAt.UTC()
	}
	row := p.pool.QueryRow(ctx, `
		update governed_external_attempts
		set status = $3, result_ref = $4, result_digest = $5, outcome_code = $6,
		    checks = checks + 1, next_check_at = $7,
		    claimed_by = '', claimed_until = null, updated_at = $8
		where idem_key = $1 and attempt_kind = $9 and not settled
		  and (status in ('prepared', 'pending', 'manual') or status = $3)
		  and (($2 <> '' and claimed_by = $2) or
		       ($2 = '' and (claimed_by = '' or claimed_until <= $8)))
		returning `+graviteeAttemptColumns,
		in.IdemKey, in.ClaimedBy, string(in.Status), in.Result.Ref, in.Result.Digest,
		in.OutcomeCode, next, in.At.UTC(), graviteeAttemptKind)
	attempt, err := scanGraviteeAttempt(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return GraviteeAttempt{}, ErrGraviteeAttemptClaim
	}
	if err != nil {
		return GraviteeAttempt{}, fmt.Errorf("connector: resolve Gravitee attempt: %w", err)
	}
	return attempt, nil
}

func (p *PostgresGraviteeAttempts) Settle(
	ctx context.Context, idemKey string, at time.Time,
) error {
	if p == nil || p.pool == nil || idemKey == "" || at.IsZero() {
		return ErrGraviteeAttemptNotFound
	}
	tag, err := p.pool.Exec(ctx, `
		update governed_external_attempts
		set settled = true, next_check_at = null,
		    claimed_by = '', claimed_until = null, updated_at = $2
		where idem_key = $1 and attempt_kind = $3
		  and status in ('confirmed', 'terminal')`, idemKey, at.UTC(), graviteeAttemptKind)
	if err != nil {
		return fmt.Errorf("connector: settle Gravitee attempt: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrGraviteeAttemptNotFound
	}
	return nil
}

const graviteeAttemptColumns = `
	attempt_kind, idem_key, ticket_key, revision, run_id, approval_at_seq, call_seq,
	instance_name, company_id, area_id, contract_digest,
	snapshot_ref, snapshot_digest, target_id, decided_by,
	status, result_ref, result_digest, outcome_code, checks,
	next_check_at, deadline_at, claimed_by, claimed_until, settled,
	created_at, updated_at`

const graviteeAttemptSelect = `select ` + graviteeAttemptColumns + `
	from governed_external_attempts`

type graviteeAttemptScanner interface {
	Scan(...any) error
}

func scanGraviteeAttempt(row graviteeAttemptScanner) (GraviteeAttempt, error) {
	var out GraviteeAttempt
	var kind, ticketKey, runID, company, area, decidedBy, status string
	var next, claimedUntil *time.Time
	err := row.Scan(
		&kind, &out.IdemKey, &ticketKey, &out.Execution.Ref.Revision, &runID,
		&out.Execution.ApprovalAtSeq, &out.CallSeq, &out.Instance, &company, &area,
		&out.ContractDigest, &out.Execution.Snapshot.Ref, &out.Execution.Snapshot.Digest,
		&out.SubscriptionID, &decidedBy, &status, &out.Result.Ref, &out.Result.Digest,
		&out.OutcomeCode, &out.Checks, &next, &out.DeadlineAt, &out.ClaimedBy,
		&claimedUntil, &out.Settled, &out.CreatedAt, &out.UpdatedAt,
	)
	out.Execution.Ref.Key = domain.TicketKey(ticketKey)
	out.Execution.RunID = domain.RunID(runID)
	out.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	out.DecidedBy = domain.UserID(decidedBy)
	out.Status = GraviteeAttemptStatus(status)
	if err == nil && kind != graviteeAttemptKind {
		return GraviteeAttempt{}, ErrGraviteeAttemptNotFound
	}
	if next != nil {
		out.NextCheckAt = next.UTC()
	}
	if claimedUntil != nil {
		out.ClaimedUntil = claimedUntil.UTC()
	}
	out.DeadlineAt = out.DeadlineAt.UTC()
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, err
}
