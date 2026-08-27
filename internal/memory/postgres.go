package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

func (p *Postgres) Find(ctx context.Context, q domain.MemoryQuery) ([]domain.MemoryAssertion, error) {
	q.Now = nowOrWall(q.Now)
	where, args, searchOrder := findWhere(q)
	args = append(args, domain.MemoryFindLimit(q.Limit))
	order := "confirmed desc, observations desc, updated_at desc, assertion_id"
	if searchOrder != "" {
		order = searchOrder + " desc, " + order
	}
	rows, err := p.pool.Query(ctx, `
		select `+columns+`
		from memory_assertions `+where+`
		order by `+order+`
		limit $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("memory: find: %w", err)
	}
	return scan(rows, q.Now)
}

func (p *Postgres) List(ctx context.Context, f Filter) ([]domain.MemoryAssertion, error) {
	f.Now = nowOrWall(f.Now)
	where, args := listWhere(f)
	args = append(args, domain.MemoryListLimit(f.Limit))
	rows, err := p.pool.Query(ctx, `
		select `+columns+`
		from memory_assertions `+where+`
		order by updated_at desc, assertion_id
		limit $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("memory: list: %w", err)
	}
	return scan(rows, f.Now)
}

func (p *Postgres) Assert(
	ctx context.Context, a domain.MemoryAssertion, by domain.UserID, reason string, now time.Time,
) (domain.MemoryAssertion, error) {
	prepared, err := prepareAssertion(a, by, now)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: begin assert: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	stored, _, err := mergeInto(ctx, tx, prepared, OriginHuman, by, reason, "asserted")
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: commit assert: %w", err)
	}
	return stored, nil
}

func (p *Postgres) Disable(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin disable: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	a, err := readAssertionTx(ctx, tx, id, scope)
	if err != nil {
		return err
	}
	a.Status, a.UpdatedBy, a.UpdatedAt = domain.MemoryDisabled, by, now.UTC()
	if err := recordEvent(ctx, tx, a, by, reason, "disabled"); err != nil {
		return err
	}
	if err := disableAssertion(ctx, tx, id, by, now); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("memory: commit disable: %w", err)
	}
	return nil
}

func (p *Postgres) Suggest(
	ctx context.Context, s domain.MemorySuggestion, policy domain.MemoryLearningPolicy,
	by domain.UserID, now time.Time,
) (domain.MemorySuggestionOutcome, error) {
	policy = policy.Normalize()
	if !policy.Enabled() {
		return domain.MemorySuggestionOutcome{Result: domain.MemorySuggestIgnored}, nil
	}
	prepared, err := prepareSuggestion(s, by, now)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: begin suggest: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	flow := suggestionTx{tx: tx, by: by, now: now}

	// Suggestion first, then identity, and the duplicate check after both. The
	// order is the one every other path takes — auto-confirm reaches the
	// identity from inside this transaction — and asking whether memory already
	// covers this before holding the identity would be asking about rows another
	// writer is in the middle of changing.
	if err := lockSuggestion(ctx, tx, prepared.ID); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	identity := identitiesForSuggestion(prepared)[0]
	if err := lockIdentity(ctx, tx, identity); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if err := keyLegacyIdentities(ctx, tx, identity); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if out, done, err := flow.alreadyActive(ctx, prepared); done || err != nil {
		return out, err
	}
	stored, err := upsertSuggestion(ctx, tx, prepared)
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if stored.Status != domain.MemorySuggestionPending {
		return flow.ignored(ctx, stored)
	}
	return flow.finish(ctx, stored, policy)
}

type suggestionTx struct {
	tx  pgx.Tx
	by  domain.UserID
	now time.Time
}

