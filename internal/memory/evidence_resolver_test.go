package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/memory"
)

// countingLedger answers Read from a fixed run and counts how often it is asked,
// because reading the same run once per citation is the difference between one
// query and eight.
type countingLedger struct {
	steps map[domain.RunID][]domain.Step
	reads int
}

func (l *countingLedger) Read(
	_ context.Context, run domain.RunID, from int64,
) ([]domain.Step, error) {
	l.reads++
	out := []domain.Step{}
	for _, s := range l.steps[run] {
		if s.Seq >= from {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no such run")
	}
	return out, nil
}

type run struct {
	t       *testing.T
	ledger  *countingLedger
	content *engine.MemoryContent
	id      domain.RunID
	scope   domain.Scope
}

func newRun(t *testing.T, id domain.RunID) *run {
	t.Helper()
	return &run{
		t:       t,
		ledger:  &countingLedger{steps: map[domain.RunID][]domain.Step{}},
		content: engine.NewMemoryContent(),
		id:      id,
		scope:   domain.Scope{Company: "acme", Area: "ops"},
	}
}

func (r *run) step(kind domain.StepKind, payload any, labels ...string) domain.Step {
	r.t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		r.t.Fatalf("payload: %v", err)
	}
	s := domain.Step{
		RunID: r.id, Seq: int64(len(r.ledger.steps[r.id]) + 1),
		Kind: kind, Scope: r.scope, Payload: body,
	}
	if len(labels) > 0 {
		s.Labels = domain.NewLabels(labels...)
	}
	r.ledger.steps[r.id] = append(r.ledger.steps[r.id], s)
	return s
}

// put stores bytes the way the run would have, and answers the reference and the
// digest the store recorded.
func (r *run) put(seq int64, data []byte) (string, string) {
	r.t.Helper()
	ref, err := r.content.PutFor(context.Background(), "run", string(r.id), seq, data)
	if err != nil {
		r.t.Fatalf("PutFor: %v", err)
	}
	meta, err := r.content.Metadata(context.Background(), ref)
	if err != nil {
		r.t.Fatalf("Metadata: %v", err)
	}
	return ref, meta.Digest
}

func (r *run) resolver() *memory.Resolver {
	return memory.NewResolver(r.ledger, r.content)
}

// finished seeds a run that opened untrusted and closed with an answer, which is
// the shape the console cites by default.
func finished(t *testing.T, id domain.RunID) (*run, domain.MemoryEvidence) {
	t.Helper()
	r := newRun(t, id)
	r.step(domain.StepRunStarted, domain.RunStartedPayload{Trigger: "channel"}, domain.LabelUntrusted)
	ref, digest := r.put(2, []byte("the alert was already acknowledged"))
	r.step(domain.StepRunFinished, domain.RunFinishedPayload{
		OutcomeRef: ref, OutcomeDigest: digest,
	})
	return r, domain.MemoryEvidence{
		RunID: id, Seq: 2, Artifact: domain.ArtifactFinalAnswer,
	}
}

/*
Labels come from the run, folded up to the step cited, and never from the caller.

The step a citation names carries only what that step itself produced — a tool
result carries the result's labels and not the taint the run had already
accumulated. Reading the step alone would let a clean result inside a poisoned
run be remembered as clean, which is exactly the inference checkTaint refuses to
make.
*/
func TestResolve_labelsAreTheRunsUpToTheCitedStep(t *testing.T) {
	t.Parallel()
	r, cited := finished(t, "run-1")

	got, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{cited})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !got[0].Labels.HasAny(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the taint the run opened with", got[0].Labels)
	}
}

// A caller cannot colour its own citation. Labels are proof, not input.
func TestResolve_labelsFromTheCallerAreDiscarded(t *testing.T) {
	t.Parallel()
	r, cited := finished(t, "run-1")
	cited.Labels = domain.NewLabels(domain.LabelPersonal)

	got, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{cited})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got[0].Labels.HasAny(domain.LabelPersonal) {
		t.Errorf("labels = %v, want only what the ledger says", got[0].Labels)
	}
}

// The digest is the store's, never the caller's: a citation is checked against
// what was recorded, not against what it claims about itself.
func TestResolve_digestIsTheStoresNotTheCallers(t *testing.T) {
	t.Parallel()
	r, cited := finished(t, "run-1")
	cited.Digest = "sha256:0000000000000000"

	got, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{cited})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got[0].Digest == "sha256:0000000000000000" {
		t.Error("the caller's digest survived into the resolved citation")
	}
}

func TestResolve_citingAnotherScope_refused(t *testing.T) {
	t.Parallel()
	r, cited := finished(t, "run-1")

	other := domain.Scope{Company: "other", Area: "ops"}
	if _, err := r.resolver().Resolve(context.Background(), other, []domain.MemoryEvidence{cited}); err == nil {
		t.Error("resolved a citation into a run outside the scope")
	}
}

func TestResolve_erasedContent_refused(t *testing.T) {
	t.Parallel()
	r, cited := finished(t, "run-1")
	if _, err := r.content.Erase(context.Background(), "run-1", "subject"); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	// Fail closed: the bytes that would explain this memory are gone, so the
	// citation cannot be proved even though the ledger still names it.
	if _, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{cited}); err == nil {
		t.Error("resolved a citation whose content was erased")
	}
}

func TestResolve_artifactThatTheStepDoesNotName_refused(t *testing.T) {
	t.Parallel()
	r, cited := finished(t, "run-1")
	cited.Artifact = "invented"

	if _, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{cited}); err == nil {
		t.Error("resolved an artifact the step never published")
	}
}

