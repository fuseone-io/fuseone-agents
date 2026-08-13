package engine

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

// chain seals a sequence of steps the way the ledger would, so folds in these
// tests operate on the same shape production sees.
func chain(t *testing.T, specs ...domain.Step) []domain.Step {
	t.Helper()
	var out []domain.Step
	var prev *domain.Step
	for _, s := range specs {
		s.RunID = "run-1"
		s.Scope = domain.Scope{Company: "acme", Area: "cx"}
		if s.At.IsZero() {
			s.At = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
		}
		sealed, err := domain.NewStep(prev, s)
		if err != nil {
			t.Fatalf("NewStep(%s): %v", s.Kind, err)
		}
		out = append(out, sealed)
		prev = &out[len(out)-1]
	}
	return out
}

func payload(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func mustFold(t *testing.T, steps []domain.Step) State {
	t.Helper()
	s, err := Fold(steps)
	if err != nil {
		t.Fatalf("Fold: %v", err)
	}
	return s
}

func TestFold_emptyLedger_isUnstarted(t *testing.T) {
	t.Parallel()

	s := mustFold(t, nil)

	if s.Phase != PhaseUnstarted {
		t.Errorf("Phase = %v, want %v", s.Phase, PhaseUnstarted)
	}
	if s.Seq != 0 {
		t.Errorf("Seq = %d, want 0", s.Seq)
	}
}

func TestFold_runStarted_capturesIdentityAndScope(t *testing.T) {
	t.Parallel()

	steps := chain(t, domain.Step{
		Kind:       domain.StepRunStarted,
		AgentID:    "triage",
		VersionID:  "v3",
		OnBehalfOf: "ana",
	})

	s := mustFold(t, steps)

	if s.Phase != PhaseRunning {
		t.Errorf("Phase = %v, want %v", s.Phase, PhaseRunning)
	}
	if s.AgentID != "triage" || s.VersionID != "v3" {
		t.Errorf("agent = %s@%s, want triage@v3", s.AgentID, s.VersionID)
	}
	// The run is pinned to the version that started it: publishing a new
	// version must never alter a run already in flight (PRD DE-09).
	if s.OnBehalfOf != "ana" {
		t.Errorf("OnBehalfOf = %s, want ana", s.OnBehalfOf)
	}
	if s.Scope.Company != "acme" {
		t.Errorf("Scope.Company = %s, want acme", s.Scope.Company)
	}
}

func TestFold_costOnEverySteap_accumulatesIntoSpent(t *testing.T) {
	t.Parallel()

	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{Kind: domain.StepPlanned, Cost: domain.Cost{InputTokens: 1000, OutputTokens: 200, Micros: 8_000}},
		domain.Step{Kind: domain.StepPlanned, Cost: domain.Cost{InputTokens: 500, CacheReadTokens: 900, Micros: 3_000}},
	)

	s := mustFold(t, steps)

	if got, want := s.Spent.Micros, int64(11_000); got != want {
		t.Errorf("Spent.Micros = %d, want %d", got, want)
	}
	if got, want := s.Spent.Tokens, int64(2600); got != want {
		t.Errorf("Spent.Tokens = %d, want %d", got, want)
	}
	if got, want := s.Spent.Steps, int64(3); got != want {
		t.Errorf("Spent.Steps = %d, want %d", got, want)
	}
}

func TestFold_reservationOutstanding_countsAgainstBudget(t *testing.T) {
	t.Parallel()

	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{
			Kind:    domain.StepBudgetReserved,
			Payload: payload(t, domain.BudgetReservedPayload{Micros: 50_000, Tokens: 20_000}),
		},
	)

	s := mustFold(t, steps)

	// Committed is what the Gate checks. A reservation that has not been
	// reconciled yet must count, otherwise parallel steps blow the ceiling in
	// the window between spending and accounting (PRD FO-01).
	if got, want := s.Committed().Micros, int64(50_000); got != want {
		t.Errorf("Committed.Micros = %d, want %d", got, want)
	}
}

func TestFold_reservationReconciled_releasesTheDifference(t *testing.T) {
	t.Parallel()

	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{
			Kind:    domain.StepBudgetReserved,
			Payload: payload(t, domain.BudgetReservedPayload{Micros: 50_000, Tokens: 20_000}),
		},
		domain.Step{
			Kind:    domain.StepBudgetReconciled,
			Cost:    domain.Cost{InputTokens: 4_000, OutputTokens: 600, Micros: 12_000},
			Payload: payload(t, domain.BudgetReconciledPayload{ReleasedMicros: 50_000, ReleasedTokens: 20_000}),
		},
	)

	s := mustFold(t, steps)

	if s.Reserved.Micros != 0 {
		t.Errorf("Reserved.Micros = %d, want 0 after reconcile", s.Reserved.Micros)
	}
	if got, want := s.Committed().Micros, int64(12_000); got != want {
		t.Errorf("Committed.Micros = %d, want %d (actual cost only)", got, want)
	}
}

