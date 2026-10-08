package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// StandingTrail records who granted and who revoked a standing approval in
// admin_events, beside the grant row that is the record of the mandate
// itself.
type StandingTrail struct{ pool *pgxpool.Pool }

func NewStandingTrail(pool *pgxpool.Pool) StandingTrail { return StandingTrail{pool: pool} }

func (t StandingTrail) RecordStanding(
	ctx context.Context, action, target string, by domain.UserID, scope domain.Scope,
) error {
	return Record(ctx, t.pool, Event{
		Principal: by, Scope: scope, Action: action, Target: target,
	})
}
