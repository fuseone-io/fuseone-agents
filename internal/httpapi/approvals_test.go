package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/ledger"
)

// A decision that does not say who made it is the one record this product
// exists to keep, kept empty.

type fixedAt struct{ t time.Time }

func (f fixedAt) Now() time.Time { return f.t }

func awaitingApproval(t *testing.T, at time.Time) *ledger.Memory {
	t.Helper()
	store := ledger.NewMemory()
	scope := domain.Scope{Company: "acme", Area: "cx"}

	for _, step := range []domain.Step{
		{RunID: "run-1", Kind: domain.StepRunStarted, Scope: scope,
			AgentID: "triage", VersionID: "v1", At: at.Add(-time.Minute)},
		{RunID: "run-1", Kind: domain.StepApprovalRequested, Scope: scope,
			AgentID: "triage", VersionID: "v1", At: at,
			Payload: mustPayload(t, domain.ApprovalRequestedPayload{Tool: "crm.reply", Rule: "taint"})},
	} {
		if _, err := store.Append(context.Background(), step); err != nil {
			t.Fatalf("seed %s: %v", step.Kind, err)
		}
	}
	return store
}

func TestDecideApproval_recordsWhoDecidedAndWhen(t *testing.T) {
	t.Parallel()

	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)
	decided := asked.Add(3 * time.Minute)

	if _, err := NewServer(store, "test").WithClock(fixedAt{t: decided}).
		DecideApproval(inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
			RunId: "run-1",
			Body:  &openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2},
		}); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	last := steps[len(steps)-1]

	var payload domain.ApprovalDecidedPayload
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Without this the trail says an action was authorised and cannot say by
	// whom — the product's whole promise, kept empty.
	if payload.By == "" {
		t.Error("the decision does not record who made it")
	}
	// Stamped with the moment it was made. Copying the previous step's time
	// makes every decision look instantaneous and erases how long a person
	// took, which is what the human queue is measured by.
	if !last.At.Equal(decided) {
		t.Errorf("decided at %s, want %s", last.At, decided)
	}
}

func TestDecideApproval_clockBehindTheLastStep_doesNotWalkTheChainBackwards(t *testing.T) {
	t.Parallel()

	// Two machines, two clocks. A step stamped before the one it seals would
	// make the trail read out of order for a reason nobody could diagnose.
	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)

	if _, err := NewServer(store, "test").WithClock(fixedAt{t: asked.Add(-time.Hour)}).
		DecideApproval(inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
			RunId: "run-1",
			Body:  &openapi.DecideApprovalJSONRequestBody{Approved: false, AtSeq: 2},
		}); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	steps, _ := store.Read(context.Background(), "run-1", domain.FirstSeq)
	last := steps[len(steps)-1]
	if last.At.Before(asked) {
		t.Errorf("the decision is stamped %s, before the request at %s", last.At, asked)
	}
}

/*
A decision is a precondition on the run, not a check the caller made earlier.

Reading state, checking the sequence and appending are three moments, and the
run can move between the first and the third. Here the run is abandoned while
the decision is in flight — which is what a slow request against a run somebody
cancelled looks like — and the decision must find that out at the write rather
than land on top of it and put a terminal run back to work.
*/
func TestDecideApproval_theRunWasAbandonedMeanwhile_isRefusedAndRecordsNothing(t *testing.T) {
	t.Parallel()

	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)
	// The abandonment lands between the read and the write, exactly where a
	// caller's own check can no longer see it.
	racing := &racesOneAppend{Store: store, before: domain.Step{
		RunID: "run-1", Kind: domain.StepAbandoned,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v1", At: asked.Add(time.Minute),
	}}

	resp, err := NewServer(racing, "test").WithClock(fixedAt{t: asked.Add(3 * time.Minute)}).
		DecideApproval(inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
			RunId: "run-1",
			Body:  &openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2},
		})
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if _, ok := resp.(openapi.DecideApproval409ApplicationProblemPlusJSONResponse); !ok {
		t.Fatalf("response = %T, want the decision refused as out of date", resp)
	}

	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, s := range steps {
		if s.Kind == domain.StepApprovalDecided {
			t.Fatal("a decision was recorded on a run that had been abandoned")
		}
	}
	if last := steps[len(steps)-1]; last.Kind != domain.StepAbandoned {
		t.Errorf("the run ends at %s, want it still abandoned", last.Kind)
	}
}

