package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/judge"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/model"
)

/*
A judge run, from its opening to its sealed verdict, on the pool the worker command builds
(NT-016).

No opener exists in this release, so the test opens judge runs the way release 2 will: the
case input, the rubric, the salt and a copy of the judged output as the run's content, and
an opening that names them. Everything after that is production: the simulation pool, the
resolver, the planner, the engine's closing and the Postgres content store.
*/

// judgingModel answers each judge with the code its model's name ends in, and remembers
// every model it was asked.
type judgingModel struct {
	mu    sync.Mutex
	asked []string
}

func (m *judgingModel) serve(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.asked = append(m.asked, body.Model)
		m.mu.Unlock()
		args, _ := json.Marshal(map[string]string{"summary": body.Model[len(body.Model)-1:]})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"finish_reason": "tool_calls", "message": map[string]any{
				"role": "assistant", "tool_calls": []map[string]any{{
					"id": "call_finish", "type": "function",
					"function": map[string]any{"name": "_fuseone__finish", "arguments": string(args)},
				}},
			}}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

const journeyRubric = `{"instructions":"Is the reply free of risky advice?","output":{"type":"category","categories":["ok","high_risk"],"passing":["ok"]}}`

func digestOf(b []byte, n int) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])[:n]
}

// experimentRun runs one experiment run to its finish and returns it with its opening.
func (w *experimentWorld) experimentRun(t *testing.T) (domain.Step, domain.RunFinishedPayload, string) {
	t.Helper()
	resp, err := w.server.StartExperiment(author(t.Context()), openapi.StartExperimentRequestObject{
		AgentId: string(w.agent),
		Params:  openapi.StartExperimentParams{IdempotencyKey: "journey-judged-1"},
		Body:    &openapi.ExperimentRequest{},
	})
	if err != nil {
		t.Fatalf("StartExperiment: %v", err)
	}
	accepted := resp.(openapi.StartExperiment202JSONResponse)
	runs, err := w.store.SimulationRuns(t.Context(), accepted.Id)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %v err=%v", runs, err)
	}
	w.settle(t, runs)
	steps, err := w.store.Read(t.Context(), runs[0], domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var finished domain.RunFinishedPayload
	if err := json.Unmarshal(steps[len(steps)-1].Payload, &finished); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return steps[0], finished, accepted.Id
}

// openJudge opens one judge run of the judged run, as release 2's opener will.
func (w *experimentWorld) openJudge(
	t *testing.T, run domain.RunID, judged domain.Step, finished domain.RunFinishedPayload,
	experiment, judgeModel string,
) {
	t.Helper()
	ctx, content := t.Context(), ledger.NewContent(w.pool)
	var opened domain.RunStartedPayload
	if err := json.Unmarshal(judged.Payload, &opened); err != nil {
		t.Fatalf("decode: %v", err)
	}
	put := func(b []byte) string {
		ref, err := content.Put(ctx, run, 1, b)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		return ref
	}
	input, err := content.Get(ctx, opened.InputRef)
	if err != nil {
		t.Fatalf("read the case input: %v", err)
	}
	output, err := content.Get(ctx, finished.OutcomeRef)
	if err != nil {
		t.Fatalf("read the judged output: %v", err)
	}
	rubric, _ := judge.ParseRubric([]byte(journeyRubric))
	salt := bytes.Repeat([]byte{byte(len(judgeModel))}, judge.SaltSize)
	salt[0] = judgeModel[len(judgeModel)-1]
	payload, _ := json.Marshal(domain.RunStartedPayload{
		Trigger: "simulation", Simulated: true, Simulation: "jdg_" + string(run), Case: opened.Case,
		DatasetVersion: opened.DatasetVersion, Experiment: experiment, InputRef: put(input),
		Judge: &domain.JudgeOpening{
			Run: judged.RunID, RubricDigest: rubric.Digest(), RubricRef: put(rubric.Canonical()),
			SaltRef: put(salt), SaltDigest: digestOf(salt, 64),
			OutputRef: put(output), OutputDigest: finished.OutcomeDigest,
			Provider: "judging", Model: judgeModel, CeilingMicros: 50_000, Repetitions: 1, Repetition: 1,
		},
	})
	if _, err := w.store.Append(ctx, domain.Step{
		RunID: run, Kind: domain.StepRunStarted, Scope: judged.Scope, AgentID: judged.AgentID,
		VersionID: judged.VersionID, OnBehalfOf: judged.OnBehalfOf, IdemKey: string(run), Payload: payload,
	}); err != nil {
		t.Fatalf("open %s: %v", run, err)
	}
}

