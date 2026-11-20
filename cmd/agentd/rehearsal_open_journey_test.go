package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
Opening a live rehearsal through the HTTP route (NT-018 phase 4, release 4b).

A rehearsal runs one candidate on one case the author typed, simulated, in the sealed
scope, and belongs to the author's conversation on the draft the seal was sealed from. The
route finds the seal under the caller's own authority, refuses text the JSON decoder would
rewrite, and does everything that decides or writes inside the agent's exclusive fence: the
key, the seal's bytes, the opener's refusals, then the conversation and the run. A refusal
writes nothing, two attempts under one key store one case, and an erasure of the seal or the
conversation either comes first or reaches the run.
*/

const rehearsalSeal = "9e5f6a7b-8c9d-4e0f-a1b2-c3d4e5f6a7b8"

func inCx(id string, role domain.Role) domain.Principal {
	return domain.Principal{
		ID: domain.UserID(id), Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "cx"}, Role: role}},
	}
}

// rehearsalReply is what the route answered: its status, and the run or the refusal's code.
type rehearsalReply struct {
	code         int
	run, problem string
	raw          string
}

func (w *experimentWorld) withDrafts() *experimentWorld {
	w.server = w.server.WithDrafts(drafts.NewStore(w.pool))
	return w
}

func (w *experimentWorld) rehearseRaw(t *testing.T, caller domain.Principal, key string, body []byte) rehearsalReply {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/api/v1/agents/"+string(w.agent)+"/rehearsals", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	req = req.WithContext(auth.WithPrincipal(req.Context(), caller))
	rec := httptest.NewRecorder()
	apiHandlerFor(w.server).ServeHTTP(rec, req)
	var out struct{ RunId, Type string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rehearsalReply{code: rec.Code, run: out.RunId, problem: out.Type, raw: rec.Body.String()}
}

func (w *experimentWorld) rehearse(t *testing.T, caller domain.Principal, key, seal, text string) rehearsalReply {
	t.Helper()
	body, err := json.Marshal(openapi.RehearsalRequest{SealId: seal, Case: text})
	if err != nil {
		t.Fatal(err)
	}
	return w.rehearseRaw(t, caller, key, body)
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

func (w *experimentWorld) pause(t *testing.T) {
	t.Helper()
	if err := spec.NewState(w.pool).SetPaused(t.Context(), w.agent, true, "usr_ops", time.Now()); err != nil {
		t.Fatalf("pause: %v", err)
	}
}

// written counts what a rehearsal writes: rehearsal runs, conversations, and stored cases.
func (w *experimentWorld) written(t *testing.T) (runs, threads, cases int) {
	t.Helper()
	if err := w.pool.QueryRow(t.Context(), `
		select (select count(*) from runs where seal_id is not null),
		       (select count(*) from agent_draft_threads where agent_id = $1),
		       (select count(*) from run_content where owner_kind = 'run')`, string(w.agent)).
		Scan(&runs, &threads, &cases); err != nil {
		t.Fatalf("count what was written: %v", err)
	}
	return runs, threads, cases
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

	opened := world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-1", rehearsalSeal, text)
	if opened.code != http.StatusCreated {
		t.Fatalf("status = %d: %s", opened.code, opened.raw)
	}
	step, started := world.opening(t, opened.run)
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
	if state := world.settleCandidate(t, domain.RunID(opened.run)); state.Phase != engine.PhaseFinished {
		t.Errorf("phase = %v, want the rehearsal run to its end", state.Phase)
	}
}

func TestRehearsal_theSameKey_answersTheRunItOpenedOrRefusesAnotherRequest(t *testing.T) {
	world, _ := rehearsalWorld(t)
	world.seal(t, otherRouteSeal)
	ana := inCx("usr_ana", domain.RoleAuthor)
	first := world.rehearse(t, ana, "rehearsal-key-2", rehearsalSeal, "case one")
	if first.code != http.StatusCreated {
		t.Fatalf("the first attempt = %d: %s", first.code, first.raw)
	}

	if again := world.rehearse(t, ana, "rehearsal-key-2", rehearsalSeal, "case one"); again.code != http.StatusOK || again.run != first.run {
		t.Errorf("the same request again = %d %s, want 200 with %s", again.code, again.run, first.run)
	}
	for name, ask := range map[string][2]string{
		"another seal": {otherRouteSeal, "case one"},
		"another case": {rehearsalSeal, "case two"},
	} {
		if got := world.rehearse(t, ana, "rehearsal-key-2", ask[0], ask[1]); got.code != http.StatusConflict || got.problem != "fuseone:conflict" {
			t.Errorf("%s under the same key = %d %q, want 409 fuseone:conflict", name, got.code, got.problem)
		}
	}
	world.eraseSeal(t, rehearsalSeal)
	if got := world.rehearse(t, ana, "rehearsal-key-2", rehearsalSeal, "case one"); got.code != http.StatusOK {
		t.Errorf("a retry after the seal was erased = %d: %s, want the run it opened", got.code, got.raw)
	}
}

// Attempts under one key racing past each other store one case and open one run: each
// later one finds the first's run inside the fence, before it writes anything.
func TestRehearsal_concurrentAttemptsUnderOneKey_storeOneCaseAndOpenOneRun(t *testing.T) {
	world, _ := rehearsalWorld(t)
	const attempts = 8
	replies := make([]rehearsalReply, attempts)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Go(func() {
			<-start
			replies[i] = world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-race", rehearsalSeal, "the same case")
		})
	}
	close(start)
	wg.Wait()

	created := 0
	for _, r := range replies {
		if r.code == http.StatusCreated {
			created++
		}
		if r.run != replies[0].run || (r.code != http.StatusCreated && r.code != http.StatusOK) {
			t.Errorf("an attempt answered %d with %q, want the one run %q", r.code, r.run, replies[0].run)
		}
	}
	if runs, _, cases := world.written(t); created != 1 || runs != 1 || cases != 1 {
		t.Errorf("created %d, runs %d, stored cases %d; want one of each", created, runs, cases)
	}
}

func TestRehearsal_anotherCallersKey_opensTheirOwnRunInTheirOwnThread(t *testing.T) {
	world, _ := rehearsalWorld(t)
	first := world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-3", rehearsalSeal, "case one")
	theirs := world.rehearse(t, inCx("usr_bia", domain.RoleAuthor), "rehearsal-key-3", rehearsalSeal, "case one")
	if first.code != http.StatusCreated || theirs.code != http.StatusCreated || theirs.run == first.run {
		t.Fatalf("ana %d %s, bia %d %s; want a run of bia's own", first.code, first.run, theirs.code, theirs.run)
	}
	own, err := drafts.NewStore(world.pool).ThreadFor(t.Context(), world.agent, world.draftOf(t, rehearsalSeal), "usr_bia")
	if err != nil {
		t.Fatalf("ThreadFor: %v", err)
	}
	if _, started := world.opening(t, theirs.run); started.AuthoringThread != own {
		t.Errorf("bia's rehearsal belongs to thread %q, want bia's own %q, not the sealer's", started.AuthoringThread, own)
	}
}

func TestRehearsal_outsideTheSealedScope_isASealThatDoesNotExist(t *testing.T) {
	world, _ := rehearsalWorld(t)
	finance := domain.Principal{
		ID: "usr_rui", Kind: domain.PrincipalUser,
		Grants: []domain.Grant{{Scope: domain.Scope{Company: "acme", Area: "finance"}, Role: domain.RoleAuthor}},
	}
	for name, caller := range map[string]domain.Principal{
		"another area": finance, "an auditor of the area": inCx("usr_eva", domain.RoleAuditor),
	} {
		if got := world.rehearse(t, caller, "rehearsal-key-4", rehearsalSeal, "case"); got.code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", name, got.code)
		}
	}
	nobody := "1f2e3d4c-5b6a-4978-8695-a4b3c2d1e0f9"
	if got := world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-4", nobody, "case"); got.code != http.StatusNotFound {
		t.Errorf("a seal nobody wrote = %d, want 404", got.code)
	}
}