func TestFold_approvalRequestedThenGranted_returnsToRunning(t *testing.T) {
	t.Parallel()

	requested := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{
			Kind:    domain.StepApprovalRequested,
			Payload: payload(t, domain.ApprovalRequestedPayload{Tool: "crm.reply", Reason: "customer_reply"}),
		},
	)

	s := mustFold(t, requested)
	if s.Phase != PhaseAwaitingApproval {
		t.Fatalf("Phase = %v, want %v", s.Phase, PhaseAwaitingApproval)
	}
	if s.PendingApproval == nil || s.PendingApproval.Tool != "crm.reply" {
		t.Fatalf("PendingApproval = %+v, want tool crm.reply", s.PendingApproval)
	}

	granted := chain(t, append(stripSeal(requested), domain.Step{
		Kind:    domain.StepApprovalDecided,
		Payload: payload(t, domain.ApprovalDecidedPayload{Approved: true, By: "gestor"}),
	})...)

	s = mustFold(t, granted)
	if s.Phase != PhaseRunning {
		t.Errorf("Phase = %v, want %v", s.Phase, PhaseRunning)
	}
	if s.PendingApproval != nil {
		t.Errorf("PendingApproval = %+v, want nil after decision", s.PendingApproval)
	}
}

func TestFold_toolCalled_marksIdempotencyKeyExecuted(t *testing.T) {
	t.Parallel()

	const key = "run-1:4:crm.refund:9f2a"
	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{Kind: domain.StepToolCalled, IdemKey: key,
			Payload: payload(t, domain.ToolCalledPayload{Tool: "crm.refund"})},
	)

	s := mustFold(t, steps)

	// This is the resume guarantee: after a crash the run reloads the ledger
	// and must not re-execute an effect it already caused (PRD DE-16).
	if !s.AlreadyExecuted(key) {
		t.Error("AlreadyExecuted = false for a key the ledger recorded")
	}
	if s.AlreadyExecuted("some-other-key") {
		t.Error("AlreadyExecuted = true for an unrecorded key")
	}
}

func TestFold_untrustedToolResult_taintsRunContext(t *testing.T) {
	t.Parallel()

	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{Kind: domain.StepToolReturned, Labels: domain.NewLabels(domain.LabelUntrusted)},
		domain.Step{Kind: domain.StepToolReturned, Labels: domain.NewLabels(domain.LabelPersonal)},
	)

	s := mustFold(t, steps)

	// Taint only ever grows within a run: dirty data read at step 2 stays a
	// consideration for the action proposed at step 6 (PRD SE-05).
	if !s.Labels.Has(domain.LabelUntrusted) || !s.Labels.Has(domain.LabelPersonal) {
		t.Errorf("Labels = %v, want both untrusted and personal", s.Labels)
	}
}

func TestFold_parked_isTerminalAndCarriesReason(t *testing.T) {
	t.Parallel()

	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{Kind: domain.StepParked,
			Payload: payload(t, domain.ParkedPayload{Reason: "budget_exhausted", Attempts: 3})},
	)

	s := mustFold(t, steps)

	if s.Phase != PhaseParked {
		t.Errorf("Phase = %v, want %v", s.Phase, PhaseParked)
	}
	// Parked is resumable, not finished: raising the ceiling continues the run
	// from the exact step it stopped at (PRD FO-04).
	if s.Terminal() {
		t.Error("Terminal = true for a parked run, which must stay resumable")
	}
}

