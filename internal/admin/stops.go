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
The switches that stop things without a deploy (PRD FO-06).

Stored as settings rows so they survive a restart and so the trail records who
threw one. An in-memory switch would come back on when the process did, which
is the opposite of what somebody wants from the control they reach for during
an incident.

Read on the path of every trigger, so it has to be cheap: one query returning
the few rows that are off, rather than a question per agent.
*/

const stopKind = "stop"

type Stops struct{ pool *pgxpool.Pool }

func NewStops(pool *pgxpool.Pool) *Stops { return &Stops{pool: pool} }

// InForce is every switch currently off.
//
// Empty is the normal state and the cheap one: a platform nobody has stopped
// answers with no rows.
func (s *Stops) InForce(ctx context.Context) ([]domain.Stop, error) {
	rows, err := s.pool.Query(ctx, `
		select name, value, updated_by, updated_at from settings
		where kind = $1 and enabled order by name`, stopKind)
	if err != nil {
		return nil, fmt.Errorf("admin: read stops: %w", err)
	}
	defer rows.Close()

	var out []domain.Stop
	for rows.Next() {
		var (
			name, by string
			raw      []byte
			at       time.Time
			stored   struct {
				Level   string `json:"level"`
				Company string `json:"company,omitempty"`
				Area    string `json:"area,omitempty"`
				Agent   string `json:"agent,omitempty"`
				Reason  string `json:"reason"`
			}
		)
		if err := rows.Scan(&name, &raw, &by, &at); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &stored); err != nil {
			return nil, fmt.Errorf("admin: decode stop %s: %w", name, err)
		}
		out = append(out, domain.Stop{
			Level: domain.StopLevel(stored.Level),
			Scope: domain.Scope{
				Company: domain.CompanyID(stored.Company),
				Area:    domain.AreaID(stored.Area),
			},
			Agent: domain.AgentID(stored.Agent), Reason: stored.Reason,
			By: domain.UserID(by), At: at,
		})
	}
	return out, rows.Err()
}

// Stop throws a switch, and records who and why in the same transaction.
func (s *Stops) Stop(ctx context.Context, stop domain.Stop) error {
	if !stop.Level.Valid() {
		return fmt.Errorf("admin: %q is not a level of stop", stop.Level)
	}
	if stop.Reason == "" {
		// Somebody else will find this. A platform stopped for no stated
		// reason makes "did we do this on purpose?" the first question of the
		// incident call.
		return fmt.Errorf("admin: a stop needs a reason")
	}
	return s.write(ctx, stop, true, "platform.stopped")
}

// Start takes a switch off. The row stays, disabled, so the trail shows the
// platform was stopped and then started rather than showing nothing at all.
func (s *Stops) Start(ctx context.Context, stop domain.Stop) error {
	if !stop.Level.Valid() {
		return fmt.Errorf("admin: %q is not a level of stop", stop.Level)
	}
	return s.write(ctx, stop, false, "platform.started")
}

func (s *Stops) write(ctx context.Context, stop domain.Stop, on bool, action string) error {
	value, err := json.Marshal(struct {
		Level   string `json:"level"`
		Company string `json:"company,omitempty"`
		Area    string `json:"area,omitempty"`
		Agent   string `json:"agent,omitempty"`
		Reason  string `json:"reason"`
	}{
		string(stop.Level), string(stop.Scope.Company), string(stop.Scope.Area),
		string(stop.Agent), stop.Reason,
	})
	if err != nil {
		return fmt.Errorf("admin: encode stop: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("admin: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by, updated_at)
		values ('installation', '', '', $1, $2, $3, $4, $5, now())
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			value = excluded.value, enabled = excluded.enabled,
			updated_by = excluded.updated_by, updated_at = now()`,
		stopKind, stop.Key(), value, on, string(stop.By)); err != nil {
		return fmt.Errorf("admin: write stop %s: %w", stop.Key(), err)
	}

	if err := Record(ctx, tx, Event{
		Principal: stop.By, Scope: stop.Scope, Action: action,
		Target: stop.Key(), Detail: json.RawMessage(value),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