func TestRehearsal_aCaseThatIsEmptyOversizedOrNotText_isRefused(t *testing.T) {
	world, _ := rehearsalWorld(t)
	ana := inCx("usr_ana", domain.RoleAuthor)
	for name, text := range map[string]string{
		"empty": "", "blank": " \n\t", "oversized": strings.Repeat("a", 32<<10+1),
	} {
		if got := world.rehearse(t, ana, "rehearsal-key-5", rehearsalSeal, text); got.code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, got.code)
		}
	}
	// What the JSON decoder would quietly turn into U+FFFD, sent as the bytes a client sends.
	backslash := string(rune(92))
	for name, body := range map[string]string{
		"not UTF-8":             "{\"sealId\":\"" + rehearsalSeal + "\",\"case\":\"caso \xff\"}",
		"an unpaired surrogate": `{"sealId":"` + rehearsalSeal + `","case":"caso ` + backslash + `ud800"}`,
	} {
		if got := world.rehearseRaw(t, ana, "rehearsal-key-5", []byte(body)); got.code != http.StatusBadRequest {
			t.Errorf("%s = %d: %s, want 400", name, got.code, got.raw)
		}
	}
	if runs, _, _ := world.written(t); runs != 0 {
		t.Fatalf("a refused case opened %d runs", runs)
	}
	if got := world.rehearse(t, ana, "rehearsal-key-5", rehearsalSeal, strings.Repeat("a", 32<<10)); got.code != http.StatusCreated {
		t.Errorf("a case of exactly the limit = %d: %s, want it opened", got.code, got.raw)
	}
}