// The resume property. Folding the whole ledger must equal folding a prefix
// and then applying the remainder — for every split point. This is NF-02: a
// worker that dies mid-run reloads and continues to the identical state.
func TestFold_anyPrefixThenRemainder_equalsFoldingWhole(t *testing.T) {
	t.Parallel()

	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted, AgentID: "triage", VersionID: "v3"},
		domain.Step{Kind: domain.StepPlanned, Cost: domain.Cost{InputTokens: 900, Micros: 4_000}},
		domain.Step{Kind: domain.StepGateDecided,
			Payload: payload(t, domain.GateDecidedPayload{Tool: "crm.lookup", Verdict: domain.VerdictAllow})},
		domain.Step{Kind: domain.StepBudgetReserved,
			Payload: payload(t, domain.BudgetReservedPayload{Micros: 20_000})},
		domain.Step{Kind: domain.StepToolCalled, IdemKey: "k1",
			Payload: payload(t, domain.ToolCalledPayload{Tool: "crm.lookup"})},
		domain.Step{Kind: domain.StepToolReturned, Labels: domain.NewLabels(domain.LabelUntrusted)},
		domain.Step{Kind: domain.StepBudgetReconciled, Cost: domain.Cost{Micros: 9_000},
			Payload: payload(t, domain.BudgetReconciledPayload{ReleasedMicros: 20_000})},
	)

	whole := mustFold(t, steps)

	for split := range len(steps) + 1 {
		resumed := mustFold(t, steps[:split])
		for _, s := range steps[split:] {
			if err := resumed.Apply(s); err != nil {
				t.Fatalf("split %d: Apply(%s): %v", split, s.Kind, err)
			}
		}
		if !reflect.DeepEqual(whole, resumed) {
			t.Errorf("split %d: resumed state differs from whole fold\n whole=%+v\n resumed=%+v",
				split, whole, resumed)
		}
	}
}

func TestApply_stepOutOfOrder_rejected(t *testing.T) {
	t.Parallel()

	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted},
		domain.Step{Kind: domain.StepPlanned},
		domain.Step{Kind: domain.StepPlanned},
	)

	s := mustFold(t, steps[:2])

	// Skipping seq 3 and applying seq 4 would silently lose an effect.
	if err := s.Apply(steps[2]); err != nil {
		t.Fatalf("in-order Apply failed: %v", err)
	}
	if err := s.Apply(steps[2]); err == nil {
		t.Error("Apply accepted a replayed sequence number")
	}
}

// stripSeal returns the mutable fields of sealed steps so a test can extend a
// chain without hand-rebuilding every hash.
func stripSeal(steps []domain.Step) []domain.Step {
	out := make([]domain.Step, len(steps))
	for i, s := range steps {
		s.Seq, s.PrevHash, s.Hash = 0, nil, nil
		out[i] = s
	}
	return out
}

func TestFold_failed_isTerminalAndNotResumable(t *testing.T) {
	t.Parallel()

	// A run somebody abandoned has ended. Parking is a pause and resumes; this
	// is the other ending, and a worker that picked it up again would redo
	// work a person just had undone.
	s := mustFold(t, chain(t, domain.Step{
		Kind: domain.StepRunStarted, AgentID: "billing", Payload: []byte(`{}`),
	}, domain.Step{
		Kind:    domain.StepFailed,
		Payload: payload(t, domain.FailedPayload{Code: "abandoned"}),
	}))

	if s.Phase != PhaseFailed {
		t.Errorf("Phase = %v, want %v", s.Phase, PhaseFailed)
	}
	if !s.Terminal() {
		t.Error("Terminal() = false; an abandoned run has ended")
	}
	if s.Resumable() {
		t.Error("Resumable() = true; a worker would redo what was just undone")
	}
}

func TestFold_abandonedWithCompensation_waitsForAWorker(t *testing.T) {
	t.Parallel()

	// The person's decision ends the run, but the undoing is real tool calls
	// that take as long as they take. A request handler is the wrong place to
	// hold them, so the run stays claimable until a worker has done it.
	s := mustFold(t, chain(t, domain.Step{
		Kind: domain.StepRunStarted, AgentID: "billing", Payload: []byte(`{}`),
	}, domain.Step{
		Kind: domain.StepAbandoned,
		Payload: payload(t, domain.AbandonedPayload{
			By: "ana", Reason: "duplicate order", Compensate: true,
		}),
	}))

	if s.Phase != PhaseCompensating {
		t.Errorf("Phase = %v, want %v", s.Phase, PhaseCompensating)
	}
	if s.Terminal() {
		t.Error("Terminal() = true; the undoing has not happened yet")
	}
}

func TestFold_abandonedWithoutCompensation_endsThere(t *testing.T) {
	t.Parallel()

	// Leaving the world as it is can be the right answer. Nothing to do, so
	// no worker should pick this up looking for work.
	s := mustFold(t, chain(t, domain.Step{
		Kind: domain.StepRunStarted, AgentID: "billing", Payload: []byte(`{}`),
	}, domain.Step{
		Kind: domain.StepAbandoned,
		Payload: payload(t, domain.AbandonedPayload{
			By: "ana", Reason: "the charge should stand", Compensate: false,
		}),
	}))

	if s.Phase != PhaseFailed || !s.Terminal() || s.Resumable() {
		t.Errorf("Phase = %v, terminal = %v, resumable = %v; want it ended",
			s.Phase, s.Terminal(), s.Resumable())
	}
}

