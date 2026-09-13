package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

func (p *Postgres) Open(ctx context.Context, in OpenInput) (Ticket, bool, error) {
	if err := validateOpen(in); err != nil {
		return Ticket{}, false, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: begin open: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The scope lock makes the count and insert one decision. A ticket lock
	// alone serialises duplicates of one root and lets 201 distinct roots all
	// observe 199 open tickets before any of them inserts.
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 0))`,
		ticketScopeLock(in.Scope)); err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: lock scope: %w", err)
	}
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"ticket:"+string(in.Key)); err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: lock: %w", err)
	}
	return openLocked(ctx, tx, in)
}

func ticketScopeLock(scope domain.Scope) string {
	company, area := string(scope.Company), string(scope.Area)
	return fmt.Sprintf("ticket-scope:%d:%s:%d:%s", len(company), company, len(area), area)
}

func openLocked(ctx context.Context, tx pgx.Tx, in OpenInput) (Ticket, bool, error) {
	if ref, exists, err := eventRef(ctx, tx, in.EventID); err != nil {
		return Ticket{}, false, err
	} else if exists {
		if ref.Key != in.Key {
			return Ticket{}, false, ErrEventTaken
		}
		ticket, err := readTicket(ctx, tx, in.Key, false)
		return ticket, false, err
	}
	if _, err := readTicket(ctx, tx, in.Key, false); err == nil {
		return Ticket{}, false, ErrMoved
	} else if !errors.Is(err, ErrNotFound) {
		return Ticket{}, false, err
	}
	var open int
	if err := tx.QueryRow(ctx, `
		select count(*)
		from governed_tickets t
		join governed_ticket_revisions r
		  on r.ticket_key = t.ticket_key and r.revision = t.current_revision
		where t.company_id = $1 and t.area_id = $2
		  and r.phase not in ('completed', 'rejected', 'cancelled')`,
		string(in.Scope.Company), string(in.Scope.Area)).Scan(&open); err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: count open tickets: %w", err)
	}
	if open >= MaxOpenPerScope {
		return Ticket{}, false, ErrTooManyOpen
	}
	return createTicket(ctx, tx, in)
}

func createTicket(ctx context.Context, tx pgx.Tx, in OpenInput) (Ticket, bool, error) {
	ref := domain.TicketRef{Key: in.Key, Revision: 1}
	if _, err := tx.Exec(ctx, `
		insert into governed_tickets
			(ticket_key, company_id, area_id, agent_id, run_as, requested_by, addressed_by,
			 current_revision, created_at, updated_at,
			 origin_connection, origin_conversation, origin_root)
		values ($1,$2,$3,$4,$5,$6,$7,1,$8,$8,$9,$10,$11)`,
		string(in.Key), string(in.Scope.Company), string(in.Scope.Area),
		string(in.Agent), string(in.RunAs), string(in.RequestedBy), in.AddressedBy, in.At.UTC(),
		in.Origin.Connection, in.Origin.Conversation, in.Origin.Root); err != nil {
		return Ticket{}, false, postgresWriteError("insert ticket", err)
	}
	if err := insertRevision(ctx, tx, Revision{
		Ref: ref, Phase: PhaseCollecting, Draft: in.Draft,
		CreatedAt: in.At.UTC(), UpdatedAt: in.At.UTC(),
	}); err != nil {
		return Ticket{}, false, err
	}
	if err := insertEvent(ctx, tx, in.EventID, ref, in.At); err != nil {
		return Ticket{}, false, err
	}
	ticket, err := readTicket(ctx, tx, in.Key, false)
	if err != nil {
		return Ticket{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, false, fmt.Errorf("ticket: commit open: %w", err)
	}
	return ticket, true, nil
}

func (p *Postgres) Current(ctx context.Context, key domain.TicketKey) (Ticket, error) {
	return readTicket(ctx, p.pool, key, false)
}

func (p *Postgres) AtOrigin(ctx context.Context, origin Origin) (Ticket, error) {
	if !origin.Valid() {
		return Ticket{}, ErrNotFound
	}
	query := `select ` + ticketColumns + `
		from governed_tickets t
		join governed_ticket_revisions c
		  on c.ticket_key = t.ticket_key and c.revision = t.current_revision
		left join governed_ticket_revisions a
		  on a.ticket_key = t.ticket_key and a.revision = t.active_revision
		where t.origin_connection = $1 and t.origin_conversation = $2 and t.origin_root = $3`
	var record ticketRecord
	err := record.scan(p.pool.QueryRow(ctx, query,
		origin.Connection, origin.Conversation, origin.Root))
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, fmt.Errorf("ticket: read origin: %w", err)
	}
	return record.value(), nil
}

func (p *Postgres) Revision(ctx context.Context, ref domain.TicketRef) (Revision, error) {
	return readRevision(ctx, p.pool, ref)
}

func (p *Postgres) EventRevision(
	ctx context.Context, eventID string,
) (Ticket, Revision, error) {
	ref, exists, err := eventRef(ctx, p.pool, eventID)
	if err != nil {
		return Ticket{}, Revision{}, err
	}
	if !exists {
		return Ticket{}, Revision{}, ErrNotFound
	}
	ticket, err := readTicket(ctx, p.pool, ref.Key, false)
	if err != nil {
		return Ticket{}, Revision{}, err
	}
	revision, err := readRevision(ctx, p.pool, ref)
	return ticket, revision, err
}

func (p *Postgres) beginLocked(ctx context.Context, key domain.TicketKey) (pgx.Tx, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("ticket: begin: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, "ticket:"+string(key),
	); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("ticket: lock: %w", err)
	}
	return tx, nil
}

func insertRevision(ctx context.Context, tx pgx.Tx, revision Revision) error {
	approvalRun, approvalSeq, snapshotRef, snapshotDigest := "", int64(0), "", ""
	if revision.Approval != nil {
		approvalRun = string(revision.Approval.RunID)
		approvalSeq = revision.Approval.AtSeq
		snapshotRef = revision.Approval.Snapshot.Ref
		snapshotDigest = revision.Approval.Snapshot.Digest
	}
	outcomeRef, outcomeDigest := "", ""
	if revision.Outcome != nil {
		outcomeRef, outcomeDigest = revision.Outcome.Result.Ref, revision.Outcome.Result.Digest
	}
	_, err := tx.Exec(ctx, `
		insert into governed_ticket_revisions
			(ticket_key, revision, phase, draft_ref, draft_digest,
			 approval_run_id, approval_at_seq, snapshot_ref, snapshot_digest,
			 recipients, outcome_ref, outcome_digest, created_at, updated_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		string(revision.Ref.Key), revision.Ref.Revision, string(revision.Phase),
		revision.Draft.Ref, revision.Draft.Digest,
		approvalRun, approvalSeq, snapshotRef, snapshotDigest,
		recipientStrings(revision.Recipients), outcomeRef, outcomeDigest,
		revision.CreatedAt, revision.UpdatedAt)
	if err != nil {
		return postgresWriteError("insert revision", err)
	}
	return nil
}

