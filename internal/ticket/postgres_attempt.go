package ticket

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/domain"
)

func insertExternalAttempt(ctx context.Context, tx pgx.Tx, attempt ExternalAttempt) error {
	_, err := tx.Exec(ctx, `
		insert into governed_external_attempts
			(idem_key, attempt_kind, ticket_key, revision, run_id, approval_at_seq,
			 call_seq, instance_name, company_id, area_id, contract_digest,
			 snapshot_ref, snapshot_digest, target_id, decided_by,
			 next_check_at, deadline_at, claimed_by, claimed_until, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$20)`,
		attempt.IdemKey, attempt.Kind, string(attempt.Execution.Ref.Key),
		attempt.Execution.Ref.Revision, string(attempt.Execution.RunID),
		attempt.Execution.ApprovalAtSeq, attempt.CallSeq, attempt.Instance,
		string(attempt.Scope.Company), string(attempt.Scope.Area), attempt.ContractDigest,
		attempt.Execution.Snapshot.Ref, attempt.Execution.Snapshot.Digest,
		attempt.TargetID, string(attempt.DecidedBy), attempt.NextCheckAt.UTC(),
		attempt.DeadlineAt.UTC(), attempt.ClaimedBy, attempt.ClaimedUntil.UTC(),
		attempt.CreatedAt.UTC())
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAttemptConflict
	}
	return fmt.Errorf("ticket: insert external attempt: %w", err)
}

func readExternalAttempt(
	ctx context.Context, db rowQuery, idemKey string,
) (ExternalAttempt, error) {
	var out ExternalAttempt
	var ticketKey, runID, company, area, decidedBy string
	err := db.QueryRow(ctx, `
		select idem_key, attempt_kind, ticket_key, revision, run_id, approval_at_seq,
		       call_seq, instance_name, company_id, area_id, contract_digest,
		       snapshot_ref, snapshot_digest, target_id, decided_by,
		       next_check_at, deadline_at, claimed_by, claimed_until, created_at, updated_at
		from governed_external_attempts where idem_key = $1`, idemKey).Scan(
		&out.IdemKey, &out.Kind, &ticketKey, &out.Execution.Ref.Revision,
		&runID, &out.Execution.ApprovalAtSeq, &out.CallSeq, &out.Instance,
		&company, &area, &out.ContractDigest, &out.Execution.Snapshot.Ref,
		&out.Execution.Snapshot.Digest, &out.TargetID, &decidedBy,
		&out.NextCheckAt, &out.DeadlineAt, &out.ClaimedBy, &out.ClaimedUntil,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExternalAttempt{}, ErrNotFound
	}
	if err != nil {
		return ExternalAttempt{}, fmt.Errorf("ticket: read external attempt: %w", err)
	}
	out.Execution.Ref.Key = domain.TicketKey(ticketKey)
	out.Execution.RunID = domain.RunID(runID)
	out.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	out.DecidedBy = domain.UserID(decidedBy)
	out.NextCheckAt = out.NextCheckAt.UTC()
	out.DeadlineAt = out.DeadlineAt.UTC()
	out.ClaimedUntil = out.ClaimedUntil.UTC()
	out.CreatedAt = out.CreatedAt.UTC()
	out.UpdatedAt = out.UpdatedAt.UTC()
	return out, nil
}
