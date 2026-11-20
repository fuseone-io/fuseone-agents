package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/testdb"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Both ordinary doors move the agent's schedules and webhooks with the version, in the
publication's transaction (#416): the file's `PUT /agents/{id}/versions` and the
workbench's `POST /agents/{id}/draft/publish`. Reconciled after the commit, two
publications committing A then B could reconcile B then A, leaving a cron B removed
firing, or a webhook B withdrew answering with its secret.
*/

// reconcilingWorld is the publish world with the publisher serve wires: one that
// reconciles, through schedules.
func reconcilingWorld(t *testing.T, schedules func(*pgxpool.Pool) spec.ScheduleSync) (publishWorld, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	store := drafts.NewStore(pool)
	publisher := spec.NewPublisher(pool, engine.SystemClock{}).
		WithReconciliation(schedules(pool), trigger.NewPostgresWebhooks(pool))
	api := httpapi.NewServer(ledger.NewMemory(), "test").
		WithAgents(spec.NewRegistry(pool)).WithPublisher(publisher).
		WithRetirements(spec.NewState(pool)).WithDrafts(store).
		WithCandidates(spec.NewCandidateReader(pool, ledger.NewContent(pool)))
	return publishWorld{
		draftJourney: draftJourney{etagJourney{handler: apiHandlerFor(api)}},
		store:        store, agent: fmt.Sprintf("nt018-reconcile-%d", time.Now().UnixNano()),
	}, pool
}

func realSchedules(pool *pgxpool.Pool) spec.ScheduleSync { return trigger.NewPostgresSchedules(pool) }

// stalling is a schedule sync that holds the first reconciliation inside its transaction.
func stalling(stall *stallingSchedules) func(*pgxpool.Pool) spec.ScheduleSync {
	return func(pool *pgxpool.Pool) spec.ScheduleSync {
		*stall = stallingSchedules{PostgresSchedules: trigger.NewPostgresSchedules(pool), once: &sync.Once{},
			entered: make(chan struct{}), release: make(chan struct{})}
		return *stall
	}
}

// putTriggered publishes the agent from the file door, unguarded, declaring triggers.
func (w publishWorld) putTriggered(t *testing.T, triggers ...openapi.AgentTrigger) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(openapi.AgentDefinition{
		Name: w.agent, Company: "acme", Area: "factory",
		Provider: "openai", Model: "gpt-test", Instructions: "Triar chamados.",
		Tools:    &[]string{"crm.lookup"},
		Budget:   &openapi.Budget{Micros: ptrTo(int64(500_000)), Steps: ptrTo(int64(60))},
		Triggers: &triggers,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return w.do(t, areaAuthor("factory"), http.MethodPut, "/api/v1/agents/"+w.agent+"/versions", body, nil)
}

func cron(schedule string) openapi.AgentTrigger {
	return openapi.AgentTrigger{Type: "cron", Schedule: ptrTo(schedule)}
}

// triggeredDraft is a draft ready to publish, declaring triggers.
func (w publishWorld) triggeredDraft(t *testing.T, triggers string) string {
	t.Helper()
	tag := w.accept(t, w.ready(t), draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer prazo.", Locale: "pt-BR"})
	rec := w.apply(t, areaAuthor("factory"), w.agent, tag, `{"kind":"set_triggers","triggers":`+triggers+`}`)
	wantStatus(t, "set_triggers", rec, http.StatusOK)
	return rec.Header().Get("ETag")
}

func schedulesFor(t *testing.T, pool *pgxpool.Pool, agent string) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `select schedule from trigger_schedules where agent_id = $1`, agent)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

/*
A slower publication never puts its triggers back after a later one. A, from the door
under test, stalls while it reconciles; B, from the file door, waits for A's locks
instead of committing around it, and reconciles after A. The schedules end as B
declares them.
*/
func TestPublication_aStalledReconciliation_holdsTheNextPublication_andTheLatestWins(t *testing.T) {
	for name, first := range map[string]func(publishWorld, *testing.T) *httptest.ResponseRecorder{
		"file": func(w publishWorld, t *testing.T) *httptest.ResponseRecorder {
			return w.putTriggered(t, cron("*/15 * * * *"))
		},
		"workbench": func(w publishWorld, t *testing.T) *httptest.ResponseRecorder {
			return w.publish(t, areaAuthor("factory"), w.triggeredDraft(t, `[{"type":"cron","schedule":"*/15 * * * *"}]`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stall stallingSchedules
			w, pool := reconcilingWorld(t, stalling(&stall))

			a := make(chan *httptest.ResponseRecorder, 1)
			go func() { a <- first(w, t) }()
			<-stall.entered
			b := make(chan *httptest.ResponseRecorder, 1)
			go func() { b <- w.putTriggered(t, cron("*/30 * * * *")) }()
			select {
			case rec := <-b:
				t.Fatalf("B committed around A's reconciliation: %d %s", rec.Code, rec.Body)
			case <-time.After(400 * time.Millisecond):
			}
			close(stall.release)

			wantStatus(t, "A", <-a, http.StatusOK)
			wantStatus(t, "B", <-b, http.StatusOK)
			if got := schedulesFor(t, pool, w.agent); len(got) != 1 || got[0] != "*/30 * * * *" {
				t.Errorf("schedules = %v, want B's alone", got)
			}
		})
	}
}

/*
A webhook path another agent answers on refuses the publication, and nothing is
written: before, the version went live and the answer was an error after it.
*/
func TestPublication_aWebhookPathTakenByAnotherAgent_isRefusedAndWritesNothing(t *testing.T) {
	for name, publish := range map[string]func(w publishWorld, t *testing.T, path string) *httptest.ResponseRecorder{
		"file": func(w publishWorld, t *testing.T, path string) *httptest.ResponseRecorder {
			return w.putTriggered(t, cron("*/15 * * * *"), openapi.AgentTrigger{Type: "webhook", Path: ptrTo(path)})
		},
		"workbench": func(w publishWorld, t *testing.T, path string) *httptest.ResponseRecorder {
			return w.publish(t, areaAuthor("factory"), w.triggeredDraft(t,
				`[{"type":"cron","schedule":"*/15 * * * *"},{"type":"webhook","path":"`+path+`"}]`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			w, pool := reconcilingWorld(t, realSchedules)
			path := "crm/" + w.agent
			if err := trigger.NewPostgresWebhooks(pool).Sync(context.Background(), domain.AgentID(w.agent+"-other"),
				domain.Scope{Company: "acme", Area: "factory"}, []domain.WebhookTrigger{{Path: path}}); err != nil {
				t.Fatalf("take the path: %v", err)
			}

			rec := publish(w, t, path)
			if wantStatus(t, "publish", rec, http.StatusConflict); problemType(t, rec) != "fuseone:webhook-path-taken" {
				t.Errorf("refusal type %q, want webhook-path-taken", problemType(t, rec))
			}
			wantStatus(t, "the agent", w.do(t, areaAuthor("factory"), http.MethodGet, "/api/v1/agents/"+w.agent, nil, nil), http.StatusNotFound)
			if got := schedulesFor(t, pool, w.agent); len(got) != 0 {
				t.Errorf("schedules = %v, want none from a refused publication", got)
			}
		})
	}
}
