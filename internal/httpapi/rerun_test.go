package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/trigger"
)

/*
Running again from a recorded input (#16).

Not Replay: Replay reads a run and executes nothing. This opens a new run whose
input is the one a recorded run was opened with, read on the server, and every
test here is about what that new run does not get from its source — its
approvals, its ledger, its scope — and the one thing it must keep: how far its
input is trusted.
*/

const recordedAsk = `{"text":"cancela o pedido 42","asked_by":"U123"}`

type rerunFixture struct {
	store   *ledger.Memory
	content *engine.MemoryContent
	server  *Server
	source  domain.RunID
}

// recordedFromChannel opens the source the way a channel ask is opened: an
// untrusted input, a conversation it arrived in, a stored body.
func recordedFromChannel(t *testing.T) rerunFixture {
	t.Helper()
	store, content := ledger.NewMemory(), engine.NewMemoryContent()
	agents := withHistory()
	opened, err := trigger.NewOpener(store, agents, fixedAt{t: time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)}).
		WithContent(content).
		Open(context.Background(), trigger.Request{
			Agent: "triage", IdemKey: "channel:ask-1", Trigger: "channel",
			Input:  []byte(recordedAsk),
			Labels: domain.NewLabels(domain.LabelUntrusted),
			Origin: &domain.RunOrigin{Channel: "slack-cx", Conversation: "C1", Message: "m1"},
		})
	if err != nil {
		t.Fatalf("open the source: %v", err)
	}
	server := NewServer(store, "test").WithAgents(agents).WithContent(content).
		WithClock(fixedAt{t: time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)})
	return rerunFixture{store: store, content: content, server: server, source: opened.RunID}
}

func withHistory() *fakeDetail {
	scope := domain.Scope{Company: "acme", Area: "cx"}
	return &fakeDetail{versions: []domain.AgentSummary{
		{ID: "triage", VersionID: "v2", Scope: scope, Latest: true},
		{ID: "triage", VersionID: "v1", Scope: scope},
	}}
}

func rerunOf(source domain.RunID, key string) openapi.RunFromInputRequestObject {
	return openapi.RunFromInputRequestObject{
		RunId:  string(source),
		Params: openapi.RunFromInputParams{IdempotencyKey: key},
		Body:   &openapi.RunFromInputJSONRequestBody{},
	}
}

// opened asserts a new run was opened and returns its opening step.
func (f rerunFixture) opened(t *testing.T, resp openapi.RunFromInputResponseObject) (openapi.Run, domain.Step) {
	t.Helper()
	created, ok := resp.(openapi.RunFromInput201JSONResponse)
	if !ok {
		t.Fatalf("response = %#v, want a new run", resp)
	}
	steps, err := f.store.Read(context.Background(), domain.RunID(created.RunId), domain.FirstSeq)
	if err != nil {
		t.Fatalf("read the new run: %v", err)
	}
	return openapi.Run(created), steps[0]
}

func (f rerunFixture) runs(t *testing.T) int {
	t.Helper()
	runs, err := f.store.Runs(context.Background())
	if err != nil {
		t.Fatalf("Runs: %v", err)
	}
	return len(runs)
}

func TestRunFromInput_sameScope_opensANewRunSeededWithTheRecordedInput(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)

	resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), rerunOf(f.source, "again-0001"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	run, first := f.opened(t, resp)

	if run.RunId == string(f.source) {
		t.Fatal("the new run reuses the source's id")
	}
	var payload domain.RunStartedPayload
	decodeJSON(t, first.Payload, &payload)
	got, err := f.content.Get(context.Background(), payload.InputRef)
	if err != nil {
		t.Fatalf("read the new input: %v", err)
	}
	if string(got) != recordedAsk {
		t.Errorf("input = %q, want the recorded one", got)
	}
	// Stored under the new run, so erasing either run never reaches the other.
	if payload.InputRef == f.sourceInputRef(t) {
		t.Error("the new run points at the source's content rather than its own copy")
	}
}

func TestRunFromInput_namesTheSourceByIDOnTheTrailAndTheRun(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)

	resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), rerunOf(f.source, "again-0002"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	run, first := f.opened(t, resp)

	var payload domain.RunStartedPayload
	decodeJSON(t, first.Payload, &payload)
	if payload.SourceRun != f.source {
		t.Errorf("trail names source %q, want %q", payload.SourceRun, f.source)
	}
	if run.SourceRunId == nil || *run.SourceRunId != string(f.source) {
		t.Errorf("run.sourceRunId = %v, want %q", run.SourceRunId, f.source)
	}
}