// A refused rehearsal writes nothing: no run, no stored case, and no conversation opened for it.
func TestRehearsal_refused_writesNothing(t *testing.T) {
	for name, refuse := range map[string]func(*testing.T, *experimentWorld){
		"an erased seal": func(t *testing.T, w *experimentWorld) { w.eraseSeal(t, rehearsalSeal) },
		"a paused agent": func(t *testing.T, w *experimentWorld) { w.pause(t) },
	} {
		t.Run(name, func(t *testing.T) {
			world, _ := rehearsalWorld(t)
			refuse(t, world)
			got := world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-6", rehearsalSeal, "case")
			if got.code != http.StatusConflict {
				t.Fatalf("status = %d: %s, want 409", got.code, got.raw)
			}
			if runs, threads, cases := world.written(t); runs != 0 || threads != 0 || cases != 0 {
				t.Errorf("wrote %d runs, %d conversations, %d cases; want nothing", runs, threads, cases)
			}
		})
	}
}

func TestRehearsal_onAnErasedSeal_isUnreadable(t *testing.T) {
	world, _ := rehearsalWorld(t)
	world.eraseSeal(t, rehearsalSeal)
	if got := world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-7", rehearsalSeal, "case"); got.problem != "fuseone:candidate-unreadable" {
		t.Errorf("an erased seal = %d %q, want fuseone:candidate-unreadable", got.code, got.problem)
	}
}

// The seal's bytes are read inside the agent's fence: an erasure holding it is waited for,
// and the rehearsal then finds the tombstone and opens nothing.
func TestRehearsal_whileASealErasureHoldsTheFence_waitsAndFindsTheTombstone(t *testing.T) {
	world, _ := rehearsalWorld(t)
	ctx := context.Background()
	entered, release, erased := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		erased <- eraseCandidate(ctx, world.pool, regression.NewStore(world.pool), world, rehearsalSeal, entered, release)
	}()
	<-entered

	opened := make(chan rehearsalReply, 1)
	go func() {
		opened <- world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-8", rehearsalSeal, "case")
	}()
	select {
	case got := <-opened:
		t.Fatalf("the rehearsal did not wait for the erasure's fence: %d", got.code)
	case <-time.After(400 * time.Millisecond):
	}
	close(release)
	if err := <-erased; err != nil {
		t.Fatalf("erase: %v", err)
	}
	if got := <-opened; got.problem != "fuseone:candidate-unreadable" {
		t.Fatalf("after the erasure = %d %q, want fuseone:candidate-unreadable", got.code, got.problem)
	}
	if runs, threads, _ := world.written(t); runs != 0 || threads != 0 {
		t.Errorf("wrote %d runs and %d conversations on an erased seal", runs, threads)
	}
}

