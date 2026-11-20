package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/auth"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
	"github.com/fuseone/agents/internal/spec"
	"github.com/fuseone/agents/internal/tools"
)

/*
Acting in a live rehearsal through the HTTP routes (NT-018 phase 4, release 4b).

The rehearsal's calls reach no tool: each parks at world_awaited, and its author answers
for the world. The answer becomes the call's result, untrusted and named back to the call.
An action the Gate holds waits for the author's decision, recorded as rehearsal_decided and
never as a real approval. The author can end the rehearsal wherever it stands, and it never
ends holding a reservation. Each route acts only on a rehearsal, only at the step it names,
and only onto that step: a second answer to one wait loses.
*/

const actSeal = "4a5b6c7d-8e9f-4a0b-9c1d-2e3f4a5b6c7d"

// actTemplate is a candidate that reads a customer and then writes a note: a read the
// Gate lets through and a write it holds for a person.
const actTemplate = `---
id: %s
name: Triage
company: acme
area: cx
provider: elsewhere
model: sealed-model
tools:
  - crm.lookup
  - crm.note
budget:
  micros: 500000
  steps: 20
---

Look the customer up, then leave a note.
`

// countingSession is a tool server that counts every call that reaches it.
type countingSession struct {
	testSession
	called atomic.Int32
}

func (s *countingSession) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.called.Add(1)
	return nil, errors.New("a rehearsal reached a tool")
}

type actWorld struct {
	*experimentWorld
	crm   *countingSession
	draft drafts.Opened
	run   string
}

func newActWorld(t *testing.T, calls ...string) *actWorld {
	t.Helper()
	world := newExperimentWorld(t).withDrafts()
	world.resume(t)
	crm := &countingSession{testSession: testSession{tools: []*mcp.Tool{{Name: "lookup"}, {Name: "note"}}}}
	catalog := tools.NewCatalog(ledger.NewContent(world.pool))
	if err := catalog.AddServer(t.Context(), "crm", crm, nil); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	for tool, effect := range map[domain.ToolID]domain.Effect{"crm.lookup": domain.EffectRead, "crm.note": domain.EffectWrite} {
		if err := catalog.Classify(domain.ToolClassification{Tool: tool, Effect: effect}); err != nil {
			t.Fatalf("Classify %s: %v", tool, err)
		}
	}
	world.catalog = catalog
	world.resolver = spec.NewResolver(spec.NewRegistry(world.pool), world.providers, catalog)
	world.tested.mu.Lock()
	world.tested.calls = calls
	world.tested.mu.Unlock()
	draft, err := drafts.NewStore(world.pool).Create(t.Context(), world.agent, nil, anaCaller())
	if err != nil {
		t.Fatalf("open the draft: %v", err)
	}
	world.sealDefinition(t, actSeal, draft.DraftID, fmt.Sprintf(actTemplate, world.agent))
	w := &actWorld{experimentWorld: world, crm: crm, draft: draft}
	opened := world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "act-key-1", actSeal, "Ana asks about her order.")
	if opened.code != http.StatusCreated {
		t.Fatalf("open the rehearsal = %d: %s", opened.code, opened.raw)
	}
	w.run = opened.run
	return w
}

func anaCaller() drafts.Caller {
	return drafts.Caller{By: "usr_ana", Allow: func(domain.Scope) bool { return true }}
}

