package ledger

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
)

// The fallback filters, exported to the plan test so it explains the exact
// text the aggregates run rather than a copy that could drift from it.
const (
	DeclaredPairRunsFilter  = declaredPairRuns
	DeclaredPairStepsFilter = declaredPairSteps
)

// EventDispatchBacklogSQL is the backlog statement, for the plan test that
// bounds what it reads.
const EventDispatchBacklogSQL = eventDispatchBacklogSQL

// TestMigration is one migration file, for a runner test that stages its own.
type TestMigration struct{ Version, Body string }

// MigrateBodies runs the production runner over the given files, as if they
// were the embedded set.
func MigrateBodies(ctx context.Context, pool *pgxpool.Pool, files []TestMigration) error {
	set := make([]embeddedMigration, 0, len(files))
	for _, f := range files {
		set = append(set, embeddedMigration{version: f.Version, body: []byte(f.Body), digest: digestBytes([]byte(f.Body))})
	}
	return migrateSet(ctx, pool, set, migrationLogger(nil))
}

// MigrationDigest is the digest the runner records for a body.
func MigrationDigest(body string) string { return digestBytes([]byte(body)) }

// WorkItemRunsSQL and WorkItemRolesSQL are the trail's page and its role
// summary, for the plan tests that hold them to the ref index, an aggregate
// and partition pruning.
var (
	WorkItemRunsSQL  = workItemRunsSQL
	WorkItemRolesSQL = workItemRolesSQL
)

// StartsAndFinishesSQL is the dispatcher's batch read, for the plan test that
// holds it to the batch's months.
const StartsAndFinishesSQL = startsAndFinishesSQL

// PlantWorkItemClaim records that a run claimed ref under an agent and scope
// that need not be the run's own, bypassing the append's judgement — the
// state an older release or a hand-written row could leave behind, which the
// trail must still attribute to the run's own identity.
func (p *Postgres) PlantWorkItemClaim(
	ctx context.Context, runID domain.RunID, agent domain.AgentID, scope domain.Scope, ref domain.WorkItemRef,
) error {
	_, err := p.pool.Exec(ctx, `
		insert into run_work_items (run_id, company_id, area_id, agent_id, ref, claimed_seq, claimed_at)
		select run_id, $2, $3, $4, $5, last_seq, updated_at from runs where run_id = $1`,
		string(runID), string(scope.Company), string(scope.Area), string(agent), string(ref))
	return err
}

// PlantWorkItemClaim is the memory store's equivalent: a claim step under
// that agent and scope, sealed onto the run without the claim's judgement.
func (m *Memory) PlantWorkItemClaim(
	ctx context.Context, runID domain.RunID, agent domain.AgentID, scope domain.Scope, ref domain.WorkItemRef,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	steps := m.runs[runID]
	if len(steps) == 0 {
		return fmt.Errorf("no run %s", runID)
	}
	payload, err := json.Marshal(domain.WorkItemClaimedPayload{Ref: ref})
	if err != nil {
		return err
	}
	last := steps[len(steps)-1]
	m.runs[runID] = append(steps, domain.Step{
		RunID: runID, Seq: last.Seq + 1, Kind: domain.StepWorkItemClaimed,
		Scope: scope, AgentID: agent, VersionID: last.VersionID, At: last.At, Payload: payload,
	})
	return nil
}

// FinishedSinceSQL is one page of the dispatcher's feed, for the same test.
const FinishedSinceSQL = finishedSinceSQL

// AppendInTx seals a step inside a transaction the test holds open, so a race
// can be staged on the production append rather than on a copy of its SQL.
func AppendInTx(ctx context.Context, tx pgx.Tx, s domain.Step) (domain.Step, error) {
	return appendTx(ctx, tx, nil, nil, s)
}

// WorkItemMeasuresSQL is the weekly measures' one statement, for the plan
// test that holds it to the window's months and the runs' indexes.
var WorkItemMeasuresSQL = workItemMeasuresSQL
