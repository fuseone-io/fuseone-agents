package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/drift"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The battery on a clock replays a corpus only where its cases were read (NT-018 phase 5b, D3),
read through the stores the worker wires on Postgres. Before the agent moves, the battery
opens; after it is published in another area, the corpus, whose cases were read in the first,
is skipped: Watching says where the version is published, and the corpus's own scope is not
that answer.
*/
func TestJourney_aCorpusLeftBehindByAMove_isNotReplayedOnAClock(t *testing.T) {
	world := newExperimentWorld(t)
	sweep := func() int {
		corpus := regression.NewStore(world.pool)
		opener := &countingOpener{}
		rerun := drift.NewRerun(corpus, world.store, regression.NewCorpus(corpus, ledger.NewContent(world.pool)), opener, nil).
			Every(time.Hour)
		if _, err := rerun.Sweep(context.Background(), time.Now()); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
		return opener.opened
	}
	if opened := sweep(); opened == 0 {
		t.Fatal("the battery opened nothing before the move: the journey would pass without reaching the rule")
	}
	world.publishIn(t, "finance")
	if opened := sweep(); opened != 0 {
		t.Errorf("%d runs opened from a corpus left in cx by a move to finance, want none", opened)
	}
}

// publishIn publishes the world's agent in another area of acme, as an author of both.
func (w *experimentWorld) publishIn(t *testing.T, area string) {
	t.Helper()
	var definition openapi.AgentDefinition
	if err := json.Unmarshal([]byte(`{
		"name": "Triage", "company": "acme", "area": "`+area+`",
		"provider": "primary", "model": "declared-model",
		"instructions": "Classify the ticket, from `+area+`.", "tools": ["crm.lookup"],
		"budget": {"micros": 500000, "steps": 20}
	}`), &definition); err != nil {
		t.Fatalf("decode: %v", err)
	}
	both := auth.WithPrincipal(context.Background(), domain.Principal{
		ID: "usr_ana", Kind: domain.PrincipalUser, Grants: []domain.Grant{
			{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor},
			{Scope: domain.Scope{Company: "acme", Area: domain.AreaID(area)}, Role: domain.RoleAuthor},
		},
	})
	resp, err := w.server.PublishAgent(both, openapi.PublishAgentRequestObject{AgentId: string(w.agent), JSONBody: &definition})
	if err != nil {
		t.Fatalf("PublishAgent: %v", err)
	}
	if _, ok := resp.(openapi.PublishAgent200JSONResponse); !ok {
		t.Fatalf("publish in %s answered %T: %+v", area, resp, resp)
	}
}

type countingOpener struct{ opened int }

func (o *countingOpener) Open(_ context.Context, req trigger.Request) (trigger.Result, error) {
	o.opened++
	return trigger.Result{RunID: domain.RunID("run-" + req.IdemKey), Created: true}, nil
}