func TestRunFromInput_fromAnotherScope_answersExactlyAsAnAbsentRun(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	outsider := inArea("marketing", domain.RoleAuthor)

	foreign, err := f.server.RunFromInput(outsider, rerunOf(f.source, "again-0003"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	missing, err := f.server.RunFromInput(outsider, rerunOf("run_nobody_1_0000", "again-0004"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}

	// The same answer a run that never existed gets, so the refusal says
	// nothing about the area the run belongs to.
	a, okA := foreign.(openapi.RunFromInput404ApplicationProblemPlusJSONResponse)
	b, okB := missing.(openapi.RunFromInput404ApplicationProblemPlusJSONResponse)
	if !okA || !okB {
		t.Fatalf("responses = %T and %T, want both absent", foreign, missing)
	}
	if *a.Type != *b.Type || a.Title != b.Title || *a.Detail != string(f.source) {
		t.Errorf("foreign answer %+v differs from absent answer %+v", a, b)
	}
	if n := f.runs(t); n != 1 {
		t.Errorf("the ledger holds %d runs, want only the source", n)
	}
}

func TestRunFromInput_readerWhoCannotStartRuns_isRefused(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)

	resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuditor), rerunOf(f.source, "again-0005"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	if _, refused := resp.(openapi.RunFromInput403ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 403", resp)
	}
	if n := f.runs(t); n != 1 {
		t.Errorf("the ledger holds %d runs, want only the source", n)
	}
}

func TestRunFromInput_erasedInput_failsClosedWithAStableCodeAndOpensNothing(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	if _, err := f.content.Erase(context.Background(), "run", string(f.source), "subject request"); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), rerunOf(f.source, "again-0006"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	refused, ok := resp.(openapi.RunFromInput409ApplicationProblemPlusJSONResponse)
	if !ok {
		t.Fatalf("response = %T, want 409", resp)
	}
	if refused.Type == nil || *refused.Type != "fuseone:source-input-erased" {
		t.Errorf("type = %v, want fuseone:source-input-erased", refused.Type)
	}
	if refused.Status != http.StatusConflict {
		t.Errorf("status = %d, want 409", refused.Status)
	}
	if n := f.runs(t); n != 1 {
		t.Errorf("the ledger holds %d runs, want nothing opened", n)
	}
}

func TestRunFromInput_untrustedChannelInput_staysUntrusted(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)

	resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), rerunOf(f.source, "again-0007"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	_, first := f.opened(t, resp)

	if !first.Labels.Has(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the source's untrusted label", first.Labels)
	}
	var payload domain.RunStartedPayload
	decodeJSON(t, first.Payload, &payload)
	// The trigger is what the input is, and the transcript reads channel input
	// differently: relabelled, the routing envelope would reach the model.
	if payload.Trigger != "channel" {
		t.Errorf("trigger = %q, want the source's", payload.Trigger)
	}
	// Not the conversation, though: an operator's rerun does not answer in
	// somebody's thread.
	if payload.Origin != nil {
		t.Errorf("origin = %+v, want none carried", payload.Origin)
	}
}

func TestRunFromInput_aPriorApprovalInTheSource_authorisesNothingInTheNewRun(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	f.approveInSource(t)

	resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), rerunOf(f.source, "again-0008"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	run, _ := f.opened(t, resp)

	steps, err := f.store.Read(context.Background(), domain.RunID(run.RunId), domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	state, err := engine.Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	if len(steps) != 1 || state.Approved != nil || state.PendingApproval != nil {
		t.Errorf("new run starts with %d steps, approved %+v; want a bare opening", len(steps), state.Approved)
	}
	if state.Reserved.Micros != 0 || state.Spent.Micros != 0 {
		t.Errorf("new run inherits money: reserved %d spent %d", state.Reserved.Micros, state.Spent.Micros)
	}
}

func TestRunFromInput_version(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		asked *string
		want  string
	}{
		{name: "defaults to the version published now", want: "v2"},
		{name: "honours an explicit published version", asked: ptr("v1"), want: "v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := recordedFromChannel(t)
			req := rerunOf(f.source, "again-"+tc.want+"-0009")
			req.Body.VersionId = tc.asked

			resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), req)
			if err != nil {
				t.Fatalf("RunFromInput: %v", err)
			}
			if run, _ := f.opened(t, resp); run.VersionId != tc.want {
				t.Errorf("version = %q, want %q", run.VersionId, tc.want)
			}
		})
	}
}

func TestRunFromInput_aVersionNeverPublished_isRefusedAndOpensNothing(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	req := rerunOf(f.source, "again-0010")
	req.Body.VersionId = ptr("v9")

	resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), req)
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	if _, refused := resp.(openapi.RunFromInput400ApplicationProblemPlusJSONResponse); !refused {
		t.Fatalf("response = %T, want 400", resp)
	}
	if n := f.runs(t); n != 1 {
		t.Errorf("the ledger holds %d runs, want nothing opened", n)
	}
}