func recipientStrings(recipients []domain.UserID) []string {
	out := make([]string, len(recipients))
	for i, recipient := range recipients {
		out[i] = string(recipient)
	}
	return out
}

func insertEvent(
	ctx context.Context, tx pgx.Tx, eventID string, ref domain.TicketRef, at time.Time,
) error {
	_, err := tx.Exec(ctx, `
		insert into governed_ticket_events (event_id, ticket_key, revision, occurred_at)
		values ($1,$2,$3,$4)`, eventID, string(ref.Key), ref.Revision, at.UTC())
	if err != nil {
		return postgresWriteError("insert event", err)
	}
	return nil
}

func eventRef(ctx context.Context, db rowQuery, eventID string) (domain.TicketRef, bool, error) {
	var key string
	var revision int64
	err := db.QueryRow(ctx, `
		select ticket_key, revision from governed_ticket_events where event_id = $1`,
		eventID).Scan(&key, &revision)
	if err == nil {
		return domain.TicketRef{Key: domain.TicketKey(key), Revision: revision}, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TicketRef{}, false, nil
	}
	return domain.TicketRef{}, false, fmt.Errorf("ticket: read event: %w", err)
}

func postgresWriteError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if pgErr.ConstraintName == "governed_ticket_events_pkey" {
			return ErrEventTaken
		}
		return ErrMoved
	}
	return fmt.Errorf("ticket: %s: %w", action, err)
}
