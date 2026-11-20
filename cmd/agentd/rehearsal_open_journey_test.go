package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/spec"
)

/*
Opening a live rehearsal from the route (NT-018 phase 4, release 4b).

A rehearsal runs one candidate on one case the author typed, simulated, in the sealed
scope, and belongs to the author's conversation on the draft the seal was sealed from. The
route finds the seal under the caller's own authority, reads its bytes inside the agent's
fence before anything is written, and answers a retry under the same key with the run it
opened, or refuses one that asks for something else.
*/

const rehearsalSeal = "9e5f6a7b-8c9d-4e0f-a1b2-c3d4e5f6a7b8"

func (w *experimentWorld) withDrafts() *experimentWorld {
	w.server = w.server.WithDrafts(drafts.NewStore(w.pool))
	return w
}

func (w *experimentWorld) rehearse(
	t *testing.T, ctx context.Context, key, seal, text string,
) openapi.OpenRehearsalResponseObject {
	t.Helper()
	resp, err := w.server.OpenRehearsal(ctx, openapi.OpenRehearsalRequestObject{
		AgentId: string(w.agent), Params: openapi.OpenRehearsalParams{IdempotencyKey: key},
		Body: &openapi.RehearsalRequest{SealId: seal, Case: text},
	})
	if err != nil {
		t.Fatalf("OpenRehearsal: %v", err)
	}
	return resp
}

func (w *experimentWorld) draftOf(t *testing.T, seal string) string {
	t.Helper()
	var draft string
	if err := w.pool.QueryRow(t.Context(),
		`select draft_id::text from agent_seals where seal_id = $1::uuid`, seal).Scan(&draft); err != nil {
		t.Fatalf("the seal's draft: %v", err)
	}
	return draft
}

func (w *experimentWorld) opening(t *testing.T, run string) (domain.Step, domain.RunStartedPayload) {
	t.Helper()
	steps, err := w.store.Read(t.Context(), domain.RunID(run), domain.FirstSeq)
	if err != nil || len(steps) == 0 {
		t.Fatalf("read %s: %v", run, err)
	}
	var started domain.RunStartedPayload
	if err := json.Unmarshal(steps[0].Payload, &started); err != nil {
		t.Fatal(err)
	}
	return steps[0], started
}

func auditor(ctx context.Context) context.Context {
	return auth.WithPrincipal(ctx, domain.Principal{
		ID: "usr_eva", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuditor}},
	})
}

func (w *experimentWorld) pause(t *testing.T) {
	t.Helper()
	if err := spec.NewState(w.pool).SetPaused(t.Context(), w.agent, true, "usr_ops", time.Now()); err != nil {
		t.Fatalf("pause: %v", err)
	}
}

func rehearsalConflict(resp openapi.OpenRehearsalResponseObject) string {
	c, ok := resp.(openapi.OpenRehearsal409ApplicationProblemPlusJSONResponse)
	if !ok || c.Type == nil {
		return ""
	}
	return *c.Type
}

func rehearsalWorld(t *testing.T) (*experimentWorld, domain.VersionID) {
	t.Helper()
	world := newExperimentWorld(t).withDrafts()
	world.resume(t)
	return world, world.seal(t, rehearsalSeal)
}

func TestRehearsal_fromTheRoute_opensTheCandidateInItsAuthorsConversation(t *testing.T) {
	world, version := rehearsalWorld(t)
	text := "Customer asks for a refund on order 1234."

	opened, ok := world.rehearse(t, author(t.Context()), "rehearsal-key-1", rehearsalSeal, text).(openapi.OpenRehearsal201JSONResponse)
	if !ok {
		t.Fatal("the rehearsal was not opened")
	}
	step, started := world.opening(t, opened.RunId)
	thread, err := drafts.NewStore(world.pool).ThreadFor(t.Context(), world.agent, world.draftOf(t, rehearsalSeal), "usr_ana")
	if err != nil {
		t.Fatalf("ThreadFor: %v", err)
	}
	if !started.Rehearsal || !started.Simulated || started.SealID != rehearsalSeal || step.VersionID != version ||
		started.AuthoringThread != thread || started.Experiment != "" || step.OnBehalfOf != "usr_ana" {
		t.Fatalf("opened %s with %+v, want a simulated rehearsal of the seal in usr_ana's thread %s", step.VersionID, started, thread)
	}
	if !step.Labels.HasAny(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the typed case marked untrusted", step.Labels)
	}
	input, err := ledger.NewContent(world.pool).Get(t.Context(), started.InputRef)
	if err != nil || string(input) != text {
		t.Errorf("the run's input = %q (%v), want the case as typed", input, err)
	}
	if strings.Contains(started.Case, "refund") {
		t.Errorf("the ledger's case id %q carries the typed text", started.Case)
	}
	if state := world.settleCandidate(t, domain.RunID(opened.RunId)); state.Phase != engine.PhaseFinished {
		t.Errorf("phase = %v, want the rehearsal run to its end", state.Phase)
	}
}

