// Package admin owns what operators do to the platform.
//
// The run ledger records what agents did; this records what people did to the
// rules agents run under. An auditor needs both to explain an outcome: a run
// that wrote to a CRM is only explicable alongside the moment somebody
// classified that tool as a write.
package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fuseone/agents/internal/domain"
)

// db is what both a pool and a transaction satisfy, so every write here can
// join a caller's transaction. An administrative change and its record must
// commit together or not at all — a platform that can lose the record of a
// change while keeping the change is one nobody can audit.
type db interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Event is one administrative change.
type Event struct {
	Principal domain.UserID
	Scope     domain.Scope
	// Action names what happened, in past tense and namespaced by what it
	// touched: tool.classified, provider.created, budget.changed.
	Action string
	// Target identifies the thing that changed, so the trail can be read
	// backwards from an object.
	Target string
	Detail any
}

// Record writes an event. It takes the executor rather than owning one so the
// caller decides which transaction the record belongs to.
func Record(ctx context.Context, conn db, e Event) error {
	detail := []byte(`{}`)
	if e.Detail != nil {
		encoded, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("admin: encode event detail: %w", err)
		}
		detail = encoded
	}

	if _, err := conn.Exec(ctx, `
		insert into admin_events (principal_id, company_id, area_id, action, target, detail)
		values ($1, $2, $3, $4, $5, $6)`,
		string(e.Principal), string(e.Scope.Company), string(e.Scope.Area),
		e.Action, e.Target, detail); err != nil {
		return fmt.Errorf("admin: record %s: %w", e.Action, err)
	}
	return nil
}
