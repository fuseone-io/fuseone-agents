/*
Package standing stores standing approvals: a human grant given ahead of time.

SE-06 names exactly one exception to the taint rule — explicit human
approval — and a standing approval is that approval made durable: one person
pre-approving one exact tool for one agent in one scope, with a daily
ceiling and an expiry. The person's name travels onto every release the
grant produces, which is what keeps the trail answering "who authorised
this" at three in the morning.

Revocation flips status and keeps the row. A grant that vanished would leave
decided steps pointing at a mandate nobody can read.
*/
package standing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

const (
	// DefaultTTLDays is how long a grant lives when its author does not say.
	DefaultTTLDays = 30
	// MaxTTLDays bounds the mandate: renewing is a deliberate re-decision,
	// and a grant nobody has looked at for a quarter is not a decision any
	// more.
	MaxTTLDays  = 90
	MaxDailyCap = 100
)

// ErrGrantNotFound means no active grant carries that id.
var ErrGrantNotFound = errors.New("standing: no active grant with that id")

type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
)

// Grant is one standing approval.
type Grant struct {
	ID        string
	Tool      domain.ToolID
	Agent     domain.AgentID
	Scope     domain.Scope
	DailyCap  int
	Reason    string
	Status    Status
	CreatedBy domain.UserID
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedBy domain.UserID
	RevokedAt *time.Time
	// UsesToday is filled on List for the console; Claim counts for real.
	UsesToday int
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// UsedAction is the admin_events action each consumed use records. The count
// of today's rows is the ceiling's ledger: Claim counts and inserts in one
// transaction, so two runs cannot spend the same slot.
const UsedAction = "standing_approval.used"

// Validate refuses what a mandate must never be: a glob, a missing reason,
// an unbounded life, an implausible ceiling.
func Validate(g Grant, now time.Time) error {
	switch {
	case strings.TrimSpace(string(g.Tool)) == "" || strings.ContainsAny(string(g.Tool), "*?["):
		return fmt.Errorf("standing: the tool must be one exact id, never a pattern")
	case strings.TrimSpace(string(g.Agent)) == "":
		return fmt.Errorf("standing: the grant names one agent")
	case strings.TrimSpace(string(g.Scope.Company)) == "":
		return fmt.Errorf("standing: the grant names a scope")
	case g.DailyCap < 1 || g.DailyCap > MaxDailyCap:
		return fmt.Errorf("standing: the daily ceiling must be between 1 and %d", MaxDailyCap)
	case strings.TrimSpace(g.Reason) == "":
		return fmt.Errorf("standing: a mandate without a reason is not a decision")
	case !g.ExpiresAt.After(now):
		return fmt.Errorf("standing: the grant must expire in the future")
	case g.ExpiresAt.After(now.Add(MaxTTLDays * 24 * time.Hour)):
		return fmt.Errorf("standing: the grant cannot live past %d days; renewing is a re-decision", MaxTTLDays)
	}
	return nil
}

func (s *Store) Create(ctx context.Context, g Grant) error {
	if err := Validate(g, g.CreatedAt); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
		insert into standing_approvals
			(grant_id, tool, agent_id, company_id, area_id, daily_cap,
			 reason, status, created_by, created_at, expires_at)
		values ($1, $2, $3, $4, $5, $6, $7, 'active', $8, $9, $10)`,
		g.ID, string(g.Tool), string(g.Agent),
		string(g.Scope.Company), string(g.Scope.Area),
		g.DailyCap, g.Reason, string(g.CreatedBy), g.CreatedAt, g.ExpiresAt,
	); err != nil {
		return fmt.Errorf("standing: create: %w", err)
	}
	return nil
}

func (s *Store) Revoke(ctx context.Context, id string, by domain.UserID, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		update standing_approvals
		set status = 'revoked', revoked_by = $2, revoked_at = $3
		where grant_id = $1 and status = 'active'`,
		id, string(by), at)
	if err != nil {
		return fmt.Errorf("standing: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrGrantNotFound
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]Grant, error) {
	rows, err := s.pool.Query(ctx, `
		select g.grant_id, g.tool, g.agent_id, g.company_id, g.area_id,
			g.daily_cap, g.reason, g.status, g.created_by, g.created_at,
			g.expires_at, g.revoked_by, g.revoked_at,
			(select count(*) from admin_events e
			 where e.action = $1 and e.target = g.grant_id
			   and e.at >= date_trunc('day', now())) as uses_today
		from standing_approvals g
		order by g.created_at desc`, UsedAction)
	if err != nil {
		return nil, fmt.Errorf("standing: list: %w", err)
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		var tool, agent, company, area, status, createdBy, revokedBy string
		if err := rows.Scan(&g.ID, &tool, &agent, &company, &area,
			&g.DailyCap, &g.Reason, &status, &createdBy, &g.CreatedAt,
			&g.ExpiresAt, &revokedBy, &g.RevokedAt, &g.UsesToday); err != nil {
			return nil, fmt.Errorf("standing: scan: %w", err)
		}
		g.Tool, g.Agent = domain.ToolID(tool), domain.AgentID(agent)
		g.Scope = domain.Scope{Company: domain.CompanyID(company), Area: domain.AreaID(area)}
		g.Status, g.CreatedBy, g.RevokedBy = Status(status), domain.UserID(createdBy), domain.UserID(revokedBy)
		out = append(out, g)
	}
	return out, rows.Err()
}

/*
Claim finds an active, unexpired grant covering the exact tool, agent and
scope, and consumes one of today's uses in the same transaction.

The count and the insert share the transaction so two runs cannot spend the
same slot: the ceiling is a brake, and a brake that over-counts under load
is a brake that failed. The admin event is the use's record — target is the
grant, detail names the run — and it is what List and the next Claim count.

If the caller's append later loses a race to a human decision, the slot
stays spent. One burned slot on a rare race is the cheap side of that trade.
*/
func (s *Store) Claim(
	ctx context.Context, tool domain.ToolID, agent domain.AgentID,
	scope domain.Scope, runID domain.RunID, now time.Time,
) (Grant, bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Grant{}, false, fmt.Errorf("standing: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var g Grant
	var createdBy string
	err = tx.QueryRow(ctx, `
		select g.grant_id, g.daily_cap, g.reason, g.created_by
		from standing_approvals g
		where g.tool = $1 and g.agent_id = $2
		  and g.company_id = $3 and g.area_id = $4
		  and g.status = 'active' and g.expires_at > $5
		order by g.created_at
		limit 1`,
		string(tool), string(agent),
		string(scope.Company), string(scope.Area), now,
	).Scan(&g.ID, &g.DailyCap, &g.Reason, &createdBy)
	if err == pgx.ErrNoRows {
		return Grant{}, false, nil
	}
	if err != nil {
		return Grant{}, false, fmt.Errorf("standing: cover: %w", err)
	}
	g.Tool, g.Agent, g.Scope = tool, agent, scope
	g.CreatedBy = domain.UserID(createdBy)

	var usedToday int
	if err := tx.QueryRow(ctx, `
		select count(*) from admin_events
		where action = $1 and target = $2 and at >= date_trunc('day', $3::timestamptz)`,
		UsedAction, g.ID, now,
	).Scan(&usedToday); err != nil {
		return Grant{}, false, fmt.Errorf("standing: count uses: %w", err)
	}
	if usedToday >= g.DailyCap {
		return Grant{}, false, nil
	}
	// The use is stamped with the caller's clock, the same one the count
	// reads: a ceiling whose spender and counter watch different clocks is
	// a ceiling with a seam at midnight.
	if _, err := tx.Exec(ctx, `
		insert into admin_events (principal_id, company_id, area_id, action, target, detail, at)
		values ($1, $2, $3, $4, $5, $6, $7)`,
		string(g.CreatedBy), string(scope.Company), string(scope.Area),
		UsedAction, g.ID,
		fmt.Sprintf(`{"run":%q,"tool":%q}`, string(runID), string(tool)),
		now,
	); err != nil {
		return Grant{}, false, fmt.Errorf("standing: record use: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Grant{}, false, fmt.Errorf("standing: commit: %w", err)
	}
	return g, true, nil
}
