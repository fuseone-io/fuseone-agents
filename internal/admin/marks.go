package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

/*
Which budget warning each scope has already had (PRD FO-05).

Kept in the database rather than in the sweeper because the sweeper is a
goroutine in a process that restarts: an in-memory mark would announce the same
crossing again after every deploy, and a warning that repeats is a warning
people filter.

One row per scope. The threshold and the period it belongs to are the value, so
a new month starts the warnings again by comparison rather than by a job that
has to remember to clear anything.
*/

const markKind = "budget_mark"

type Marks struct{ pool *pgxpool.Pool }

func NewMarks(pool *pgxpool.Pool) *Marks { return &Marks{pool: pool} }

func (m *Marks) Announced(ctx context.Context) (map[string]domain.BudgetMark, error) {
	rows, err := m.pool.Query(ctx, `
		select name, value from settings where kind = $1`, markKind)
	if err != nil {
		return nil, fmt.Errorf("admin: read budget marks: %w", err)
	}
	defer rows.Close()

	out := map[string]domain.BudgetMark{}
	for rows.Next() {
		var name string
		var raw []byte
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, err
		}
		var stored storedMark
		if err := json.Unmarshal(raw, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode budget mark %s: %w", name, err)
		}
		out[name] = stored.mark()
	}
	return out, rows.Err()
}

// Announce records the crossing and writes it to the administrative trail.
//
// Both, in one transaction. The row is what stops it being said twice; the
// trail entry is what somebody reads three weeks later when they ask when the
// area started running out of money.
func (m *Marks) Announce(ctx context.Context, mark domain.BudgetMark) error {
	value, err := json.Marshal(storedMark{
		Threshold: mark.Threshold, Since: mark.Since,
		SpentMicros: mark.SpentMicros, CeilingMicros: mark.CeilingMicros,
		Company: string(mark.Scope.Company), Area: string(mark.Scope.Area),
	})
	if err != nil {
		return fmt.Errorf("admin: encode budget mark: %w", err)
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by, updated_at)
		values ('installation', '', '', $1, $2, $3, true, '', now())
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			value = excluded.value, updated_at = now()`,
		markKind, mark.Key(), value); err != nil {
		return fmt.Errorf("admin: write budget mark %s: %w", mark.Key(), err)
	}

	// No principal: nobody did this. The platform noticed, which is a
	// different kind of entry and reads as one.
	if err := Record(ctx, tx, Event{
		Scope: mark.Scope, Action: "budget.threshold", Target: mark.Key(),
		Detail: json.RawMessage(value),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type storedMark struct {
	Threshold     int       `json:"threshold"`
	Since         time.Time `json:"since"`
	SpentMicros   int64     `json:"spent_micros"`
	CeilingMicros int64     `json:"ceiling_micros"`
	Company       string    `json:"company,omitempty"`
	Area          string    `json:"area,omitempty"`
}

func (s storedMark) mark() domain.BudgetMark {
	return domain.BudgetMark{
		Scope: domain.Scope{
			Company: domain.CompanyID(s.Company), Area: domain.AreaID(s.Area),
		},
		Threshold: s.Threshold, Since: s.Since,
		SpentMicros: s.SpentMicros, CeilingMicros: s.CeilingMicros,
	}
}