/*
The author erasing their conversation while a rehearsal is being opened in it: the erasure
waits for the fence the rehearsal holds, and then reaches the run, the case included. Opened
outside that fence, the run could be written after the erasure had read which runs the
conversation holds, and keep a case its author had erased.
*/
func TestRehearsal_whileItHoldsTheFence_anErasureOfTheConversationWaitsAndReachesIt(t *testing.T) {
	world := newExperimentWorld(t).withDrafts()
	world.resume(t)
	store := drafts.NewStore(world.pool)
	caller := drafts.Caller{By: "usr_ana", Allow: func(domain.Scope) bool { return true }}
	draft, err := store.Create(t.Context(), world.agent, nil, caller)
	if err != nil {
		t.Fatalf("open the draft: %v", err)
	}
	world.sealFrom(t, rehearsalSeal, draft.DraftID)
	fence := stallingFence{Store: regression.NewStore(world.pool), entered: make(chan struct{}), release: make(chan struct{})}
	world.server.WithRegressions(fence)

	opened := make(chan rehearsalReply, 1)
	go func() {
		opened <- world.rehearse(t, inCx("usr_ana", domain.RoleAuthor), "rehearsal-key-9", rehearsalSeal, "a case to erase")
	}()
	select {
	case <-fence.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the rehearsal never took the agent's exclusive fence")
	}
	erased := make(chan error, 1)
	go func() { erased <- store.EraseThread(context.Background(), world.agent, caller) }()
	select {
	case err := <-erased:
		t.Fatalf("the erasure did not wait for the rehearsal's fence: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	close(fence.release)
	got := <-opened
	if got.code != http.StatusCreated {
		t.Fatalf("the rehearsal = %d: %s", got.code, got.raw)
	}
	if err := <-erased; err != nil {
		t.Fatalf("EraseThread: %v", err)
	}
	_, started := world.opening(t, got.run)
	if _, err := ledger.NewContent(world.pool).Get(t.Context(), started.InputRef); !errors.Is(err, domain.ErrContentErased) {
		t.Errorf("the rehearsal's case after its conversation was erased: %v, want erased", err)
	}
}

// firstStalls holds the first caller inside the fence it asked for, shared or exclusive,
// until released; every later caller takes its own fence as asked.
type firstStalls struct {
	*regression.Store
	once             *sync.Once
	entered, release chan struct{}
}

func (f firstStalls) stall(inside func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		f.once.Do(func() { close(f.entered); <-f.release })
		return inside(ctx)
	}
}

func (f firstStalls) Hold(ctx context.Context, agent domain.AgentID, inside func(context.Context) error) error {
	return f.Store.Hold(ctx, agent, f.stall(inside))
}

func (f firstStalls) Exclusive(ctx context.Context, agents []domain.AgentID, inside func(context.Context) error) error {
	return f.Store.Exclusive(ctx, agents, f.stall(inside))
}

// A second attempt under the key waits for the first to leave the fence, whatever fence
// the route takes, and then answers with the first's run: never a second case stored.
func TestRehearsal_aSecondAttemptUnderTheKey_waitsForTheFirstInsideTheFence(t *testing.T) {
	world, _ := rehearsalWorld(t)
	fence := firstStalls{Store: regression.NewStore(world.pool), once: &sync.Once{},
		entered: make(chan struct{}), release: make(chan struct{})}
	world.server.WithRegressions(fence)
	ana := inCx("usr_ana", domain.RoleAuthor)

	first := make(chan rehearsalReply, 1)
	go func() { first <- world.rehearse(t, ana, "rehearsal-key-10", rehearsalSeal, "the same case") }()
	select {
	case <-fence.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first attempt never took the agent's fence")
	}
	second := make(chan rehearsalReply, 1)
	go func() { second <- world.rehearse(t, ana, "rehearsal-key-10", rehearsalSeal, "the same case") }()
	select {
	case got := <-second:
		t.Fatalf("the second attempt did not wait for the first's fence: %d", got.code)
	case <-time.After(400 * time.Millisecond):
	}
	close(fence.release)
	a, b := <-first, <-second
	if a.code != http.StatusCreated || b.code != http.StatusOK || a.run != b.run {
		t.Errorf("first %d %s, second %d %s; want 201 then 200 with the same run", a.code, a.run, b.code, b.run)
	}
	if runs, _, cases := world.written(t); runs != 1 || cases != 1 {
		t.Errorf("runs %d, stored cases %d; want one of each", runs, cases)
	}
}
