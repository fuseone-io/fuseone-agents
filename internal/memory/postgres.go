package memory

import (
	"context"
	"fmt"
	"time"

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
	stored, outcome, err := mergeInto(ctx, tx, prepared, OriginHuman, by, reason, "asserted")
	if err != nil {
		return domain.MemoryAssertion{}, err
	}
	if outcome == Covered {
		return domain.MemoryAssertion{}, coveredBy(stored)
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
