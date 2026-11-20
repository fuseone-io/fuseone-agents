package spec_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/trigger"
)

/*
A publication moves the agent's schedules and webhooks with the version, in its own
transaction and under the agent's row (#416). Reconciled after the commit instead, two
publications committing A then B could reconcile B then A, and leave a cron B removed
firing, or a webhook B withdrew answering with its secret.
*/

// reconcilingPublisher is a publisher that reconciles, over tables emptied for the test.
func reconcilingPublisher(t *testing.T, pool *pgxpool.Pool) *spec.Publisher {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `truncate trigger_schedules, webhook_triggers`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return spec.NewPublisher(pool, fixedClock{}).
		WithReconciliation(trigger.NewPostgresSchedules(pool), trigger.NewPostgresWebhooks(pool))
}

func schedulesIn(t *testing.T, pool *pgxpool.Pool, agent domain.AgentID) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `select schedule from trigger_schedules where agent_id = $1`, string(agent))
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPublish_reconcilesTheVersionsSchedulesInItsTransaction(t *testing.T) {
	pool := openSpecPool(t)
	publisher := reconcilingPublisher(t, pool)
	version := published(t, definition)

	if _, err := publisher.Publish(t.Context(), spec.Publication{Spec: version, By: "usr_ana", Company: "acme"}); err != nil {
		t.Fatalf("Publish = %v", err)
	}
	if seen := schedulesIn(t, pool, version.ID); len(seen) != 1 || seen[0] != "*/15 * * * *" {
		t.Errorf("schedules = %v, want the published version's cron", seen)
	}
}

// Undone, not half done: a version live whose webhook another agent answers on is the
// failure after the commit this replaces.
func TestPublish_aWebhookPathTakenByAnotherAgent_undoesThePublication(t *testing.T) {
	pool := openSpecPool(t)
	first, _ := twoVersions(t, pool)
	publisher := reconcilingPublisher(t, pool)
	hooked := published(t, strings.Replace(definition,
		`triggers:`, "triggers:\n  - { type: webhook, path: crm/ticket }", 1))
	if err := trigger.NewPostgresWebhooks(pool).Sync(t.Context(), "someone-else",
		domain.Scope{Company: "acme", Area: "cx"}, []domain.WebhookTrigger{{Path: "crm/ticket"}}); err != nil {
		t.Fatalf("take the path: %v", err)
	}

	_, err := publisher.Publish(t.Context(), spec.Publication{Spec: hooked, By: "usr_ana", Company: "acme"})
	if !errors.Is(err, trigger.ErrPathTaken) {
		t.Fatalf("Publish = %v, want the taken path refused", err)
	}
	wroteNothing(t, pool, first, hooked)
	if got := schedulesIn(t, pool, first.ID); len(got) != 0 {
		t.Errorf("schedules = %v, want none written by an undone publication", got)
	}
}

// Read where the version became current: nothing about a publication is read after it.
func TestPublish_answersWhetherTheAgentIsPaused_asItsTransactionLeftIt(t *testing.T) {
	pool := openSpecPool(t)
	first, second := twoVersions(t, pool)
	publisher := spec.NewPublisher(pool, fixedClock{})

	judged, err := publisher.Publish(t.Context(), spec.Publication{Spec: second, By: "usr_ana", Company: "acme"})
	if err != nil || !judged.Paused {
		t.Fatalf("Publish = %+v, %v, want a new agent answered paused", judged, err)
	}
	if err := spec.NewState(pool).SetPaused(t.Context(), first.ID, false, "usr_ops", fixedClock{}.Now()); err != nil {
		t.Fatalf("SetPaused: %v", err)
	}
	judged, err = publisher.Publish(t.Context(), spec.Publication{Spec: first, By: "usr_ana", Company: "acme"})
	if err != nil || judged.Paused {
		t.Fatalf("Publish = %+v, %v, want a started agent answered running", judged, err)
	}
}

/*
A trigger a version declares is a trigger the platform reaches, and one it withdraws stops
firing: the same reconciliation with an empty list, not a second call. A console that let
somebody choose a schedule nothing ever fired was how this was first found.
*/
func TestPublish_aVersionWithdrawingItsSchedule_stopsIt(t *testing.T) {
	pool := openSpecPool(t)
	publisher := reconcilingPublisher(t, pool)
	first := published(t, definition)
	quiet := published(t, strings.Replace(definition, `  - { type: cron, schedule: "*/15 * * * *" }`+"\n", "", 1))
	for i, version := range []spec.Spec{first, quiet} {
		if _, err := publisher.Publish(t.Context(), spec.Publication{Spec: version, By: "usr_ana", Company: "acme"}); err != nil {
			t.Fatalf("Publish(%s) = %v", version.Version, err)
		}
		if got := schedulesIn(t, pool, first.ID); i == 0 && len(got) != 1 {
			t.Fatalf("schedules = %v, want the first version's cron before it is withdrawn", got)
		}
	}
	if got := schedulesIn(t, pool, first.ID); len(got) != 0 {
		t.Errorf("schedules = %v, want none after a version withdrew it", got)
	}
}
