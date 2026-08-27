package memory

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

// Reading and writing the rows, and nothing about when to.
//
// Every statement here is narrow on purpose: what a caller may change is
// decided where the decision is made, and a helper that wrote more than its
// name says would make that decision somewhere nobody is looking.

const columns = `assertion_id, company_id, area_id, agent_id, kind, subject,
	signature, claim, evidence, observations, confirmed, labels, status,
	expires_at, created_by, created_at, updated_by, updated_at`

const suggestionColumns = `suggestion_id, assertion_id, company_id, area_id,
	agent_id, kind, subject, signature, claim, evidence, observations, labels,
	status, expires_at, created_by, created_at, updated_by, updated_at,
	covered_by`

type db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func upsertAssertion(ctx context.Context, tx db, a domain.MemoryAssertion) error {
	evidence, _ := json.Marshal(a.Evidence)
	_, err := tx.Exec(ctx, `
		insert into memory_assertions (`+columns+`, canonical_identity_key)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		on conflict (assertion_id) do update set
			canonical_identity_key = excluded.canonical_identity_key,
			claim = excluded.claim, evidence = excluded.evidence,
			observations = excluded.observations, confirmed = excluded.confirmed,
			labels = excluded.labels, status = excluded.status,
			expires_at = excluded.expires_at, updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`,
		a.ID, string(a.Scope.Company), string(a.Scope.Area), string(a.AgentID),
		a.Kind, a.Subject, a.Signature, a.Claim, evidence, a.Observations,
		a.Confirmed, []string(a.Labels), string(a.Status), a.ExpiresAt,
		string(a.CreatedBy), a.CreatedAt, string(a.UpdatedBy), a.UpdatedAt,
		domain.CanonicalIdentityKey(a))
	if err != nil {
		return fmt.Errorf("memory: project assertion %s: %w", a.ID, err)
	}
	return nil
}

func upsertSuggestion(ctx context.Context, tx db, next domain.MemorySuggestion) (domain.MemorySuggestion, error) {
	old, err := readSuggestionByIDTx(ctx, tx, next.ID, true)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := insertSuggestion(ctx, tx, next); err != nil {
			return domain.MemorySuggestion{}, err
		}
		return next, nil
	}
	if err != nil {
		return domain.MemorySuggestion{}, err
	}
	merged := mergeSuggestion(old, next)
	if err := updateSuggestion(ctx, tx, merged); err != nil {
		return domain.MemorySuggestion{}, err
	}
	return merged, nil
}

