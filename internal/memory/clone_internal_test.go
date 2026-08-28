package memory

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func cited(run domain.RunID, seq int64, labels ...string) domain.MemoryEvidence {
	ev := domain.MemoryEvidence{
		RunID: run, Seq: seq, Artifact: "result", Digest: "sha256:abcd",
	}
	if len(labels) > 0 {
		ev.Labels = domain.NewLabels(labels...)
	}
	return ev
}

/*
The same step cited twice is one citation, and its labels are the union.

Evidence arrives from the resolver, which reads the ledger at the moment it is
asked. The same step can come back clean today and tainted after the run that
produced it gained a label, or empty before a reconciliation filled it in.
Keeping both would spend the eight-record budget on a duplicate of itself;
keeping the first would let a later, more complete reading be thrown away.
*/
func TestBoundedEvidence_sameCitationTwice_isOneRecordWithBothLabels(t *testing.T) {
	t.Parallel()

	got := boundedEvidence([]domain.MemoryEvidence{
		cited("run-1", 4),
		cited("run-1", 4, domain.LabelUntrusted),
	})

	if len(got) != 1 {
		t.Fatalf("kept %d records, want the two readings folded into one", len(got))
	}
	if !got[0].Labels.HasAny(domain.LabelUntrusted) {
		t.Errorf("labels = %v, want the taint the second reading carried", got[0].Labels)
	}
}

// Two steps of one run are two citations. Before Seq the digest was the only
// thing telling them apart, so identical bytes stored twice collapsed into one.
func TestBoundedEvidence_twoStepsOfOneRun_areTwoRecords(t *testing.T) {
	t.Parallel()

	got := boundedEvidence([]domain.MemoryEvidence{
		cited("run-1", 4),
		cited("run-1", 9),
	})

	if len(got) != 2 {
		t.Fatalf("kept %d records, want both steps", len(got))
	}
}

func TestBoundedEvidence_neverKeepsMoreThanTheCap(t *testing.T) {
	t.Parallel()

	in := make([]domain.MemoryEvidence, 0, domain.MaxMemoryEvidence+3)
	for seq := range int64(domain.MaxMemoryEvidence + 3) {
		in = append(in, cited("run-1", seq))
	}

	if got := boundedEvidence(in); len(got) != domain.MaxMemoryEvidence {
		t.Errorf("kept %d records, want the %d-record cap", len(got), domain.MaxMemoryEvidence)
	}
}

/*
A copy that shares the evidence labels is not a copy.

The in-memory store hands out what it holds, so a reader that mutated the labels
of a returned citation would be editing the stored taint in place — the one
thing the whole provenance chain rests on. The assertion's own labels were
already cloned; the ones inside each citation were not.
*/
func TestCloneAssertion_evidenceLabelsAreNotShared(t *testing.T) {
	t.Parallel()

	stored := domain.MemoryAssertion{
		Evidence: []domain.MemoryEvidence{cited("run-1", 4, domain.LabelUntrusted)},
	}

	copied := cloneAssertion(stored)
	copied.Evidence[0].Labels[0] = domain.LabelPersonal

	if stored.Evidence[0].Labels[0] != domain.LabelUntrusted {
		t.Errorf("stored label became %q, want the copy to own its labels",
			stored.Evidence[0].Labels[0])
	}
}

func TestCloneSuggestion_evidenceLabelsAreNotShared(t *testing.T) {
	t.Parallel()

	stored := domain.MemorySuggestion{
		Evidence: []domain.MemoryEvidence{cited("run-1", 4, domain.LabelUntrusted)},
	}

	copied := cloneSuggestion(stored)
	copied.Evidence[0].Labels[0] = domain.LabelPersonal

	if stored.Evidence[0].Labels[0] != domain.LabelUntrusted {
		t.Errorf("stored label became %q, want the copy to own its labels",
			stored.Evidence[0].Labels[0])
	}
}
