package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
Evidence identity is the citation, not what the citation turned out to mean.

Labels are derived by the resolver from the ledger, so the same step read twice
can arrive carrying different labels — before and after the run that produced it
gained taint, or before and after a reconciliation filled them in. Two records
pointing at the same bytes are one citation, and folding them by anything that
includes the labels would keep both and spend the eight-record budget on a
duplicate.
*/
func TestMemoryEvidenceKey_labelsDoNotChangeIt(t *testing.T) {
	t.Parallel()

	cited := domain.MemoryEvidence{
		RunID: "run-1", Seq: 7, Artifact: "result", Digest: "sha256:abcd",
	}
	labelled := cited
	labelled.Labels = domain.NewLabels(domain.LabelUntrusted)

	if cited.Key() != labelled.Key() {
		t.Errorf("key changed with the labels: %q vs %q", cited.Key(), labelled.Key())
	}
}

// Two steps of one run are two citations. Before Seq existed the digest was the
// only thing telling them apart, and identical bytes stored twice — a retried
// tool call, a cached result — collapsed into one.
func TestMemoryEvidenceKey_seqSeparatesTwoStepsOfOneRun(t *testing.T) {
	t.Parallel()

	first := domain.MemoryEvidence{
		RunID: "run-1", Seq: 4, Artifact: "result", Digest: "sha256:abcd",
	}
	second := first
	second.Seq = 9

	if first.Key() == second.Key() {
		t.Error("two steps of one run share a key")
	}
}

// A context artifact is produced by one run and cited by another, so the source
// is part of what the citation is. Without it, the same artifact read through
// two runs would look like one citation and lose the provenance that explains
// its labels.
func TestMemoryEvidenceKey_sourceRunSeparatesASharedArtifact(t *testing.T) {
	t.Parallel()

	mine := domain.MemoryEvidence{
		RunID: "run-1", SourceRunID: "run-1", Seq: 2, Artifact: "summary", Digest: "sha256:abcd",
	}
	shared := mine
	shared.SourceRunID = "run-upstream"

	if mine.Key() == shared.Key() {
		t.Error("an artifact from another run shares a key with a local one")
	}
}

// Fields must not run together, for the same reason the identity key separates
// them: a run id ending where an artifact begins would otherwise be the same
// citation as the two spelled the other way round.
func TestMemoryEvidenceKey_fieldsDoNotBleedIntoEachOther(t *testing.T) {
	t.Parallel()

	first := domain.MemoryEvidence{RunID: "run-1", Artifact: "ab", Digest: "sha256:x"}
	second := domain.MemoryEvidence{RunID: "run-1a", Artifact: "b", Digest: "sha256:x"}

	if first.Key() == second.Key() {
		t.Error("run id and artifact collided with their concatenation")
	}
}

// Evidence written before this change has no seq and no source run. It keeps
// the identity it always had, so an upgrade does not turn one citation into two.
func TestMemoryEvidenceKey_legacyEvidenceKeepsOneIdentity(t *testing.T) {
	t.Parallel()

	legacy := domain.MemoryEvidence{
		RunID: "run-1", Artifact: "final_answer", Digest: "sha256:abcd",
	}
	same := domain.MemoryEvidence{
		RunID: "run-1", Artifact: "final_answer", Digest: "sha256:abcd",
	}

	if legacy.Key() != same.Key() {
		t.Error("two identical legacy citations disagree")
	}
}

/*
An absent source means the citing run, so saying it out loud must not rename the
citation.

Hydration fills SourceRunID on legacy evidence, and if that changed the key the
repair would mint a duplicate of every record it touched — a second citation of
the same bytes, in the one operation whose job is to make old rows whole.
*/
func TestMemoryEvidenceKey_absentSourceIsTheCitingRun(t *testing.T) {
	t.Parallel()

	legacy := domain.MemoryEvidence{
		RunID: "run-1", Artifact: "final_answer", Digest: "sha256:abcd",
	}
	hydrated := legacy
	hydrated.SourceRunID = "run-1"

	if legacy.Key() != hydrated.Key() {
		t.Errorf("key changed when the source was spelled out: %q vs %q",
			legacy.Key(), hydrated.Key())
	}
}