func TestRehearsal_theSameKey_answersTheRunItOpenedOrRefusesAnotherRequest(t *testing.T) {
	world, _ := rehearsalWorld(t)
	world.seal(t, otherRouteSeal)
	ctx := author(t.Context())
	first, ok := world.rehearse(t, ctx, "rehearsal-key-2", rehearsalSeal, "case one").(openapi.OpenRehearsal201JSONResponse)
	if !ok {
		t.Fatal("the first attempt was not opened")
	}

	again, ok := world.rehearse(t, ctx, "rehearsal-key-2", rehearsalSeal, "case one").(openapi.OpenRehearsal200JSONResponse)
	if !ok || again.RunId != first.RunId {
		t.Errorf("the same request again = %+v, want the run it opened (%s)", again, first.RunId)
	}
	for name, ask := range map[string][2]string{
		"another seal": {otherRouteSeal, "case one"},
		"another case": {rehearsalSeal, "case two"},
	} {
		if code := rehearsalConflict(world.rehearse(t, ctx, "rehearsal-key-2", ask[0], ask[1])); code != "fuseone:conflict" {
			t.Errorf("%s under the same key = %q, want fuseone:conflict", name, code)
		}
	}
	world.eraseSeal(t, rehearsalSeal)
	if _, ok := world.rehearse(t, ctx, "rehearsal-key-2", rehearsalSeal, "case one").(openapi.OpenRehearsal200JSONResponse); !ok {
		t.Errorf("a retry after the seal was erased was refused, want the run it opened")
	}
}

func TestRehearsal_anotherCallersKey_opensTheirOwnRun(t *testing.T) {
	world, _ := rehearsalWorld(t)
	ana, ok := world.rehearse(t, author(t.Context()), "rehearsal-key-3", rehearsalSeal, "case one").(openapi.OpenRehearsal201JSONResponse)
	if !ok {
		t.Fatal("ana's rehearsal was not opened")
	}
	bia := auth.WithPrincipal(t.Context(), domain.Principal{
		ID: "usr_bia", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: domain.RoleAuthor}},
	})
	theirs, ok := world.rehearse(t, bia, "rehearsal-key-3", rehearsalSeal, "case one").(openapi.OpenRehearsal201JSONResponse)
	if !ok || theirs.RunId == ana.RunId {
		t.Fatalf("bia under ana's key = %+v, want a run of bia's own", theirs)
	}
	own, err := drafts.NewStore(world.pool).ThreadFor(t.Context(), world.agent, world.draftOf(t, rehearsalSeal), "usr_bia")
	if err != nil {
		t.Fatalf("ThreadFor: %v", err)
	}
	if _, started := world.opening(t, theirs.RunId); started.AuthoringThread != own {
		t.Errorf("bia's rehearsal belongs to thread %q, want bia's own %q, not the sealer's", started.AuthoringThread, own)
	}
}