func lockSuggestion(ctx context.Context, tx db, id string) error {
	_, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1, 0))`, id)
	if err != nil {
		return fmt.Errorf("memory: lock suggestion %s: %w", id, err)
	}
	return nil
}

func insertSuggestion(ctx context.Context, tx db, s domain.MemorySuggestion) error {
	evidence, _ := json.Marshal(s.Evidence)
	_, err := tx.Exec(ctx, `
		insert into memory_suggestions (`+suggestionColumns+`)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
		        nullif($19, ''))`,
		s.ID, s.AssertionID, string(s.Scope.Company), string(s.Scope.Area),
		string(s.AgentID), s.Kind, s.Subject, s.Signature, s.Claim, evidence,
		s.Observations, []string(s.Labels), string(s.Status), s.ExpiresAt,
		string(s.CreatedBy), s.CreatedAt, string(s.UpdatedBy), s.UpdatedAt,
		s.CoveredBy)
	if err != nil {
		return fmt.Errorf("memory: insert suggestion %s: %w", s.ID, err)
	}
	return nil
}

/*
closeAsCovered ends a suggestion that memory already there answered.

Only the suggestion changes. The assertion that covered it is not touched, gets
no event and keeps its updated_at: it was not modified, and recording that it
was would put a mutation in the trail that never happened.
*/
func closeAsCovered(
	ctx context.Context, tx db, s domain.MemorySuggestion,
	by domain.MemoryAssertion, actor domain.UserID, now time.Time,
) error {
	s.Status, s.CoveredBy = domain.MemorySuggestionCovered, by.ID
	s.UpdatedBy, s.UpdatedAt = actor, now.UTC()
	return updateSuggestion(ctx, tx, s)
}

func updateSuggestion(ctx context.Context, tx db, s domain.MemorySuggestion) error {
	evidence, _ := json.Marshal(s.Evidence)
	_, err := tx.Exec(ctx, `
		update memory_suggestions
		set claim = $2, evidence = $3, observations = $4, labels = $5,
		    status = $6, expires_at = $7, updated_by = $8, updated_at = $9,
		    covered_by = nullif($10, '')
		where suggestion_id = $1`,
		s.ID, s.Claim, evidence, s.Observations, []string(s.Labels),
		string(s.Status), s.ExpiresAt, string(s.UpdatedBy), s.UpdatedAt, s.CoveredBy)
	if err != nil {
		return fmt.Errorf("memory: update suggestion %s: %w", s.ID, err)
	}
	return nil
}

func disableAssertion(ctx context.Context, tx db, id string, by domain.UserID, now time.Time) error {
	_, err := tx.Exec(ctx, `
		update memory_assertions
		set status = 'disabled', updated_by = $2, updated_at = $3
		where assertion_id = $1`, id, string(by), now.UTC())
	if err != nil {
		return fmt.Errorf("memory: disable %s: %w", id, err)
	}
	return nil
}

/*
recordSuggestionEnded writes the terminal event of a proposal.

Its own function because the detail is the suggestion and not an assertion. The
other suggestion events describe the assertion the proposal would become, which
is the right shape while it might still become one — but a proposal the platform
ended never will, and an assertion-shaped detail would say active about
something terminal.

The same shape the administrative erasure writes for the same act, so the two
routes into this state leave the trail one story rather than two.
*/
func recordSuggestionEnded(ctx context.Context, tx db, s domain.MemorySuggestion) error {
	detail, _ := json.Marshal(s)
	_, err := tx.Exec(ctx, `
		insert into memory_assertion_events
			(assertion_id, action, company_id, area_id, agent_id, principal_id, reason, detail, at)
		values ($1,'source_erased',$2,$3,$4,$5,$6,$7,$8)`,
		s.AssertionID, string(s.Scope.Company), string(s.Scope.Area), string(s.AgentID),
		string(systemMemory), "the source the evidence names is no longer there",
		detail, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("memory: record suggestion source_erased: %w", err)
	}
	return nil
}

func recordEvent(ctx context.Context, tx db, a domain.MemoryAssertion, by domain.UserID, reason, action string) error {
	detail, _ := json.Marshal(a)
	_, err := tx.Exec(ctx, `
		insert into memory_assertion_events
			(assertion_id, action, company_id, area_id, agent_id, principal_id, reason, detail, at)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		a.ID, action, string(a.Scope.Company), string(a.Scope.Area), string(a.AgentID),
		string(by), clean(reason), detail, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("memory: record %s: %w", action, err)
	}
	return nil
}

func readAssertionTx(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, scope domain.Scope) (domain.MemoryAssertion, error) {
	a, err := scanAssertion(tx.QueryRow(ctx, `select `+columns+` from memory_assertions where assertion_id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	if err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: read %s: %w", id, err)
	}
	if !scope.Contains(a.Scope) {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	return a, nil
}

func readSuggestionTx(
	ctx context.Context,
	tx interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	id string,
	scope domain.Scope,
	lock bool,
) (domain.MemorySuggestion, error) {
	s, err := readSuggestionByIDTx(ctx, tx, id, lock)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.MemorySuggestion{}, ErrNotFound
	}
	if err != nil {
		return domain.MemorySuggestion{}, fmt.Errorf("memory: read suggestion %s: %w", id, err)
	}
	if !scope.Contains(s.Scope) {
		return domain.MemorySuggestion{}, ErrNotFound
	}
	return s, nil
}

func readSuggestionByIDTx(
	ctx context.Context,
	tx interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	id string,
	lock bool,
) (domain.MemorySuggestion, error) {
	sql := `select ` + suggestionColumns + ` from memory_suggestions where suggestion_id = $1`
	if lock {
		sql += ` for update`
	}
	return scanSuggestion(tx.QueryRow(ctx, sql, id))
}