/*
Two people answer the same question at the same moment, and one of them wins.

A card in a channel is addressed to whoever is watching it, so this is not an
exotic interleaving — it is what happens when two approvers read the same
message. Both requests pass their own check against the same state; only the
ledger can settle it, and it settles it by refusing the second write rather
than by letting both land and folding the damage afterwards.
*/
func TestDecideApproval_twoDecisionsOnOneRequest_recordExactlyOne(t *testing.T) {
	t.Parallel()

	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)
	server := NewServer(store, "test").WithClock(fixedAt{t: asked.Add(time.Minute)})

	answers := make(chan openapi.DecideApprovalResponseObject, 2)
	var wg sync.WaitGroup
	for _, approved := range []bool{true, false} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := server.DecideApproval(
				inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
					RunId: "run-1",
					Body:  &openapi.DecideApprovalJSONRequestBody{Approved: approved, AtSeq: 2},
				})
			if err != nil {
				t.Errorf("DecideApproval: %v", err)
				return
			}
			answers <- resp
		}()
	}
	wg.Wait()
	close(answers)

	accepted, refused := 0, 0
	for resp := range answers {
		switch resp.(type) {
		case openapi.DecideApproval200JSONResponse:
			accepted++
		case openapi.DecideApproval409ApplicationProblemPlusJSONResponse:
			refused++
		default:
			t.Errorf("response = %T, want an answer or a conflict", resp)
		}
	}
	if accepted != 1 || refused != 1 {
		t.Errorf("%d accepted and %d refused, want exactly one of each", accepted, refused)
	}

	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	decisions := 0
	for _, s := range steps {
		if s.Kind == domain.StepApprovalDecided {
			decisions++
		}
	}
	if decisions != 1 {
		t.Errorf("%d decisions recorded, want one", decisions)
	}
}

// racesOneAppend lands a step of its own just before the first guarded append,
// which is the window a caller's own check cannot see into.
type racesOneAppend struct {
	Store
	before domain.Step
	once   sync.Once
}

func (r *racesOneAppend) AppendIfHead(
	ctx context.Context, head domain.StepRef, s domain.Step,
) (domain.Step, error) {
	r.once.Do(func() { _, _ = r.Store.Append(ctx, r.before) })
	return r.Store.AppendIfHead(ctx, head, s)
}

/*
The record says which request it answered.

Twice over, and each half earns its place. The payload names the step so a fold
can tell a decision about the question now open from one about a question the
run moved past — a stale card in a channel produces the second, and read by
position it reopened a finished run.

The idempotency key names the same pair, and it is what makes a second decision
impossible durably rather than only where somebody remembered to check. The
ledger already refuses a repeated key, in the store and in the fake, so this
needs no rule of its own beside it.
*/
func TestDecideApproval_theRecord_namesTheRequestItAnswers(t *testing.T) {
	t.Parallel()

	asked := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	store := awaitingApproval(t, asked)

	if _, err := NewServer(store, "test").WithClock(fixedAt{t: asked.Add(time.Minute)}).
		DecideApproval(inArea("cx", domain.RoleApprover), openapi.DecideApprovalRequestObject{
			RunId: "run-1",
			Body:  &openapi.DecideApprovalJSONRequestBody{Approved: true, AtSeq: 2},
		}); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	steps, err := store.Read(context.Background(), "run-1", domain.FirstSeq)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	last := steps[len(steps)-1]

	var decided domain.ApprovalDecidedPayload
	if err := json.Unmarshal(last.Payload, &decided); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decided.AtSeq != 2 {
		t.Errorf("payload names step %d, want the request it answered", decided.AtSeq)
	}
	if want := domain.ApprovalDecisionKey("run-1", 2); last.IdemKey != want {
		t.Errorf("key = %q, want %q", last.IdemKey, want)
	}
}

// And the key is what stops a second one, for any caller that reaches the
// write without the precondition.
func TestDecideApproval_asecondDecisionUnderTheSameKey_isRefusedByTheLedger(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := awaitingApproval(t, time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC))
	decision := domain.Step{
		RunID: "run-1", Kind: domain.StepApprovalDecided,
		Scope:   domain.Scope{Company: "acme", Area: "cx"},
		AgentID: "triage", VersionID: "v1",
		IdemKey: domain.ApprovalDecisionKey("run-1", 2),
		Payload: mustPayload(t, domain.ApprovalDecidedPayload{Approved: true, By: "ana", AtSeq: 2}),
	}

	if _, err := store.Append(ctx, decision); err != nil {
		t.Fatalf("the first decision: %v", err)
	}
	if _, err := store.Append(ctx, decision); !errors.Is(err, ledger.ErrIdemConflict) {
		t.Fatalf("err = %v, want the second refused by the key", err)
	}
}