func TestRehearsal_outsideTheSealedScope_isASealThatDoesNotExist(t *testing.T) {
	world, _ := rehearsalWorld(t)
	finance := auth.WithPrincipal(t.Context(), domain.Principal{
		ID: "usr_rui", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "finance"}, Role: domain.RoleAuthor}},
	})
	for name, ctx := range map[string]context.Context{"another area": finance, "an auditor of the area": auditor(t.Context())} {
		if _, ok := world.rehearse(t, ctx, "rehearsal-key-4", rehearsalSeal, "case").(openapi.OpenRehearsal404ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("%s: want 404", name)
		}
	}
	if _, ok := world.rehearse(t, author(t.Context()), "rehearsal-key-4", "1f2e3d4c-5b6a-4978-8695-a4b3c2d1e0f9", "case").(openapi.OpenRehearsal404ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("a seal nobody wrote: want 404")
	}
}

func TestRehearsal_aCaseThatIsEmptyOversizedOrNotText_isRefused(t *testing.T) {
	world, _ := rehearsalWorld(t)
	for name, text := range map[string]string{
		"empty": "", "blank": " \n\t", "oversized": strings.Repeat("a", 32<<10+1), "not UTF-8": "caso \xff",
	} {
		if _, ok := world.rehearse(t, author(t.Context()), "rehearsal-key-5", rehearsalSeal, text).(openapi.OpenRehearsal400ApplicationProblemPlusJSONResponse); !ok {
			t.Errorf("%s: want 400", name)
		}
	}
	if _, ok := world.rehearse(t, author(t.Context()), "rehearsal-key-5", rehearsalSeal, strings.Repeat("a", 32<<10)).(openapi.OpenRehearsal201JSONResponse); !ok {
		t.Errorf("a case of exactly the limit: want it opened")
	}
}

func (w *experimentWorld) rehearsalsOpened(t *testing.T) int {
	t.Helper()
	var runs int
	if err := w.pool.QueryRow(t.Context(), `select count(*) from runs where seal_id is not null`).Scan(&runs); err != nil {
		t.Fatalf("count the runs: %v", err)
	}
	return runs
}

func TestRehearsal_onAnErasedSeal_opensNothing(t *testing.T) {
	world, _ := rehearsalWorld(t)
	world.eraseSeal(t, rehearsalSeal)
	if code := rehearsalConflict(world.rehearse(t, author(t.Context()), "rehearsal-key-6", rehearsalSeal, "case")); code != "fuseone:candidate-unreadable" {
		t.Errorf("an erased seal = %q, want fuseone:candidate-unreadable", code)
	}
	if n := world.rehearsalsOpened(t); n != 0 {
		t.Errorf("runs opened = %d, want none", n)
	}
}

func TestRehearsal_ofAPausedAgent_opensNothing(t *testing.T) {
	world, _ := rehearsalWorld(t)
	world.pause(t)
	if code := rehearsalConflict(world.rehearse(t, author(t.Context()), "rehearsal-key-7", rehearsalSeal, "case")); code != "fuseone:conflict" {
		t.Errorf("a paused agent = %q, want fuseone:conflict", code)
	}
	if n := world.rehearsalsOpened(t); n != 0 {
		t.Errorf("runs opened = %d, want none", n)
	}
}

// The seal's bytes are read inside the agent's fence: an erasure holding it is waited for,
// and the rehearsal then finds the tombstone and opens nothing.
func TestRehearsal_whileAnErasureHoldsTheFence_waitsAndFindsTheTombstone(t *testing.T) {
	world, _ := rehearsalWorld(t)
	ctx := context.Background()
	entered, release, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		erased <- eraseCandidate(ctx, world.pool, regression.NewStore(world.pool), world, rehearsalSeal, entered, release)
	}()
	<-entered

	opened := make(chan openapi.OpenRehearsalResponseObject, 1)
	go func() { opened <- world.rehearse(t, author(ctx), "rehearsal-key-8", rehearsalSeal, "case") }()
	select {
	case resp := <-opened:
		t.Fatalf("the rehearsal did not wait for the erasure's fence: %T", resp)
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	if err := <-erased; err != nil {
		t.Fatalf("erase: %v", err)
	}
	if code := rehearsalConflict(<-opened); code != "fuseone:candidate-unreadable" {
		t.Fatalf("after the erasure = %q, want fuseone:candidate-unreadable", code)
	}
	if n := world.rehearsalsOpened(t); n != 0 {
		t.Errorf("runs opened = %d, want none", n)
	}
}
