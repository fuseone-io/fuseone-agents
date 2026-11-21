package httpapi

import (
	gocontext "context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/regression"
)

/*
A correction kept from a rehearsal (NT-018 phase 5b). The case keeps the world it was
corrected in: each call the rehearsal made, with the answer the author gave for it, copied
with the input in one transaction. Only a rehearsal that ended, and whose every call was
answered, is kept (D5): a world with a hole in it would replay a situation nobody corrected.
The agent may be unpublished: the rehearsal's sealed scope is where the case is recorded, and
the caller needs agent:publish and run:read there, or the run reads as absent (D3).
*/

// rehearsed is a rehearsal of triage in acme/cx: the case typed, one call, its answer.
type rehearsed struct {
	store   *ledger.Memory
	content *engine.MemoryContent
	answer  string
}

func (r rehearsed) append(t *testing.T, kind domain.StepKind, payload any) domain.Step {
	t.Helper()
	raw, _ := json.Marshal(payload)
	s, err := r.store.Append(gocontext.Background(), domain.Step{
		RunID: "run-reh", Kind: kind, Scope: domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v-cand", Payload: raw,
	})
	if err != nil {
		t.Fatalf("append %s: %v", kind, err)
	}
	return s
}

// aRehearsal opens a rehearsal and makes one call, answered when answered is true; ended
// says whether it finishes.
func aRehearsal(t *testing.T, answered, ended bool) rehearsed {
	t.Helper()
	r := rehearsed{store: ledger.NewMemory(), content: engine.NewMemoryContent(), answer: `{"plan":"gold"}`}
	ctx := gocontext.Background()
	input, _ := r.content.Put(ctx, "run-reh", 1, []byte("A cliente pede reembolso."))
	r.append(t, domain.StepRunStarted, domain.RunStartedPayload{
		Trigger: "rehearsal", Simulated: true, Simulation: "reh_triage", Rehearsal: true,
		SealID: "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11", InputRef: input,
	})
	called := r.append(t, domain.StepToolCalled, domain.ToolCalledPayload{Tool: "crm.lookup", Effect: domain.EffectRead, ArgsDigest: "sha256:abc"})
	awaited := r.append(t, domain.StepWorldAwaited, domain.WorldAwaitedPayload{Tool: "crm.lookup", CallSeq: called.Seq})
	if answered {
		// Owned apart here, so a test can erase the answer alone and the case still reads.
		ref, _ := r.content.Put(ctx, "run-reh-answers", awaited.Seq+1, []byte(r.answer))
		r.append(t, domain.StepWorldSupplied, domain.WorldSuppliedPayload{AtSeq: awaited.Seq, Ref: ref, Digest: "d", Bytes: int64(len(r.answer)), By: "usr_ana"})
		r.append(t, domain.StepToolReturned, domain.ToolReturnedPayload{Tool: "crm.lookup", ForSeq: called.Seq, ResultRef: ref})
	}
	if ended {
		kind := domain.StepRunFinished
		var payload any = domain.RunFinishedPayload{Outcome: "answered"}
		if !answered {
			kind, payload = domain.StepAbandoned, domain.AbandonedPayload{Reason: "rehearsal ended by an author"}
		}
		r.append(t, kind, payload)
	}
	return r
}

// recordingCopies is the corpus fake, keeping what each recording copied.
type recordingCopies struct {
	*fakeCorpus
	copied []regression.Copying
}

func (f *recordingCopies) RecordCopying(_ gocontext.Context, c domain.RegressionCase, copying regression.Copying) (domain.RegressionCase, error) {
	if !f.inside {
		return domain.RegressionCase{}, errors.New("recorded outside the corpus fence")
	}
	if f.err != nil {
		return domain.RegressionCase{}, f.err
	}
	f.copied = append(f.copied, copying)
	f.recorded = append(f.recorded, c)
	return c, nil
}

func (r rehearsed) server(corpus *recordingCopies) *Server {
	return NewServer(r.store, "test").WithContent(r.content).WithCases(r.content).WithRegressions(corpus)
}

func fromRehearsal() openapi.RecordRegressionRequestObject {
	req := correction(openapi.Expectation{Kind: "never_calls", Value: ptr("crm.refund")})
	req.Body.RunId = "run-reh"
	return req
}