func TestFold_abandonedWhileAwaitingApproval_leavesNothingToDecide(t *testing.T) {
	t.Parallel()

	// The console offers Approve and Reject from the pending approval. A run
	// somebody ended still carrying one asks a person to decide on a call that
	// will never happen — and the decision would append to a finished trail.
	s := mustFold(t, chain(t, domain.Step{
		Kind: domain.StepRunStarted, AgentID: "billing", Payload: []byte(`{}`),
	}, domain.Step{
		Kind: domain.StepApprovalRequested,
		Payload: payload(t, domain.ApprovalRequestedPayload{
			Tool: "crm.reply", Rule: "taint", Effect: domain.EffectWrite,
		}),
	}, domain.Step{
		Kind: domain.StepAbandoned,
		Payload: payload(t, domain.AbandonedPayload{
			By: "ana", Reason: "handled by phone", Compensate: true,
		}),
	}))

	if s.PendingApproval != nil {
		t.Errorf("PendingApproval = %+v, want nothing left to decide", s.PendingApproval)
	}
}

func TestFold_resumed_returnsAParkedRunToRunning(t *testing.T) {
	t.Parallel()

	// Parking is a pause, and the product says so in four places: raise the
	// ceiling and the run continues from the exact step it stopped at. There
	// was no way to say the ceiling had been raised, so every parked run was
	// parked for ever.
	s := mustFold(t, chain(t, domain.Step{
		Kind: domain.StepRunStarted, AgentID: "billing", Payload: []byte(`{}`),
	}, domain.Step{
		Kind:    domain.StepParked,
		Payload: payload(t, domain.ParkedPayload{Reason: "budget_exhausted"}),
	}, domain.Step{
		Kind:    domain.StepResumed,
		Payload: payload(t, domain.ResumedPayload{By: "ana", Note: "teto erguido"}),
	}))

	if s.Phase != PhaseRunning {
		t.Errorf("Phase = %v, want %v", s.Phase, PhaseRunning)
	}
	if !s.Resumable() {
		t.Error("Resumable() = false; no worker will ever pick it up")
	}
}

func TestFold_resumed_forgetsTheRefusalsThatCausedTheParking(t *testing.T) {
	t.Parallel()

	// The block counter exists to stop the platform arguing with a planner
	// that will not take no for an answer. Carried across a resume it is
	// evidence about a world that changed: the person resuming has just said
	// they fixed the thing, and the run would park again on its first refusal
	// instead of getting the attempts the supervision policy allows.
	blocked := domain.Step{
		Kind: domain.StepGateDecided,
		Payload: payload(t, domain.GateDecidedPayload{
			Tool: "kb.search", Verdict: domain.VerdictBlock, Rule: "policy",
		}),
	}
	s := mustFold(t, chain(t,
		domain.Step{Kind: domain.StepRunStarted, AgentID: "suporte", Payload: []byte(`{}`)},
		blocked, blocked, blocked,
		domain.Step{Kind: domain.StepParked, Payload: payload(t, domain.ParkedPayload{Reason: "no_progress"})},
		domain.Step{Kind: domain.StepResumed, Payload: payload(t, domain.ResumedPayload{By: "ana", Note: "política desligada"})},
	))

	if s.ConsecutiveBlocks != 0 {
		t.Errorf("ConsecutiveBlocks = %d, want the count forgotten", s.ConsecutiveBlocks)
	}
}

func TestFold_wallClock_accumulatesFromTheRunsOwnInstants(t *testing.T) {
	t.Parallel()

	// A budget covers an amount, a number of steps, a number of tool calls
	// and wall-clock time, and all four are checked at the Gate (PRD FO-03).
	// Three of them were. Nothing ever measured how long a run had been going,
	// so `wall_clock_ms` in a specification was a ceiling an author could
	// write, the API would report, and no run would ever hit.
	steps := chain(t,
		domain.Step{Kind: domain.StepRunStarted, AgentID: "suporte", Payload: []byte(`{}`)},
		domain.Step{Kind: domain.StepPlanned, Payload: []byte(`{}`)},
	)
	steps[1].At = steps[0].At.Add(90 * time.Second)

	s := mustFold(t, steps)

	if s.Spent.WallClockMS != 90_000 {
		t.Errorf("Spent.WallClockMS = %d, want the 90s between the first step and the last",
			s.Spent.WallClockMS)
	}
	if s.Committed().WallClockMS != 90_000 {
		t.Errorf("Committed().WallClockMS = %d; the Gate never sees the elapsed time",
			s.Committed().WallClockMS)
	}
}