func (w *actWorld) send(t *testing.T, caller domain.Principal, path string, body []byte) rehearsalReply {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/runs/"+w.run+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithPrincipal(req.Context(), caller))
	rec := httptest.NewRecorder()
	apiHandlerFor(w.server).ServeHTTP(rec, req)
	var out struct{ RunId, Type string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rehearsalReply{code: rec.Code, run: out.RunId, problem: out.Type, raw: rec.Body.String()}
}

func (w *actWorld) answer(t *testing.T, caller domain.Principal, atSeq int64, text string) rehearsalReply {
	t.Helper()
	body, _ := json.Marshal(openapi.WorldAnswer{AtSeq: atSeq, Answer: text})
	return w.send(t, caller, "/world", body)
}

func (w *actWorld) decide(t *testing.T, caller domain.Principal, atSeq int64, approved bool) rehearsalReply {
	t.Helper()
	body, _ := json.Marshal(openapi.RehearsalDecision{AtSeq: atSeq, Approved: approved})
	return w.send(t, caller, "/rehearsal-decisions", body)
}

func (w *actWorld) abandon(t *testing.T, caller domain.Principal) rehearsalReply {
	t.Helper()
	return w.send(t, caller, "/rehearsal-abandon", nil)
}

func (w *actWorld) steps(t *testing.T) []domain.Step {
	t.Helper()
	steps, err := w.store.Read(t.Context(), domain.RunID(w.run), domain.FirstSeq)
	if err != nil {
		t.Fatalf("read %s: %v", w.run, err)
	}
	return steps
}

func (w *actWorld) head(t *testing.T) domain.Step {
	t.Helper()
	steps := w.steps(t)
	return steps[len(steps)-1]
}

func (w *actWorld) until(t *testing.T, phase engine.Phase) engine.State {
	t.Helper()
	return w.advanceUntil(t, domain.RunID(w.run), func(s engine.State) bool { return s.Phase == phase })
}

func ana() domain.Principal { return inCx("usr_ana", domain.RoleAuthor) }

func kinds(steps []domain.Step, kind domain.StepKind) []domain.Step {
	var out []domain.Step
	for _, s := range steps {
		if s.Kind == kind {
			out = append(out, s)
		}
	}
	return out
}

func TestRehearsal_aFullJourney_answersTwoCallsDecidesOneActionAndFinishes(t *testing.T) {
	w := newActWorld(t, "crm__lookup", "crm__note")

	w.until(t, engine.PhaseAwaitingWorld)
	first := w.head(t)
	if got := w.answer(t, ana(), first.Seq, `{"customer":"Ana","orders":1}`); got.code != http.StatusOK {
		t.Fatalf("answer the lookup = %d: %s", got.code, got.raw)
	}
	held := w.until(t, engine.PhaseAwaitingApproval)
	if got := w.decide(t, ana(), held.PendingApproval.AtSeq, true); got.code != http.StatusOK {
		t.Fatalf("decide the note = %d: %s", got.code, got.raw)
	}
	w.until(t, engine.PhaseAwaitingWorld)
	second := w.head(t)
	if got := w.answer(t, ana(), second.Seq, `{"noted":true}`); got.code != http.StatusOK {
		t.Fatalf("answer the note = %d: %s", got.code, got.raw)
	}
	end := w.until(t, engine.PhaseFinished)

	steps := w.steps(t)
	called, returned, supplied := kinds(steps, domain.StepToolCalled), kinds(steps, domain.StepToolReturned), kinds(steps, domain.StepWorldSupplied)
	if len(called) != 2 || len(returned) != 2 || len(supplied) != 2 {
		t.Fatalf("called %d, returned %d, supplied %d; want two of each", len(called), len(returned), len(supplied))
	}
	for i := range returned {
		var result domain.ToolReturnedPayload
		var answer domain.WorldSuppliedPayload
		_ = json.Unmarshal(returned[i].Payload, &result)
		_ = json.Unmarshal(supplied[i].Payload, &answer)
		if result.ForSeq != called[i].Seq || result.ResultRef != answer.Ref || answer.By != "usr_ana" ||
			!returned[i].Labels.HasAny(domain.LabelUntrusted) {
			t.Errorf("result %d = %+v (labels %v), want the answer %s to call %d, untrusted", i, result, returned[i].Labels, answer.Ref, called[i].Seq)
		}
	}
	if real := kinds(steps, domain.StepApprovalDecided); len(real) != 0 || len(kinds(steps, domain.StepRehearsalDecided)) != 1 {
		t.Errorf("approval_decided %d, rehearsal_decided %d; want none and one", len(real), len(kinds(steps, domain.StepRehearsalDecided)))
	}
	if n := w.crm.called.Load(); n != 0 {
		t.Errorf("the tool server was called %d times by a rehearsal", n)
	}
	if end.Reserved.Micros != 0 || end.Reserved.Tokens != 0 {
		t.Errorf("the rehearsal ended holding %+v reserved", end.Reserved)
	}
}

// A run that is not a rehearsal, a wait the run does not stand at, an empty or oversized
// answer and a caller outside the scope are each refused, and none of them writes a step.
func TestRehearsal_anAnswerForTheWorld_isRefusedAnywhereButTheWaitItNames(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	w.until(t, engine.PhaseAwaitingWorld)
	at := w.head(t).Seq
	before := len(w.steps(t))

	backslash := string(rune(92))
	cases := map[string]struct {
		got  func() rehearsalReply
		code int
	}{
		"another step":           {func() rehearsalReply { return w.answer(t, ana(), at-1, "{}") }, http.StatusConflict},
		"a step not yet reached": {func() rehearsalReply { return w.answer(t, ana(), at+1, "{}") }, http.StatusConflict},
		"an empty answer":        {func() rehearsalReply { return w.answer(t, ana(), at, "") }, http.StatusBadRequest},
		"an oversized answer":    {func() rehearsalReply { return w.answer(t, ana(), at, strings.Repeat("a", 256<<10+1)) }, http.StatusBadRequest},
		"not UTF-8": {func() rehearsalReply {
			return w.send(t, ana(), "/world", []byte(fmt.Sprintf("{\"atSeq\":%d,\"answer\":\"\xff\"}", at)))
		}, http.StatusBadRequest},
		"an unpaired surrogate": {func() rehearsalReply {
			return w.send(t, ana(), "/world", []byte(fmt.Sprintf(`{"atSeq":%d,"answer":"%sud800"}`, at, backslash)))
		}, http.StatusBadRequest},
		"an auditor of the area": {func() rehearsalReply { return w.answer(t, inCx("usr_eva", domain.RoleAuditor), at, "{}") }, http.StatusNotFound},
	}
	for name, c := range cases {
		if got := c.got(); got.code != c.code {
			t.Errorf("%s = %d: %s, want %d", name, got.code, got.raw, c.code)
		}
	}
	if after := len(w.steps(t)); after != before {
		t.Errorf("refused answers wrote %d steps", after-before)
	}
	if got := w.answer(t, ana(), at, strings.Repeat("a", 256<<10)); got.code != http.StatusOK {
		t.Errorf("an answer of exactly the limit = %d: %s, want it taken", got.code, got.raw)
	}
}

func TestRehearsal_theActingRoutes_refuseARunThatIsNotARehearsal(t *testing.T) {
	w := newActWorld(t)
	version := w.sealFrom(t, rehearsalSeal, "")
	experiment := domain.RunID("run_triage_experiment_on_seal")
	w.openOnSeal(t, experiment, version, rehearsalSeal)
	w.run = string(experiment)
	before := len(w.steps(t))
	for name, got := range map[string]rehearsalReply{
		"an answer":  w.answer(t, ana(), 1, "{}"),
		"a decision": w.decide(t, ana(), 1, true),
		"abandoning": w.abandon(t, ana()),
	} {
		if got.code != http.StatusConflict {
			t.Errorf("%s on an experiment's run = %d: %s, want 409", name, got.code, got.raw)
		}
	}
	if after := len(w.steps(t)); after != before {
		t.Errorf("refusals wrote %d steps on an experiment's run", after-before)
	}
}

// Two answers to one wait: one is taken and the other is told the wait was answered.
func TestRehearsal_twoAnswersToOneWait_onlyOneIsTaken(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	w.until(t, engine.PhaseAwaitingWorld)
	at := w.head(t).Seq

	const attempts = 6
	replies := make([]rehearsalReply, attempts)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Go(func() {
			<-start
			replies[i] = w.answer(t, ana(), at, fmt.Sprintf(`{"answer":%d}`, i))
		})
	}
	close(start)
	wg.Wait()
	var kept int
	if err := w.pool.QueryRow(t.Context(), `select count(*) from run_content where owner_kind = 'run' and run_id = $1 and seq = $2`,
		w.run, at+1).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Errorf("answers kept = %d, want only the one a step names", kept)
	}
	taken := 0
	for _, r := range replies {
		switch r.code {
		case http.StatusOK:
			taken++
		case http.StatusConflict:
		default:
			t.Errorf("an answer = %d: %s", r.code, r.raw)
		}
	}
	if supplied := kinds(w.steps(t), domain.StepWorldSupplied); taken != 1 || len(supplied) != 1 {
		t.Errorf("taken %d, world_supplied %d; want one of each", taken, len(supplied))
	}
}