func TestRecordRegression_fromAnEndedRehearsal_keepsTheCaseWithItsWorld(t *testing.T) {
	t.Parallel()
	r := aRehearsal(t, true, true)
	corpus := &recordingCopies{fakeCorpus: &fakeCorpus{}}
	resp, err := r.server(corpus).RecordRegression(inArea("cx", domain.RoleAuthor), fromRehearsal())
	if err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	if _, ok := resp.(openapi.RecordRegression201JSONResponse); !ok {
		t.Fatalf("response = %T %+v, want 201", resp, resp)
	}
	copied := corpus.copied[0]
	if string(copied.Input) != "A cliente pede reembolso." || copied.World == nil || len(copied.World.Answers) != 1 {
		t.Fatalf("copied %+v, want the case typed and one answer", copied)
	}
	if a := copied.World.Answers[0]; a.Tool != "crm.lookup" || a.ArgsDigest != "sha256:abc" || string(a.Bytes) != r.answer {
		t.Errorf("answer %+v, want the call and the answer the author gave", a)
	}
	if c := corpus.recorded[0]; c.Scope != (domain.Scope{Company: "acme", Area: "cx"}) || c.FromRun != "run-reh" {
		t.Errorf("case %+v, want it in the rehearsal's scope, from the rehearsal", c)
	}
}

func TestRecordRegression_fromARehearsalThatCannotBeKept_isAConflict(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		r   rehearsed
		why string
	}{
		"still waiting for the world":  {aRehearsal(t, false, false), "has not ended"},
		"ended with a call unanswered": {aRehearsal(t, false, true), "call 1 (crm.lookup) was never answered"},
		"still going after the answer": {aRehearsal(t, true, false), "has not ended"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			corpus := &recordingCopies{fakeCorpus: &fakeCorpus{}}
			resp, err := tc.r.server(corpus).RecordRegression(inArea("cx", domain.RoleAuthor), fromRehearsal())
			if err != nil {
				t.Fatalf("RecordRegression: %v", err)
			}
			conflict, ok := resp.(openapi.RecordRegression409ApplicationProblemPlusJSONResponse)
			if !ok || len(corpus.copied) != 0 {
				t.Fatalf("response = %T, copied %d; want 409 and nothing copied", resp, len(corpus.copied))
			}
			// Said, so an author knows what to finish or to answer before keeping it.
			if conflict.Detail == nil || !strings.Contains(*conflict.Detail, tc.why) {
				t.Errorf("detail = %v, want it to say %q", conflict.Detail, tc.why)
			}
		})
	}
}

func TestRecordRegression_fromARehearsalWhoseAnswerWasErased_isAConflict(t *testing.T) {
	t.Parallel()
	r := aRehearsal(t, true, true)
	if _, err := r.content.Erase(gocontext.Background(), "run", "run-reh-answers", "subject"); err != nil {
		t.Fatal(err)
	}
	corpus := &recordingCopies{fakeCorpus: &fakeCorpus{}}
	resp, _ := r.server(corpus).RecordRegression(inArea("cx", domain.RoleAuthor), fromRehearsal())
	conflict, ok := resp.(openapi.RecordRegression409ApplicationProblemPlusJSONResponse)
	if !ok || len(corpus.copied) != 0 {
		t.Fatalf("response = %T, copied %d; want 409 and nothing copied", resp, len(corpus.copied))
	}
	if conflict.Detail == nil || !strings.Contains(*conflict.Detail, "answer 1 can no longer be read") {
		t.Errorf("detail = %v, want it to name the answer erased", conflict.Detail)
	}
}

// Outside the sealed scope, or for another agent, or into a corpus kept in another scope,
// the rehearsal reads as absent: nothing says whether it exists.
func TestRecordRegression_fromARehearsalOutOfReach_isAbsent(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ctx   gocontext.Context
		agent string
		err   error
	}{
		"an author of another area":      {ctx: inArea("finance", domain.RoleAuthor), agent: "triage"},
		"an auditor of the area":         {ctx: inArea("cx", domain.RoleAuditor), agent: "triage"},
		"another agent's path":           {ctx: inArea("cx", domain.RoleAuthor), agent: "billing"},
		"a corpus kept in another scope": {ctx: inArea("cx", domain.RoleAuthor), agent: "triage", err: regression.ErrDatasetElsewhere},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := aRehearsal(t, true, true)
			corpus := &recordingCopies{fakeCorpus: &fakeCorpus{err: tc.err}}
			req := fromRehearsal()
			req.AgentId = tc.agent
			resp, err := r.server(corpus).RecordRegression(tc.ctx, req)
			if err != nil {
				t.Fatalf("RecordRegression: %v", err)
			}
			if _, ok := resp.(openapi.RecordRegression404ApplicationProblemPlusJSONResponse); !ok {
				t.Errorf("response = %T, want 404", resp)
			}
		})
	}
}

