package domain

import (
	"testing"
	"time"
)

// base returns a valid step template; tests vary one field at a time.
func base() Step {
	return Step{
		RunID:      "run-1",
		Kind:       StepToolCalled,
		Scope:      Scope{Company: "acme", Area: "cx"},
		AgentID:    "triage",
		VersionID:  "v3",
		OnBehalfOf: "ana",
		Payload:    []byte(`{"tool":"crm.lookup"}`),
		At:         time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
	}
}

func mustStep(t *testing.T, prev *Step, s Step) Step {
	t.Helper()
	out, err := NewStep(prev, s)
	if err != nil {
		t.Fatalf("NewStep: %v", err)
	}
	return out
}

func TestNewStep_firstInChain_startsAtSeqOneWithNoPrevHash(t *testing.T) {
	t.Parallel()
	s := mustStep(t, nil, base())

	if s.Seq != FirstSeq {
		t.Errorf("Seq = %d, want %d", s.Seq, FirstSeq)
	}
	if len(s.PrevHash) != 0 {
		t.Errorf("PrevHash = %x, want empty", s.PrevHash)
	}
	if err := s.VerifyLink(nil); err != nil {
		t.Errorf("VerifyLink: %v", err)
	}
}

func TestNewStep_sealedAgainstPrevious_verifiesAsChain(t *testing.T) {
	t.Parallel()
	first := mustStep(t, nil, base())
	second := mustStep(t, &first, base())

	if second.Seq != first.Seq+1 {
		t.Errorf("Seq = %d, want %d", second.Seq, first.Seq+1)
	}
	if err := VerifyChain([]Step{first, second}); err != nil {
		t.Errorf("VerifyChain: %v", err)
	}
}

func TestVerifyLink_payloadTamperedAfterSealing_detected(t *testing.T) {
	t.Parallel()
	s := mustStep(t, nil, base())

	s.Payload = []byte(`{"tool":"crm.delete"}`)

	if err := s.VerifyLink(nil); err != ErrHashMismatch {
		t.Errorf("VerifyLink = %v, want %v", err, ErrHashMismatch)
	}
}

func TestVerifyLink_costTamperedAfterSealing_detected(t *testing.T) {
	t.Parallel()

	priced := base()
	priced.Cost = Cost{InputTokens: 12_040, OutputTokens: 891, Micros: 28_300}
	s := mustStep(t, nil, priced)

	// Rewriting cost is the interesting attack: it changes the invoice without
	// touching anything the agent visibly did.
	s.Cost.Micros = 0

	if err := s.VerifyLink(nil); err != ErrHashMismatch {
		t.Errorf("VerifyLink = %v, want %v", err, ErrHashMismatch)
	}
}

func TestVerifyChain_middleStepRemoved_detected(t *testing.T) {
	t.Parallel()
	first := mustStep(t, nil, base())
	second := mustStep(t, &first, base())
	third := mustStep(t, &second, base())

	if err := VerifyChain([]Step{first, third}); err == nil {
		t.Fatal("VerifyChain accepted a chain with a missing step")
	}
}

func TestVerifyChain_stepRepointedToWrongParent_detected(t *testing.T) {
	t.Parallel()
	first := mustStep(t, nil, base())
	second := mustStep(t, &first, base())
	forged := mustStep(t, &first, base())

	// Same sequence, different parent link: a fork the chain must reject.
	forged.Seq = second.Seq
	forged.PrevHash = []byte("not the real parent hash")

	if err := VerifyChain([]Step{first, forged}); err != ErrChainBroken {
		t.Errorf("VerifyChain = %v, want %v", err, ErrChainBroken)
	}
}

// The length prefix is what stops two different field splits from digesting
// to the same value. Without it these two steps would hash identically.
func TestComputeHash_adjacentFieldsShiftedAcrossBoundary_differentHashes(t *testing.T) {
	t.Parallel()

	a := base()
	a.AgentID, a.VersionID = "ab", "c"

	b := base()
	b.AgentID, b.VersionID = "a", "bc"

	sa := mustStep(t, nil, a)
	sb := mustStep(t, nil, b)

	if equalBytes(sa.Hash, sb.Hash) {
		t.Error("field boundary ambiguity: distinct steps produced the same hash")
	}
}

func TestNewStep_labelsGivenOutOfOrder_hashIsStable(t *testing.T) {
	t.Parallel()

	a := base()
	a.Labels = Labels{LabelPersonal, LabelUntrusted}

	b := base()
	b.Labels = Labels{LabelUntrusted, LabelPersonal, LabelUntrusted}

	sa := mustStep(t, nil, a)
	sb := mustStep(t, nil, b)

	if !equalBytes(sa.Hash, sb.Hash) {
		t.Error("same label set in a different order produced a different hash")
	}
}

func TestNewStep_subMicrosecondTime_survivesStorageRoundTrip(t *testing.T) {
	t.Parallel()

	s := base()
	s.At = time.Date(2026, 8, 10, 12, 0, 0, 123_456_789, time.UTC)
	sealed := mustStep(t, nil, s)

	// Postgres keeps microseconds. Simulate the read-back and re-verify.
	readBack := sealed
	readBack.At = sealed.At.Truncate(time.Microsecond)

	if err := readBack.VerifyLink(nil); err != nil {
		t.Errorf("step failed to verify after microsecond round trip: %v", err)
	}
}

func TestNewStep_invalidInput_rejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mutil func(*Step)
		want  error
	}{
		{"missing run", func(s *Step) { s.RunID = "" }, ErrNoRun},
		{"unknown kind", func(s *Step) { s.Kind = "teleport" }, ErrInvalidKind},
		{"missing company", func(s *Step) { s.Scope.Company = "" }, ErrInvalidScope},
		{"missing area", func(s *Step) { s.Scope.Area = "" }, ErrInvalidScope},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := base()
			tc.mutil(&s)

			if _, err := NewStep(nil, s); err != tc.want {
				t.Errorf("NewStep = %v, want %v", err, tc.want)
			}
		})
	}
}
