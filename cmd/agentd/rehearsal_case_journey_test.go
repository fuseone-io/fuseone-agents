package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
)

/*
A rehearsal becomes a case that keeps its world (NT-018 phase 5b), on Postgres, through the
server and the corpus store the command wires: the case is recorded from the rehearsal's
answers, and the world the replay's reader reads back at the new version is the rehearsal's,
answer for answer.
*/
func TestJourney_aRehearsalKeptAsACase_isReadBackByTheReplay(t *testing.T) {
	world := newExperimentWorld(t)
	ctx := context.Background()
	content := ledger.NewContent(world.pool)
	answer := []byte(`{"plan":"gold"}`)
	rehearse(t, world, content, answer)

	resp, err := world.server.RecordRegression(author(ctx), openapi.RecordRegressionRequestObject{
		AgentId: string(world.agent),
		Body: &openapi.RecordRegressionJSONRequestBody{
			RunId:        "run-reh-journey",
			Expectations: []openapi.Expectation{{Kind: "never_calls", Value: ptrTo("crm.refund")}},
		},
	})
	if err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	kept, ok := resp.(openapi.RecordRegression201JSONResponse)
	if !ok {
		t.Fatalf("RecordRegression answered %T %+v, want the case kept", resp, resp)
	}
	version, _, err := regression.NewStore(world.pool).Pinned(ctx, world.agent)
	if err != nil {
		t.Fatalf("Pinned: %v", err)
	}
	replayed, err := regression.NewWorlds(world.pool).WorldOf(ctx, world.agent, version, kept.Id)
	if err != nil || replayed == nil || len(replayed.Entries) != 1 || replayed.Entries[0].ArgsDigest != "sha256:abc" {
		t.Fatalf("WorldOf = %+v (%v), want the rehearsal's one call", replayed, err)
	}
	if got, err := content.Get(ctx, replayed.Entries[0].AnswerRef); err != nil || string(got) != string(answer) {
		t.Errorf("the replayed answer is %q (%v), want the author's", got, err)
	}
}

// rehearse writes an ended rehearsal of the world's agent in acme/cx: the case typed, one
// call, the author's answer, and the run finished.
func rehearse(t *testing.T, world *experimentWorld, content *ledger.Content, answer []byte) {
	t.Helper()
	ctx := context.Background()
	run := domain.RunID("run-reh-journey")
	input, err := content.Put(ctx, run, 1, []byte("A cliente pede reembolso."))
	if err != nil {
		t.Fatal(err)
	}
	seq := int64(0)
	appendStep := func(kind domain.StepKind, payload any) domain.Step {
		raw, _ := json.Marshal(payload)
		s, err := world.store.Append(ctx, domain.Step{
			RunID: run, Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
			AgentID: world.agent, VersionID: "v-cand", At: time.Now(), Payload: raw,
		})
		if err != nil {
			t.Fatalf("append %s: %v", kind, err)
		}
		seq = s.Seq
		return s
	}
	appendStep(domain.StepRunStarted, domain.RunStartedPayload{
		Trigger: "rehearsal", Simulated: true, Simulation: "reh_journey", Rehearsal: true,
		SealID: "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11", InputRef: input,
	})
	called := appendStep(domain.StepToolCalled, domain.ToolCalledPayload{Tool: "crm.lookup", Effect: domain.EffectRead, ArgsDigest: "sha256:abc"})
	awaited := appendStep(domain.StepWorldAwaited, domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: called.Seq})
	ref, err := content.Put(ctx, run, seq+1, answer)
	if err != nil {
		t.Fatal(err)
	}
	appendStep(domain.StepWorldSupplied, domain.WorldSuppliedPayload{AtSeq: awaited.Seq, Ref: ref, Bytes: int64(len(answer)), By: "usr_ana"})
	appendStep(domain.StepToolReturned, domain.ToolReturnedPayload{Tool: "crm.lookup", ForSeq: called.Seq, ResultRef: ref})
	appendStep(domain.StepRunFinished, domain.RunFinishedPayload{Outcome: "answered"})
}