// The author decides where the Gate held an action, and only there; the real approval
// route still refuses a rehearsal, and nothing a rehearsal does is a real approval.
func TestRehearsal_aDecision_answersOnlyTheHeldRequestAndIsNeverARealApproval(t *testing.T) {
	w := newActWorld(t, "crm__lookup", "crm__note")
	w.until(t, engine.PhaseAwaitingWorld)
	if got := w.decide(t, ana(), w.head(t).Seq, true); got.code != http.StatusConflict {
		t.Errorf("a decision while waiting for the world = %d, want 409", got.code)
	}
	if got := w.answer(t, ana(), w.head(t).Seq, "{}"); got.code != http.StatusOK {
		t.Fatalf("answer = %d: %s", got.code, got.raw)
	}
	held := w.until(t, engine.PhaseAwaitingApproval)
	at := held.PendingApproval.AtSeq

	real, err := w.server.DecideApproval(auth.WithPrincipal(t.Context(), inCx("usr_ana", domain.RoleAdmin)),
		openapi.DecideApprovalRequestObject{RunId: w.run, Body: &openapi.DecideApprovalJSONRequestBody{AtSeq: at, Approved: true}})
	if _, refused := real.(openapi.DecideApproval409ApplicationProblemPlusJSONResponse); err != nil || !refused {
		t.Errorf("the real approval route on a rehearsal = %T (%v), want 409", real, err)
	}
	if got := w.decide(t, ana(), at-1, true); got.code != http.StatusConflict {
		t.Errorf("a decision on another step = %d, want 409", got.code)
	}
	if got := w.decide(t, inCx("usr_eva", domain.RoleAuditor), at, true); got.code != http.StatusNotFound {
		t.Errorf("an auditor's decision = %d, want 404", got.code)
	}
	if got := w.decide(t, ana(), at, false); got.code != http.StatusOK {
		t.Fatalf("a refusal = %d: %s", got.code, got.raw)
	}
	if again := w.decide(t, ana(), at, true); again.code != http.StatusConflict {
		t.Errorf("a second decision on the same request = %d, want 409", again.code)
	}
	steps := w.steps(t)
	decided := kinds(steps, domain.StepRehearsalDecided)
	var p domain.RehearsalDecidedPayload
	if len(decided) == 1 {
		_ = json.Unmarshal(decided[0].Payload, &p)
	}
	if len(decided) != 1 || p.Approved || p.AtSeq != at || p.By != "usr_ana" || len(kinds(steps, domain.StepApprovalDecided)) != 0 {
		t.Errorf("rehearsal_decided %d (%+v), approval_decided %d; want one refusal by usr_ana at %d and no real approval",
			len(decided), p, len(kinds(steps, domain.StepApprovalDecided)), at)
	}
}

