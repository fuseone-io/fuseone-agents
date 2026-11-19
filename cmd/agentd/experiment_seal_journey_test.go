package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/spec"
)

/*
An experiment on a candidate, from the route to the provider the sealed text names (NT-018
phase 2, release 2b).

The seal is written the way the seal route writes it. The route checks the seal before the
first run opens, opens every run on it in the sealed scope, and the production simulation
pool runs the sealed bytes: the provider and model only the sealed text names are the ones
asked. A retry inherits the seal, refuses another, and once runs exist answers from them
even after the seal is gone.
*/

const (
	routeSeal      = "7c3d4e5f-6a7b-4c8d-9e0f-1a2b3c4d5e6f"
	otherRouteSeal = "8d4e5f6a-7b8c-4d9e-0f1a-2b3c4d5e6f70"
)

// experimentIDFor names an experiment as the route does: its agent and its key.
func experimentIDFor(agent domain.AgentID, key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("exp_%s_%s", agent, hex.EncodeToString(sum[:8]))
}

func onSeal(seal string) *openapi.ExperimentRequest {
	return &openapi.ExperimentRequest{SealId: &seal}
}

func (w *experimentWorld) startOnSeal(
	t *testing.T, ctx context.Context, key string, body *openapi.ExperimentRequest,
) openapi.StartExperimentResponseObject {
	t.Helper()
	resp, err := w.server.StartExperiment(ctx, openapi.StartExperimentRequestObject{
		AgentId: string(w.agent), Params: openapi.StartExperimentParams{IdempotencyKey: key}, Body: body,
	})
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	return resp
}

// resume lets the published agent run, as an operator would: a published agent is born
// paused, and a recorded pause refuses a candidate too (decision 3).
func (w *experimentWorld) resume(t *testing.T) {
	t.Helper()
	if err := spec.NewState(w.pool).SetPaused(t.Context(), w.agent, false, "usr_ops", time.Now()); err != nil {
		t.Fatalf("resume: %v", err)
	}
}

func (w *experimentWorld) eraseSeal(t *testing.T, seal string) {
	t.Helper()
	tx, err := w.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := ledger.NewContent(w.pool).EraseOwnersTx(t.Context(), tx, "authoring", []string{seal}, "test"); err != nil {
		t.Fatalf("erase the seal: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func conflictCode(resp openapi.StartExperimentResponseObject) (string, bool) {
	c, ok := resp.(openapi.StartExperiment409ApplicationProblemPlusJSONResponse)
	if !ok || c.Type == nil {
		return "", ok
	}
	return *c.Type, true
}

func TestExperiment_onASeal_fromTheRoute_runsTheSealedDefinition(t *testing.T) {
	world := newExperimentWorld(t)
	world.resume(t)
	version := world.seal(t, routeSeal)

	resp := world.startOnSeal(t, author(t.Context()), "journey-seal-1", onSeal(routeSeal))
	accepted, ok := resp.(openapi.StartExperiment202JSONResponse)
	if !ok || accepted.Runs != 1 {
		if c, isConflict := resp.(openapi.StartExperiment409ApplicationProblemPlusJSONResponse); isConflict && c.Detail != nil {
			t.Fatalf("StartExperiment answered 409: %s", *c.Detail)
		}
		t.Fatalf("StartExperiment answered %T %+v, want one run on the seal", resp, resp)
	}
	runs, err := world.store.SimulationRuns(t.Context(), accepted.Id)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v (%v)", runs, err)
	}
	steps, err := world.store.Read(t.Context(), runs[0], domain.FirstSeq)
	if err != nil || steps[0].VersionID != version {
		t.Fatalf("the run opened on %v (%v), want the sealed version %s", steps[0].VersionID, err, version)
	}

	if state := world.settleCandidate(t, runs[0]); state.Phase != engine.PhaseFinished {
		t.Fatalf("phase = %v, want finished", state.Phase)
	}
	if got := world.tested.models(); !slices.Equal(got, []string{"sealed-model"}) {
		t.Errorf("the sealed provider was asked %v, want the sealed model", got)
	}
	if got := world.declared.models(); len(got) != 0 {
		t.Errorf("the published version's provider was asked %v, want nothing", got)
	}
}

func TestExperiment_onASeal_aRetryInheritsTheSealAndRefusesAnother(t *testing.T) {
	world := newExperimentWorld(t)
	world.resume(t)
	world.seal(t, routeSeal)
	world.seal(t, otherRouteSeal)
	first, ok := world.startOnSeal(t, author(t.Context()), "journey-seal-2", onSeal(routeSeal)).(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatal("the first attempt was not accepted")
	}

	inherited, ok := world.startOnSeal(t, author(t.Context()), "journey-seal-2", &openapi.ExperimentRequest{}).(openapi.StartExperiment202JSONResponse)
	if !ok || inherited.Id != first.Id || inherited.Runs != first.Runs {
		t.Errorf("a retry leaving the seal out = %+v, want the first attempt's experiment", inherited)
	}
	version := "v000000000000"
	for name, body := range map[string]*openapi.ExperimentRequest{
		"another seal": onSeal(otherRouteSeal), "a version": {Version: &version},
		// The value the bug takes: a seal that does not exist must not be judged before the
		// recorded intention, or the retry is a 404 and not the conflict it is.
		"a seal nobody wrote": onSeal("9f9f9f9f-7b8c-4d9e-0f1a-2b3c4d5e6f70"),
	} {
		if _, isConflict := conflictCode(world.startOnSeal(t, author(t.Context()), "journey-seal-2", body)); !isConflict {
			t.Errorf("a retry naming %s was not a conflict", name)
		}
	}
}

func TestExperiment_onAnErasedSeal_refusesTheFirstAttemptButAnswersARetryFromItsRuns(t *testing.T) {
	world := newExperimentWorld(t)
	world.resume(t)
	world.seal(t, routeSeal)
	world.seal(t, otherRouteSeal)
	world.eraseSeal(t, otherRouteSeal)

	code, isConflict := conflictCode(world.startOnSeal(t, author(t.Context()), "journey-seal-3", onSeal(otherRouteSeal)))
	if !isConflict || code != "fuseone:candidate-unreadable" {
		t.Fatalf("a first attempt on an erased seal = %q (conflict=%v), want fuseone:candidate-unreadable", code, isConflict)
	}
	if runs, _ := world.store.SimulationRuns(t.Context(), experimentIDFor(world.agent, "journey-seal-3")); len(runs) != 0 {
		t.Errorf("the refused attempt opened %v", runs)
	}

	first, ok := world.startOnSeal(t, author(t.Context()), "journey-seal-4", onSeal(routeSeal)).(openapi.StartExperiment202JSONResponse)
	if !ok {
		t.Fatal("the first attempt was not accepted")
	}
	world.eraseSeal(t, routeSeal)
	again, ok := world.startOnSeal(t, author(t.Context()), "journey-seal-4", &openapi.ExperimentRequest{}).(openapi.StartExperiment202JSONResponse)
	if !ok || again.Runs != first.Runs {
		t.Errorf("a retry after the seal was erased = %+v, want the runs the experiment already holds", again)
	}
}

func TestExperiment_onASeal_outsideTheSealedScope_isNotFound(t *testing.T) {
	world := newExperimentWorld(t)
	world.seal(t, routeSeal)
	sales := auth.WithPrincipal(t.Context(), domain.Principal{
		ID: "usr_bia", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "sales"}, Role: domain.RoleAuthor}},
	})
	for name, seal := range map[string]string{"another area's author": routeSeal} {
		if _, absent := world.startOnSeal(t, sales, "journey-seal-5", onSeal(seal)).(openapi.StartExperiment404ApplicationProblemPlusJSONResponse); !absent {
			t.Errorf("%s was not answered 404", name)
		}
	}
	if _, absent := world.startOnSeal(t, author(t.Context()), "journey-seal-6",
		onSeal("9f9f9f9f-7b8c-4d9e-0f1a-2b3c4d5e6f70")).(openapi.StartExperiment404ApplicationProblemPlusJSONResponse); !absent {
		t.Error("a seal nobody wrote was not answered 404")
	}
}