func (s suggestionTx) alreadyActive(
	ctx context.Context,
	prepared domain.MemorySuggestion,
) (domain.MemorySuggestionOutcome, bool, error) {
	var active *domain.MemoryAssertion
	for _, identity := range identitiesForSuggestion(prepared) {
		found, err := byIdentityTx(ctx, s.tx, identity)
		if err != nil {
			return domain.MemorySuggestionOutcome{}, false, err
		}
		if found != nil && found.Status == domain.MemoryActive && !expired(*found, s.now) {
			active = found
			break
		}
	}
	if active == nil {
		return domain.MemorySuggestionOutcome{}, false, nil
	}
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, true,
			fmt.Errorf("memory: commit already active suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{
		Suggestion: prepared, Assertion: active, Result: domain.MemorySuggestAlreadyActive,
	}, true, nil
}

func (s suggestionTx) ignored(
	ctx context.Context, stored domain.MemorySuggestion,
) (domain.MemorySuggestionOutcome, error) {
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit ignored suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{Suggestion: stored, Result: domain.MemorySuggestIgnored}, nil
}

func (s suggestionTx) recordSuggested(ctx context.Context, stored domain.MemorySuggestion) error {
	return recordEvent(ctx, s.tx, assertionFromSuggestion(stored, 0, s.by, s.now),
		s.by, "suggested by agent", "suggested")
}

func (s suggestionTx) finish(
	ctx context.Context,
	stored domain.MemorySuggestion,
	policy domain.MemoryLearningPolicy,
) (domain.MemorySuggestionOutcome, error) {
	if err := s.recordSuggested(ctx, stored); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if policy.AutoConfirms(stored.Labels) && stored.Observations >= policy.MinObservations {
		return s.autoConfirm(ctx, stored)
	}
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{Suggestion: stored, Result: domain.MemorySuggestPending}, nil
}

func (s suggestionTx) autoConfirm(
	ctx context.Context, stored domain.MemorySuggestion,
) (domain.MemorySuggestionOutcome, error) {
	assertion := assertionFromSuggestion(stored, stored.Observations, domain.UserID("system:memory"), s.now)
	merged, outcome, err := mergeInto(ctx, s.tx, assertion, OriginAutoConfirm,
		assertion.UpdatedBy, "auto-confirmed repeated suggestions", "auto_confirmed")
	if err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if outcome == Covered {
		if err := closeAsCovered(ctx, s.tx, stored, merged, assertion.UpdatedBy, s.now); err != nil {
			return domain.MemorySuggestionOutcome{}, err
		}
		if err := s.tx.Commit(ctx); err != nil {
			return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit suggestion: %w", err)
		}
		covered := merged
		return domain.MemorySuggestionOutcome{
			Suggestion: stored, Assertion: &covered, Result: domain.MemorySuggestAlreadyActive,
		}, nil
	}
	assertion = merged
	stored.Status = domain.MemorySuggestionAutoConfirmed
	stored.UpdatedBy, stored.UpdatedAt = assertion.UpdatedBy, assertion.UpdatedAt
	if err := updateSuggestion(ctx, s.tx, stored); err != nil {
		return domain.MemorySuggestionOutcome{}, err
	}
	if err := s.tx.Commit(ctx); err != nil {
		return domain.MemorySuggestionOutcome{}, fmt.Errorf("memory: commit auto-confirm suggestion: %w", err)
	}
	return domain.MemorySuggestionOutcome{
		Suggestion: stored, Assertion: &assertion, Result: domain.MemorySuggestAutoConfirmed,
	}, nil
}