// Abandoning ends the rehearsal wherever it stands, undoes nothing, and releases what it
// held reserved first: an abandoned rehearsal never ends holding a reservation.
func TestRehearsal_abandoned_endsFailedWithNothingReserved(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	waiting := w.until(t, engine.PhaseAwaitingWorld)
	at := w.head(t).Seq
	_ = waiting
	if got := w.abandon(t, inCx("usr_eva", domain.RoleAuditor)); got.code != http.StatusNotFound {
		t.Errorf("an auditor abandoning = %d, want 404", got.code)
	}
	if got := w.abandon(t, ana()); got.code != http.StatusOK {
		t.Fatalf("abandon = %d: %s", got.code, got.raw)
	}
	steps := w.steps(t)
	state, err := engine.Fold(steps)
	if err != nil {
		t.Fatal(err)
	}
	last := steps[len(steps)-1]
	var p domain.AbandonedPayload
	_ = json.Unmarshal(last.Payload, &p)
	if state.Phase != engine.PhaseFailed || last.Kind != domain.StepAbandoned || p.Compensate || p.By != "usr_ana" ||
		state.Reserved.Micros != 0 || state.Reserved.Tokens != 0 {
		t.Errorf("phase %v, last %s %+v, reserved %+v; want failed, abandoned without compensating, nothing reserved",
			state.Phase, last.Kind, p, state.Reserved)
	}
	if again := w.abandon(t, ana()); again.code != http.StatusConflict {
		t.Errorf("abandoning an ended rehearsal = %d, want 409", again.code)
	}
	if got := w.answer(t, ana(), at, "{}"); got.code != http.StatusConflict {
		t.Errorf("answering an abandoned rehearsal = %d, want 409", got.code)
	}
}

// Erasing the author's conversation reaches the rehearsal's case and every answer given
// for the world, since both are owned by the run the conversation holds.
func TestRehearsal_erasingTheConversation_erasesTheAnswersForTheWorld(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	w.until(t, engine.PhaseAwaitingWorld)
	if got := w.answer(t, ana(), w.head(t).Seq, `{"customer":"Ana"}`); got.code != http.StatusOK {
		t.Fatalf("answer = %d: %s", got.code, got.raw)
	}
	if err := drafts.NewStore(w.pool).EraseThread(t.Context(), w.agent, anaCaller()); err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	supplied := kinds(w.steps(t), domain.StepWorldSupplied)
	if len(supplied) != 1 {
		t.Fatalf("world_supplied %d, want one", len(supplied))
	}
	var p domain.WorldSuppliedPayload
	_ = json.Unmarshal(supplied[0].Payload, &p)
	if _, err := ledger.NewContent(w.pool).Get(t.Context(), p.Ref); !errors.Is(err, domain.ErrContentErased) {
		t.Errorf("the answer after its conversation was erased: %v, want erased", err)
	}
}