// failingReads fails the reads it is told to, by their order, and serves the rest.
type failingReads struct {
	*ledger.Memory
	failing map[int]bool
	reads   int
}

func (f *failingReads) Read(ctx gocontext.Context, run domain.RunID, from int64) ([]domain.Step, error) {
	f.reads++
	if f.failing[f.reads] {
		return nil, errors.New("the ledger is away")
	}
	return f.Memory.Read(ctx, run, from)
}

// A read that fails is not a run that is not a rehearsal: the request stops, rather than
// record a published agent's rehearsal the ordinary way, with its input and without its world.
func TestRecordRegression_whenTheFirstReadFails_recordsNothing(t *testing.T) {
	t.Parallel()
	r := aRehearsal(t, true, true)
	store := &failingReads{Memory: r.store, failing: map[int]bool{1: true}}
	corpus := &recordingCopies{fakeCorpus: &fakeCorpus{}}
	server := NewServer(store, "test").WithAgents(triggerable(t)).
		WithContent(r.content).WithCases(r.content).WithRegressions(corpus)
	_, err := server.RecordRegression(inArea("cx", domain.RoleAuthor), fromRehearsal())
	if err == nil {
		t.Error("the request went on past a read that failed")
	}
	if len(corpus.recorded) != 0 {
		t.Errorf("recorded %+v, want nothing", corpus.recorded)
	}
}

// failingContent fails every read of content, as a store that is away does.
type failingContent struct{ *engine.MemoryContent }

func (failingContent) Get(gocontext.Context, string) ([]byte, error) {
	return nil, errors.New("the content store is away")
}

// What fails is the server's to answer for, not a refusal of the caller's request: only an
// absent or erased run or content, and a case the corpus refuses, are 4xx.
func TestRecordRegression_fromARehearsal_anOperationalFailure_isTheServersError(t *testing.T) {
	t.Parallel()
	for name, server := range map[string]func(r rehearsed, corpus *recordingCopies) *Server{
		"the ledger fails inside the fence": func(r rehearsed, corpus *recordingCopies) *Server {
			return NewServer(&failingReads{Memory: r.store, failing: map[int]bool{2: true}}, "test").
				WithContent(r.content).WithCases(r.content).WithRegressions(corpus)
		},
		"the content store fails": func(r rehearsed, corpus *recordingCopies) *Server {
			c := failingContent{r.content}
			return NewServer(r.store, "test").WithContent(c).WithCases(c).WithRegressions(corpus)
		},
		"the corpus fails": func(r rehearsed, corpus *recordingCopies) *Server {
			corpus.err = errors.New("the corpus is away")
			return r.server(corpus)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			corpus := &recordingCopies{fakeCorpus: &fakeCorpus{}}
			resp, err := server(aRehearsal(t, true, true), corpus).RecordRegression(inArea("cx", domain.RoleAuthor), fromRehearsal())
			if err == nil {
				t.Errorf("answered %T, want the failure returned as the server's", resp)
			}
		})
	}
}

// A world the corpus cannot keep whole is a rehearsal that cannot be kept, said as one: a
// conflict, as the others are, not a request written wrong.
func TestRecordRegression_aWorldTheCorpusCannotKeepWhole_isAConflict(t *testing.T) {
	t.Parallel()
	corpus := &recordingCopies{fakeCorpus: &fakeCorpus{err: fmt.Errorf("answer 1: %w", regression.ErrWorldUnkeepable)}}
	resp, err := aRehearsal(t, true, true).server(corpus).RecordRegression(inArea("cx", domain.RoleAuthor), fromRehearsal())
	if err != nil {
		t.Fatalf("RecordRegression: %v", err)
	}
	if _, ok := resp.(openapi.RecordRegression409ApplicationProblemPlusJSONResponse); !ok {
		t.Errorf("response = %T, want 409", resp)
	}
}
