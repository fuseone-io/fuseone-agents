package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/autonomy"
	"github.com/fuseone/agents/internal/domain"
)

// autonomySweep is how often an agent being overruled is demoted.
//
// Often enough that a bad afternoon does not run all night, rare enough that
// the answer settles: demoting on every approval would move an agent's stage
// on one person's mind changing, which is noise rather than evidence.
const autonomySweep = 15 * time.Minute

// demotions records what the platform decided about an agent, in the trail
// where every other administrative change is.
type demotions struct{ pool *pgxpool.Pool }

func (d demotions) Record(
	ctx context.Context, agent domain.AgentID, action string, detail any,
) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("record %s: %w", action, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := admin.Record(ctx, tx, admin.Event{
		// No principal: nobody did this, which is the fact worth recording.
		// Attributing it to whoever happened to be signed in would put a
		// person's name on a decision they did not make.
		Action: action, Target: string(agent), Detail: detail,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// watchAutonomy demotes overruled agents until its context is cancelled.
func watchAutonomy(ctx context.Context, watch *autonomy.Watch) {
	ticker := time.NewTicker(autonomySweep)
	defer ticker.Stop()

	for {
		if demoted, err := watch.Sweep(ctx, time.Now()); err != nil {
			slog.Error("autonomy sweep failed", "err", err)
		} else if demoted > 0 {
			slog.Warn("agents demoted to copilot", "count", demoted)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
