package egress

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/egressmetrics"
)

// Denial is what a stdio MCP process tried through the managed egress proxy.
//
// It deliberately names only the proxy instance, host, port and stable cause.
// URL paths, query strings, headers, bodies and tool arguments are not here.
type Denial struct {
	Server    string
	Host      string
	Port      int
	Code      string
	Attempts  int64
	FirstSeen time.Time
	LastSeen  time.Time
}

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	if pool == nil {
		return nil
	}
	return &Postgres{pool: pool}
}

func (p *Postgres) RecordDenial(ctx context.Context, denial Denial) error {
	return p.RecordDenials(ctx, []Denial{denial})
}

func (p *Postgres) RecordDenials(ctx context.Context, denials []Denial) error {
	if p == nil || p.pool == nil {
		return nil
	}
	if len(denials) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("egress: begin stdio denial record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, denial := range denials {
		denial = normalizeDenial(denial)
		if _, err := tx.Exec(ctx, `
		insert into mcp_egress_denials (server, host, port, code, attempts, first_seen, last_seen)
		values ($1, $2, $3, $4, $5, $6, $7)
		on conflict (server, host, port, code)
		do update set
			attempts = mcp_egress_denials.attempts + excluded.attempts,
			first_seen = least(mcp_egress_denials.first_seen, excluded.first_seen),
			last_seen = greatest(mcp_egress_denials.last_seen, excluded.last_seen)`,
			denial.Server, denial.Host, denial.Port, egressmetrics.Code(denial.Code),
			denial.Attempts, denial.FirstSeen.UTC(), denial.LastSeen.UTC()); err != nil {
			return fmt.Errorf("egress: record stdio denial: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("egress: commit stdio denial record: %w", err)
	}
	return nil
}

func normalizeDenial(denial Denial) Denial {
	if denial.Attempts < 1 {
		denial.Attempts = 1
	}
	if denial.FirstSeen.IsZero() {
		denial.FirstSeen = time.Now()
	}
	if denial.LastSeen.IsZero() {
		denial.LastSeen = denial.FirstSeen
	}
	return denial
}