func TestRunFromInput_leavesTheSourceLedgerExactlyAsItWas(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	f.approveInSource(t)
	before := f.sourceSteps(t)

	if _, err := f.server.RunFromInput(inArea("cx", domain.RoleAuthor), rerunOf(f.source, "again-0011")); err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}

	after := f.sourceSteps(t)
	if !slices.EqualFunc(before, after, func(a, b domain.Step) bool { return bytes.Equal(a.Hash, b.Hash) }) {
		t.Errorf("source chain changed: %d steps before, %d after", len(before), len(after))
	}
}

func TestRunFromInput_sameKeyTwice_answersWithTheRunItAlreadyOpened(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	ctx := inArea("cx", domain.RoleAuthor)

	first, err := f.server.RunFromInput(ctx, rerunOf(f.source, "again-0012"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	opened, _ := f.opened(t, first)
	second, err := f.server.RunFromInput(ctx, rerunOf(f.source, "again-0012"))
	if err != nil {
		t.Fatalf("RunFromInput again: %v", err)
	}
	again, ok := second.(openapi.RunFromInput200JSONResponse)
	if !ok || again.RunId != opened.RunId {
		t.Fatalf("second answer = %#v, want the run %q", second, opened.RunId)
	}
}

func TestRunFromInput_retryAfterTheSourceInputWasErased_answersWithTheRunAlreadyOpened(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	ctx := inArea("cx", domain.RoleAuthor)

	first, err := f.server.RunFromInput(ctx, rerunOf(f.source, "again-0013"))
	if err != nil {
		t.Fatalf("RunFromInput: %v", err)
	}
	opened, _ := f.opened(t, first)
	// The answer was lost, and retention ran before the caller retried. The
	// intention was already fulfilled; the retry must reach that run rather
	// than be judged again against content that is now gone.
	if _, err := f.content.Erase(context.Background(), "run", string(f.source), "retention"); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	second, err := f.server.RunFromInput(ctx, rerunOf(f.source, "again-0013"))
	if err != nil {
		t.Fatalf("RunFromInput again: %v", err)
	}
	again, ok := second.(openapi.RunFromInput200JSONResponse)
	if !ok || again.RunId != opened.RunId {
		t.Fatalf("retry answer = %#v, want the run %q", second, opened.RunId)
	}
}

func TestRunFromInput_readerWhoCannotStartRuns_learnsNothingAboutVersions(t *testing.T) {
	t.Parallel()
	f := recordedFromChannel(t)
	want := forbidden(domain.PermRunTrigger, domain.Scope{Company: "acme", Area: "cx"})

	// Published, never published, and none at all must read the same to a
	// caller who may not start runs: a 400 for one and a 403 for another
	// would let an auditor enumerate the agent's version history.
	for _, asked := range []*string{ptr("v1"), ptr("v9"), nil} {
		req := rerunOf(f.source, "again-0014")
		req.Body.VersionId = asked

		resp, err := f.server.RunFromInput(inArea("cx", domain.RoleAuditor), req)
		if err != nil {
			t.Fatalf("RunFromInput: %v", err)
		}
		refused, ok := resp.(openapi.RunFromInput403ApplicationProblemPlusJSONResponse)
		if !ok {
			t.Fatalf("version %q: response = %T, want 403", deref(asked), resp)
		}
		if *refused.Detail != *want.Detail {
			t.Errorf("version %q: detail %q, want %q", deref(asked), *refused.Detail, *want.Detail)
		}
	}
	if n := f.runs(t); n != 1 {
		t.Errorf("the ledger holds %d runs, want only the source", n)
	}
}

func (f rerunFixture) sourceSteps(t *testing.T) []domain.Step {
	t.Helper()
	steps, err := f.store.Read(context.Background(), f.source, domain.FirstSeq)
	if err != nil {
		t.Fatalf("read the source: %v", err)
	}
	return steps
}

func (f rerunFixture) sourceInputRef(t *testing.T) string {
	t.Helper()
	var payload domain.RunStartedPayload
	decodeJSON(t, f.sourceSteps(t)[0].Payload, &payload)
	return payload.InputRef
}

// approveInSource records a human approval in the source run.
func (f rerunFixture) approveInSource(t *testing.T) {
	t.Helper()
	scope := domain.Scope{Company: "acme", Area: "cx"}
	for _, step := range []domain.Step{
		{RunID: f.source, Kind: domain.StepApprovalRequested, Scope: scope, AgentID: "triage", VersionID: "v2",
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{Tool: "crm.cancel", Rule: "taint"})},
		{RunID: f.source, Kind: domain.StepApprovalDecided, Scope: scope, AgentID: "triage", VersionID: "v2",
			IdemKey: domain.ApprovalDecisionKey(f.source, 2),
			Payload: mustPayload(t, domain.ApprovalDecidedPayload{Approved: true, By: "ana", AtSeq: 2})},
	} {
		if _, err := f.store.Append(context.Background(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
}

func decodeJSON(t *testing.T, raw []byte, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