// Any author who may publish in the rehearsal's scope may answer for it, and the answer
// says who gave it.
func TestRehearsal_anotherAuthorOfTheArea_answersAndIsNamed(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	w.until(t, engine.PhaseAwaitingWorld)
	if got := w.answer(t, inCx("usr_bia", domain.RoleAuthor), w.head(t).Seq, `{"customer":"Ana"}`); got.code != http.StatusOK {
		t.Fatalf("bia's answer = %d: %s", got.code, got.raw)
	}
	var p domain.WorldSuppliedPayload
	_ = json.Unmarshal(w.head(t).Payload, &p)
	if p.By != "usr_bia" {
		t.Errorf("the answer names %q, want usr_bia", p.By)
	}
}

// The largest answer the contract allows is taken whatever JSON spells it: 256 KiB written
// as \u escapes is six times that on the wire.
func TestRehearsal_anAnswerAtTheLimitSpelledInEscapes_isTaken(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	w.until(t, engine.PhaseAwaitingWorld)
	escape := string(rune(92)) + "u0061"
	body := fmt.Sprintf(`{"atSeq":%d,"answer":"%s"}`, w.head(t).Seq, strings.Repeat(escape, 256<<10))
	if got := w.send(t, ana(), "/world", []byte(body)); got.code != http.StatusOK {
		t.Fatalf("an answer of 256 KiB in escapes (%d bytes on the wire) = %d: %.200s", len(body), got.code, got.raw)
	}
}

/*
The answer is kept inside the agent's exclusive fence, the one erasing the conversation
takes. Held by an answer, the erasure waits and then reaches it; held by an erasure, the
answer waits for it.
*/
func TestRehearsal_anAnswerHoldingTheFence_isWaitedForAndThenErased(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	w.until(t, engine.PhaseAwaitingWorld)
	at := w.head(t).Seq
	fence := firstStalls{Store: regression.NewStore(w.pool), once: &sync.Once{},
		entered: make(chan struct{}), release: make(chan struct{})}
	w.server.WithRegressions(fence)

	answered := make(chan rehearsalReply, 1)
	go func() { answered <- w.answer(t, ana(), at, `{"customer":"Ana"}`) }()
	select {
	case <-fence.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the answer never took the agent's fence")
	}
	erased := make(chan error, 1)
	go func() { erased <- drafts.NewStore(w.pool).EraseThread(context.Background(), w.agent, anaCaller()) }()
	select {
	case err := <-erased:
		t.Fatalf("the erasure did not wait for the answer's fence: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	close(fence.release)
	if got := <-answered; got.code != http.StatusOK {
		t.Fatalf("answer = %d: %s", got.code, got.raw)
	}
	if err := <-erased; err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	var p domain.WorldSuppliedPayload
	_ = json.Unmarshal(w.head(t).Payload, &p)
	if _, err := ledger.NewContent(w.pool).Get(t.Context(), p.Ref); !errors.Is(err, domain.ErrContentErased) {
		t.Errorf("the answer after the erasure that waited for it: %v, want erased", err)
	}
}

func TestRehearsal_anAnswerWhileAnErasureHoldsTheFence_waitsForIt(t *testing.T) {
	w := newActWorld(t, "crm__lookup")
	w.until(t, engine.PhaseAwaitingWorld)
	at := w.head(t).Seq
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- regression.NewStore(w.pool).Exclusive(context.Background(), []domain.AgentID{w.agent},
			func(context.Context) error { close(held); <-release; return nil })
	}()
	<-held
	answered := make(chan rehearsalReply, 1)
	go func() { answered <- w.answer(t, ana(), at, `{"customer":"Ana"}`) }()
	select {
	case got := <-answered:
		t.Fatalf("the answer did not wait for the fence an erasure holds: %d", got.code)
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := <-answered; got.code != http.StatusOK {
		t.Errorf("answer after the fence = %d: %s", got.code, got.raw)
	}
}