// A candidate needs no published version: the first version of a new agent is
// experimented on before anything is published.
func TestExperiment_onASeal_ofAnAgentNeverPublished_opensItsRuns(t *testing.T) {
	world := newExperimentWorld(t)
	world.agent = domain.AgentID(fmt.Sprintf("fresh-%d", time.Now().UnixNano()))
	world.seal(t, routeSeal)
	content := ledger.NewContent(world.pool)
	ref, err := content.RestoreFor(t.Context(), "regression", string(world.agent), 1, []byte(`{"ask":"refund"}`))
	if err != nil {
		t.Fatalf("keep the occurrence: %v", err)
	}
	if err := regression.NewStore(world.pool).Record(t.Context(), domain.RegressionCase{
		ID: "refund", Agent: world.agent, Scope: domain.Scope{Company: "acme", Area: "cx"},
		InputRef: ref, FromRun: "run-origin",
		Expectations: []domain.Expectation{{Kind: domain.ExpectNeverCalls, Value: "crm.refund"}},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	accepted, ok := world.startOnSeal(t, author(t.Context()), "journey-seal-7", onSeal(routeSeal)).(openapi.StartExperiment202JSONResponse)
	if !ok || accepted.Runs != 1 {
		t.Fatalf("an experiment on an unpublished agent's seal = %T %+v, want its run opened", accepted, accepted)
	}
	// With nothing published, only the recorded seal can answer a retry that leaves it out.
	again, ok := world.startOnSeal(t, author(t.Context()), "journey-seal-7", &openapi.ExperimentRequest{}).(openapi.StartExperiment202JSONResponse)
	if !ok || again.Id != accepted.Id || again.Runs != accepted.Runs {
		t.Errorf("a retry leaving the seal out = %+v, want the recorded experiment", again)
	}
}

func TestExperiment_namingBothAVersionAndASeal_isABadRequest(t *testing.T) {
	world := newExperimentWorld(t)
	world.seal(t, routeSeal)
	version := "v000000000000"
	seal := routeSeal
	body := &openapi.ExperimentRequest{Version: &version, SealId: &seal}
	if resp := world.startOnSeal(t, author(t.Context()), "journey-seal-8", body); !isBadRequest(resp) {
		t.Errorf("both a version and a seal = %T, want 400", resp)
	}
	// Judged on the request alone: an agent with nothing published answers the same.
	world.agent = domain.AgentID(fmt.Sprintf("fresh-%d", time.Now().UnixNano()))
	world.seal(t, otherRouteSeal)
	other := otherRouteSeal
	body.SealId = &other
	if resp := world.startOnSeal(t, author(t.Context()), "journey-seal-9", body); !isBadRequest(resp) {
		t.Errorf("both on an agent never published = %T, want 400", resp)
	}
}

func isBadRequest(resp openapi.StartExperimentResponseObject) bool {
	_, bad := resp.(openapi.StartExperiment400ApplicationProblemPlusJSONResponse)
	return bad
}