// verdictOf is a finished judge run's closing and the code its sealed judgment holds.
func (w *experimentWorld) verdictOf(t *testing.T, run domain.RunID) (domain.RunFinishedPayload, byte) {
	t.Helper()
	head, err := w.store.Head(t.Context(), run)
	if err != nil || head.Kind != domain.StepRunFinished {
		t.Fatalf("%s ended at %s (%v), want finished", run, head.Kind, err)
	}
	var p domain.RunFinishedPayload
	if err := json.Unmarshal(head.Payload, &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	sealed, err := ledger.NewContent(w.pool).Get(t.Context(), p.JudgmentRef)
	if err != nil {
		t.Fatalf("read the judgment of %s: %v", run, err)
	}
	code, _, err := judge.OpenJudgment(sealed)
	if err != nil {
		t.Fatalf("open the judgment of %s: %v", run, err)
	}
	return p, code
}

func newJudgeWorld(t *testing.T) (*experimentWorld, *judgingModel) {
	t.Helper()
	world := newExperimentWorld(t)
	judging := &judgingModel{}
	if err := world.providers.Register(model.Provider{
		Name: "judging", Kind: model.KindOpenAICompatible, BaseURL: judging.serve(t), APIKey: "k",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	return world, judging
}

func TestJudge_fromItsOpening_sealsTheVerdictOnTheJudgesModel(t *testing.T) {
	world, judging := newJudgeWorld(t)
	judged, finished, experiment := world.experimentRun(t)
	declaredBefore := len(world.declared.models())

	world.openJudge(t, "run-judge-B", judged, finished, experiment, "judge-B")
	world.settle(t, []domain.RunID{"run-judge-B"})

	closing, code := world.verdictOf(t, "run-judge-B")
	if code != 'B' || closing.Reason != domain.RunFinishedByFinishTool || closing.JudgmentDigest == "" {
		t.Errorf("verdict = %c reason %s, want B sealed", code, closing.Reason)
	}
	if got := judging.asked; len(got) != 1 || got[0] != "judge-B" {
		t.Errorf("the judge's provider was asked %v, want judge-B once", got)
	}
	if got := len(world.declared.models()); got != declaredBefore {
		t.Errorf("the judged agent's model was asked %d more times, want none", got-declaredBefore)
	}
}

// contentRow is what a content row keeps once its bytes are gone.
type contentRow struct {
	seq       int64
	kind      string
	full      int64
	truncated bool
}

/*
After erasure, nothing that survives tells the declared outputs apart (NT-016).

One judge run per declared value, each answered with that value's code. Once their content
is erased, every row's sequence, owner, length and truncation flag is the same across them,
the run holds exactly six objects, and no enumeration of the declared outputs reproduces a
digest or the 64 bits in a reference.
*/
func TestJudge_afterErasure_nothingTellsTheValuesApart(t *testing.T) {
	world, _ := newJudgeWorld(t)
	judged, finished, experiment := world.experimentRun(t)
	rubric, _ := judge.ParseRubric([]byte(journeyRubric))
	var runs []domain.RunID
	for _, o := range rubric.Options() {
		run := domain.RunID(fmt.Sprintf("run-judge-%c", o.Code))
		world.openJudge(t, run, judged, finished, experiment, fmt.Sprintf("judge-%c", o.Code))
		runs = append(runs, run)
	}
	world.settle(t, runs)

	shapes := map[domain.RunID][]contentRow{}
	var digests []string
	for _, run := range runs {
		if _, err := world.pool.Exec(t.Context(), `
			update run_content set bytes = null, erased_at = now(), erased_reason = 'subject'
			where owner_kind = 'run' and run_id = $1`, string(run)); err != nil {
			t.Fatalf("erase: %v", err)
		}
		rows, err := world.pool.Query(t.Context(), `
			select ref, digest, seq, owner_kind, full_bytes, truncated from run_content
			where run_id = $1 order by seq, full_bytes`, string(run))
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		for rows.Next() {
			var ref, digest string
			var row contentRow
			if err := rows.Scan(&ref, &digest, &row.seq, &row.kind, &row.full, &row.truncated); err != nil {
				t.Fatalf("scan: %v", err)
			}
			shapes[run] = append(shapes[run], row)
			digests = append(digests, digest, ref)
		}
		rows.Close()
	}
	first := shapes[runs[0]]
	if len(first) != 6 {
		t.Fatalf("a judge run holds %d objects, want six: %+v", len(first), first)
	}
	for _, run := range runs[1:] {
		if fmt.Sprint(shapes[run]) != fmt.Sprint(first) {
			t.Errorf("%s left %+v, %s left %+v: the rows tell the values apart", runs[0], first, run, shapes[run])
		}
	}
	for _, o := range rubric.Options() {
		answer, _ := judge.SealAnswer(make([]byte, judge.SaltSize), string(o.Code))
		for _, guess := range [][]byte{{o.Code}, []byte(o.Value), judge.SealJudgment(make([]byte, judge.SaltSize), o), answer} {
			full := digestOf(guess, 64)[len("sha256:"):]
			for _, kept := range digests {
				if strings.Contains(kept, full) || strings.Contains(kept, full[:16]) {
					t.Errorf("enumerating %s reproduced what an erasure kept: %s", o.Value, kept)
				}
			}
		}
	}
}