/*
On the suggestion form the tool, the digest and the reference must agree.

The citation points at the arguments of one $fuseone.memory.suggest call. Two of
the three matching is a citation that names a real reference in the same run and
means something else — and the whole point of proving a citation is that it
cannot be pointed somewhere convenient.
*/
func TestResolve_memorySuggestionNeedsToolDigestAndRefToAgree(t *testing.T) {
	t.Parallel()

	seed := func(t *testing.T, tool domain.ToolID) (*run, domain.MemoryEvidence) {
		t.Helper()
		r := newRun(t, "run-1")
		r.step(domain.StepRunStarted, domain.RunStartedPayload{Trigger: "channel"}, domain.LabelUntrusted)
		ref, digest := r.put(2, []byte(`{"kind":"incident","subject":"slack"}`))
		r.step(domain.StepToolCalled, domain.ToolCalledPayload{
			Tool: tool, Effect: domain.EffectWrite, ArgsRef: ref, ArgsDigest: digest,
		})
		return r, domain.MemoryEvidence{
			RunID: "run-1", Seq: 2, Artifact: domain.ArtifactMemorySuggestion,
		}
	}

	t.Run("the suggest call resolves", func(t *testing.T) {
		r, cited := seed(t, domain.ToolMemorySuggest)
		got, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{cited})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if !got[0].Labels.HasAny(domain.LabelUntrusted) {
			t.Errorf("labels = %v, want the run's taint", got[0].Labels)
		}
	})

	t.Run("another tool's arguments at the same step do not", func(t *testing.T) {
		r, cited := seed(t, "crm.lookup")
		if _, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{cited}); err == nil {
			t.Error("resolved a memory suggestion against another tool's call")
		}
	})
}

// Eight citations of one run are one read. Resolving each on its own would fold
// the whole run eight times to answer the same question.
func TestResolve_manyCitationsOfOneRun_readTheLedgerOnce(t *testing.T) {
	t.Parallel()
	r, cited := finished(t, "run-1")

	in := make([]domain.MemoryEvidence, domain.MaxMemoryEvidence)
	for i := range in {
		in[i] = cited
	}
	if _, err := r.resolver().Resolve(context.Background(), r.scope, in); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if r.ledger.reads != 1 {
		t.Errorf("read the run %d times, want once", r.ledger.reads)
	}
}

// failingLedger stands in for a database that is unreachable, not for a run
// that does not exist.
type failingLedger struct{ err error }

func (l failingLedger) Read(context.Context, domain.RunID, int64) ([]domain.Step, error) {
	return nil, l.err
}

/*
A citation with no digest in the ledger is not a proved citation.

The tool call always records the digest beside the reference, so a payload
carrying one without the other is not a shape production writes. Accepting it
would be worse than refusing it: the resolver would fill the digest in from the
content store and hand back a citation marked as proved, whose proof it had
supplied itself.
*/
func TestResolve_memorySuggestionWithoutADigest_refused(t *testing.T) {
	t.Parallel()
	r := newRun(t, "run-1")
	r.step(domain.StepRunStarted, domain.RunStartedPayload{Trigger: "channel"})
	ref, _ := r.put(2, []byte(`{"kind":"incident"}`))
	r.step(domain.StepToolCalled, domain.ToolCalledPayload{
		Tool: domain.ToolMemorySuggest, Effect: domain.EffectWrite, ArgsRef: ref,
	})

	_, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{{
		RunID: "run-1", Seq: 2, Artifact: domain.ArtifactMemorySuggestion,
	}})
	if err == nil {
		t.Fatal("resolved a citation the ledger never recorded a digest for")
	}
	if !errors.Is(err, memory.ErrEvidenceInvalid) {
		t.Errorf("err = %v, want it to read as an invalid citation", err)
	}
}

// The ledger's digest and the store's must agree. They are written at the same
// moment about the same bytes, so a disagreement means one of them is not
// describing this content.
func TestResolve_ledgerDigestDisagreesWithTheStore_refused(t *testing.T) {
	t.Parallel()
	r := newRun(t, "run-1")
	r.step(domain.StepRunStarted, domain.RunStartedPayload{Trigger: "channel"})
	ref, _ := r.put(2, []byte("the answer"))
	r.step(domain.StepRunFinished, domain.RunFinishedPayload{
		OutcomeRef: ref, OutcomeDigest: "sha256:" + "00000000000000000000000000000000",
	})

	_, err := r.resolver().Resolve(context.Background(), r.scope, []domain.MemoryEvidence{{
		RunID: "run-1", Seq: 2, Artifact: domain.ArtifactFinalAnswer,
	}})
	if !errors.Is(err, memory.ErrEvidenceInvalid) {
		t.Errorf("err = %v, want an invalid citation", err)
	}
}

/*
A database that is away is not a citation that is wrong.

The reconciliation job repairs rows one at a time and has to tell the two apart:
invalid data is refused and moves on, an unavailable dependency is tried again.
Collapsing them would make the job skip rows it should have retried, and record
that it had checked them.
*/
func TestResolve_ledgerUnavailable_isNotAnInvalidCitation(t *testing.T) {
	t.Parallel()

	resolver := memory.NewResolver(failingLedger{err: context.Canceled}, engine.NewMemoryContent())
	_, err := resolver.Resolve(context.Background(), domain.Scope{Company: "acme", Area: "ops"},
		[]domain.MemoryEvidence{{RunID: "run-1", Seq: 2, Artifact: domain.ArtifactFinalAnswer}})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want the cancellation to survive", err)
	}
	if errors.Is(err, memory.ErrEvidenceInvalid) {
		t.Error("an unreachable ledger was reported as an invalid citation")
	}
}
