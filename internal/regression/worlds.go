package regression

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

/*
Worlds reads the world a replayed case is answered with (NT-018 phase 5).

A reader of its own and not the Store: it takes no fence and holds no connection outside the
pool, so the simulation pool can have one without a second Store, whose fence budget means
something only while there is one per process.
*/
type Worlds struct{ pool *pgxpool.Pool }

func NewWorlds(pool *pgxpool.Pool) *Worlds { return &Worlds{pool: pool} }

/*
WorldOf answers the world of a case as the version a run was opened on recorded it: nil for a
case with none. The version and never the working set, so what is replayed is what the run
says it checked. A case missing from that version, an object erased or never kept, or one that
names another case is ErrWorldUnreadable: none of them can be answered honestly.
*/
func (w *Worlds) WorldOf(
	ctx context.Context, agent domain.AgentID, version int64, caseID string,
) (*domain.CaseWorld, error) {
	var ref string
	var body []byte
	var erased *time.Time
	err := w.pool.QueryRow(ctx, `
		select v.world_ref, rc.bytes, rc.erased_at
		from dataset_version_cases v
		left join run_content rc on rc.ref = v.world_ref and v.world_ref <> ''
		where v.dataset_id = $1 and v.version = $2 and v.case_id = $3`,
		defaultDatasetOf(agent), version, caseID).Scan(&ref, &body, &erased)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s is not in version %d of %s", domain.ErrWorldUnreadable, caseID, version, agent)
	}
	if err != nil {
		return nil, fmt.Errorf("regression: read the world of %s: %w", caseID, err)
	}
	if ref == "" {
		return nil, nil
	}
	if erased != nil || body == nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrWorldUnreadable, ref)
	}
	var world domain.CaseWorld
	if err := json.Unmarshal(body, &world); err != nil || world.Case != caseID {
		return nil, fmt.Errorf("%w: %s does not name %s", domain.ErrWorldUnreadable, ref, caseID)
	}
	return &world, nil
}