func (p *Postgres) ListSuggestions(ctx context.Context, f SuggestionFilter) ([]domain.MemorySuggestion, error) {
	where, args := suggestionWhere(f)
	args = append(args, domain.MemorySuggestLimit(f.Limit))
	rows, err := p.pool.Query(ctx, `
		select `+suggestionColumns+`
		from memory_suggestions `+where+`
		order by updated_at desc, suggestion_id
		limit $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("memory: list suggestions: %w", err)
	}
	return scanSuggestions(rows)
}

func (p *Postgres) AcceptSuggestion(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) (domain.MemoryAssertion, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: begin accept suggestion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	s, err := readSuggestionTx(ctx, tx, id, scope, true)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if s.Status != domain.MemorySuggestionPending {
		return domain.MemoryAssertion{}, ErrNotFound
	}
	assertion := assertionFromSuggestion(s, s.Observations, by, now)
	stored, outcome, err := mergeInto(ctx, tx, assertion, OriginAccept, by, reason, "accepted")
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if outcome == Covered {
		// Shared memory already answers this and was not modified. The proposal
		// is finished all the same: leaving it pending would be a queue item
		// with no honest exit, and dismissing it would record a refusal nobody
		// made about a fact the platform already holds.
		if err := closeAsCovered(ctx, tx, s, stored, by, now); err != nil {
			return domain.MemoryAssertion{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.MemoryAssertion{}, fmt.Errorf("memory: commit accept suggestion: %w", err)
		}
		return stored, nil
	}
	// After the merge, in the same transaction: a suggestion marked accepted
	// beside an assertion that was never written is a queue that empties while
	// nothing is learned.
	s.Status, s.UpdatedBy, s.UpdatedAt = domain.MemorySuggestionAccepted, by, now.UTC()
	if err := updateSuggestion(ctx, tx, s); err != nil {
		return domain.MemoryAssertion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: commit accept suggestion: %w", err)
	}
	return stored, nil
}

func (p *Postgres) DismissSuggestion(
	ctx context.Context, id string, scope domain.Scope, by domain.UserID, reason string, now time.Time,
) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("memory: begin dismiss suggestion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	s, err := readSuggestionTx(ctx, tx, id, scope, true)
	if err != nil {
		return err
	}
	if s.Status != domain.MemorySuggestionPending {
		return ErrNotFound
	}
	s.Status, s.UpdatedBy, s.UpdatedAt = domain.MemorySuggestionDismissed, by, now.UTC()
	if err := recordEvent(ctx, tx, assertionFromSuggestion(s, 0, by, now), by, reason, "dismissed"); err != nil {
		return err
	}
	if err := updateSuggestion(ctx, tx, s); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("memory: commit dismiss suggestion: %w", err)
	}
	return nil
}

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

func scan(rows pgx.Rows, now time.Time) ([]domain.MemoryAssertion, error) {
	defer rows.Close()
	var out []domain.MemoryAssertion
	for rows.Next() {
		a, err := scanAssertion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, effectiveStatus(a, now))
	}
	return out, rows.Err()
}

func scanSuggestions(rows pgx.Rows) ([]domain.MemorySuggestion, error) {
	defer rows.Close()
	var out []domain.MemorySuggestion
	for rows.Next() {
		s, err := scanSuggestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanAssertion(row scanner) (domain.MemoryAssertion, error) {
	var a domain.MemoryAssertion
	var company, area, agent, status, createdBy, updatedBy string
	var evidence []byte
	var labels []string
	err := row.Scan(
		&a.ID, &company, &area, &agent, &a.Kind, &a.Subject, &a.Signature,
		&a.Claim, &evidence, &a.Observations, &a.Confirmed, &labels,
		&status, &a.ExpiresAt, &createdBy, &a.CreatedAt, &updatedBy, &a.UpdatedAt)
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	a.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	a.AgentID = domain.AgentID(agent)
	a.Status = domain.MemoryStatus(status)
	a.CreatedBy, a.UpdatedBy = domain.UserID(createdBy), domain.UserID(updatedBy)
	if err := json.Unmarshal(evidence, &a.Evidence); err != nil {
		return domain.MemoryAssertion{}, fmt.Errorf("memory: decode evidence: %w", err)
	}
	a.Labels = domain.NewLabels(labels...)
	return a, nil
}

func scanSuggestion(row scanner) (domain.MemorySuggestion, error) {
	var s domain.MemorySuggestion
	var company, area, agent, status, createdBy, updatedBy string
	var coveredBy *string
	var evidence []byte
	var labels []string
	err := row.Scan(
		&s.ID, &s.AssertionID, &company, &area, &agent, &s.Kind, &s.Subject,
		&s.Signature, &s.Claim, &evidence, &s.Observations, &labels,
		&status, &s.ExpiresAt, &createdBy, &s.CreatedAt, &updatedBy, &s.UpdatedAt,
		&coveredBy)
	if err != nil {
		return domain.MemorySuggestion{}, err
	}
	s.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
	s.AgentID = domain.AgentID(agent)
	s.Status = domain.MemorySuggestionStatus(status)
	if coveredBy != nil {
		s.CoveredBy = *coveredBy
	}
	s.CreatedBy, s.UpdatedBy = domain.UserID(createdBy), domain.UserID(updatedBy)
	if err := json.Unmarshal(evidence, &s.Evidence); err != nil {
		return domain.MemorySuggestion{}, fmt.Errorf("memory: decode suggestion evidence: %w", err)
	}
	s.Labels = domain.NewLabels(labels...)
	return s, nil
}

func listWhere(f Filter) (string, []any) {
	var clauses []string
	var args []any
	clauses = append(clauses, scopedClause(f.Scopes, &args))
	if f.AgentID != "" {
		args = append(args, string(f.AgentID))
		clauses = append(clauses, "(agent_id = $"+fmt.Sprint(len(args))+" or agent_id = '')")
	}
	if f.Status.Valid() {
		clauses = append(clauses, statusClause(f.Status, f.Now, &args))
	}
	clauses = appendSearchClauses(clauses, &args, f.Search)
	return "where " + strings.Join(clauses, " and "), args
}

func suggestionWhere(f SuggestionFilter) (string, []any) {
	var clauses []string
	var args []any
	clauses = append(clauses, scopedClause(f.Scopes, &args))
	if f.AgentID != "" {
		args = append(args, string(f.AgentID))
		clauses = append(clauses, "(agent_id = $"+fmt.Sprint(len(args))+" or agent_id = '')")
	}
	if f.Status.Valid() {
		args = append(args, string(f.Status))
		clauses = append(clauses, "status = $"+fmt.Sprint(len(args)))
	}
	clauses = appendSearchClauses(clauses, &args, f.Search)
	return "where " + strings.Join(clauses, " and "), args
}

func findWhere(q domain.MemoryQuery) (string, []any, string) {
	var clauses []string
	var args []any
	args = append(args, string(q.Scope.Company))
	clauses = append(clauses, "company_id = $"+fmt.Sprint(len(args)))
	args = append(args, string(q.Scope.Area))
	clauses = append(clauses, "area_id = $"+fmt.Sprint(len(args)))
	args = append(args, string(q.AgentID))
	clauses = append(clauses, "(agent_id = $"+fmt.Sprint(len(args))+" or agent_id = '')")
	if kind := strings.TrimSpace(q.Kind); kind != "" {
		args = append(args, kind)
		clauses = append(clauses, "kind = $"+fmt.Sprint(len(args)))
	}
	if subject := strings.TrimSpace(q.Subject); subject != "" {
		args = append(args, subject)
		clauses = append(clauses, "subject = $"+fmt.Sprint(len(args)))
	}
	if signature := strings.TrimSpace(q.Signature); signature != "" {
		args = append(args, signature)
		clauses = append(clauses, "signature = $"+fmt.Sprint(len(args)))
	}
	var searchOrder string
	clauses, searchOrder = appendFindSearchClause(clauses, &args, q.Search)
	args = append(args, q.Now.UTC())
	clauses = append(clauses, "status = 'active'")
	clauses = append(clauses, "(expires_at is null or expires_at > $"+fmt.Sprint(len(args))+")")
	return "where " + strings.Join(clauses, " and "), args, searchOrder
}

func appendSearchClauses(clauses []string, args *[]any, search string) []string {
	parsed := parseSearchTerms(search)
	if !parsed.hasInput {
		return clauses
	}
	if len(parsed.terms) == 0 {
		return append(clauses, "false")
	}
	for _, term := range parsed.terms {
		*args = append(*args, searchPattern(term))
		n := fmt.Sprint(len(*args))
		clauses = append(clauses,
			searchTermCondition(n, term))
	}
	return clauses
}

func appendFindSearchClause(clauses []string, args *[]any, search string) ([]string, string) {
	parsed := parseSearchTerms(search)
	terms := parsed.terms
	if !parsed.hasInput {
		return clauses, ""
	}
	if len(terms) == 0 {
		return append(clauses, "false"), ""
	}
	conds := make([]string, 0, len(terms))
	scoreParts := make([]string, 0, len(terms))
	strongConds := []string{}
	for _, term := range terms {
		*args = append(*args, searchPattern(term))
		cond := searchTermCondition(fmt.Sprint(len(*args)), term)
		conds = append(conds, cond)
		scoreParts = append(scoreParts,
			fmt.Sprintf("(case when %s then %d else 0 end)", cond, searchTermWeight(term)))
		if strongSearchTerm(term) {
			strongConds = append(strongConds, cond)
		}
	}
	// The broad OR is deliberately separate from the match-count predicate:
	// PostgreSQL can turn this plain boolean shape into BitmapOr over the
	// trigram indexes, then apply the stricter count as a filter.
	clauses = append(clauses, "("+strings.Join(conds, " or ")+")")
	if len(strongConds) > 0 {
		clauses = append(clauses, "("+strings.Join(strongConds, " or ")+")")
	}
	clauses = append(clauses, searchTermsMatchedClause(conds, minFindSearchMatches(len(terms))))
	score := strings.Join(scoreParts, " + ")
	return clauses, score
}

func searchTermsMatchedClause(conds []string, required int) string {
	if required <= 1 || len(conds) == 1 {
		return "(" + strings.Join(conds, " or ") + ")"
	}
	if required >= len(conds) {
		return "(" + strings.Join(conds, " and ") + ")"
	}
	var pairs []string
	for i := range conds {
		for j := i + 1; j < len(conds); j++ {
			pairs = append(pairs, "("+conds[i]+" and "+conds[j]+")")
		}
	}
	return "(" + strings.Join(pairs, " or ") + ")"
}

func searchTermCondition(n string, term string) string {
	if shortSearchTerm(term) {
		return "(subject ~* $" + n + " or " +
			"signature ~* $" + n + " or " +
			"claim ~* $" + n + ")"
	}
	return "(subject ilike $" + n + " escape '\\' or " +
		"signature ilike $" + n + " escape '\\' or " +
		"claim ilike $" + n + " escape '\\')"
}

func searchPattern(term string) string {
	if shortSearchTerm(term) {
		return searchRegexPattern(term)
	}
	return likePattern(term)
}

func searchRegexPattern(term string) string {
	return "(^|[^[:alnum:]])" + regexp.QuoteMeta(term) + "([^[:alnum:]]|$)"
}

func likePattern(term string) string {
	var b strings.Builder
	b.Grow(len(term) + 2)
	b.WriteByte('%')
	for _, r := range term {
		if r == '%' || r == '_' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

func statusClause(status domain.MemoryStatus, now time.Time, args *[]any) string {
	switch status {
	case domain.MemoryActive:
		*args = append(*args, now.UTC())
		return "(status = 'active' and (expires_at is null or expires_at > $" + fmt.Sprint(len(*args)) + "))"
	case domain.MemoryExpired:
		*args = append(*args, now.UTC())
		return "(status = 'expired' or (status = 'active' and expires_at <= $" + fmt.Sprint(len(*args)) + "))"
	default:
		*args = append(*args, string(status))
		return "status = $" + fmt.Sprint(len(*args))
	}
}

func scopedClause(scopes []domain.Scope, args *[]any) string {
	if len(scopes) == 0 {
		return "false"
	}
	var parts []string
	for _, scope := range scopes {
		parts = append(parts, scopeClause(scope, args))
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

func scopeClause(scope domain.Scope, args *[]any) string {
	if scope.Company == domain.Installation && scope.Area == "" {
		return "true"
	}
	*args = append(*args, string(scope.Company))
	company := len(*args)
	if scope.Area == "" {
		return fmt.Sprintf("(company_id = $%d)", company)
	}
	*args = append(*args, string(scope.Area))
	return fmt.Sprintf("(company_id = $%d and area_id = $%d)", company, len(*args))
}
