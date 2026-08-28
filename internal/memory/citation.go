package memory

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// What a step says about the bytes it named, in the three forms that exist.
//
// Reading, not deciding. Two of them live on run_finished — the closing answer
// and a published artifact — and the third is the arguments of a memory
// suggestion, which points at a tool call rather than at the end of a run.

// citation is what the ledger says about the bytes a step named.
type citation struct {
	ref       string
	digest    string
	sourceRun domain.RunID
}

// citedStep locates the step and reads the citation out of its payload. A
// citation with no seq is the older shape, which named a run and an artifact and
// left the platform to find it.
func citedStep(steps []domain.Step, ev domain.MemoryEvidence) (int, citation, error) {
	for at := len(steps) - 1; at >= 0; at-- {
		if ev.Seq > 0 && steps[at].Seq != ev.Seq {
			continue
		}
		cite, ok := citationIn(steps[at], ev.Artifact)
		if !ok {
			continue
		}
		// Whenever the citation carries a digest, seq or no seq. Without a seq it
		// is also what tells one step from another answering to the same
		// artifact, and conditioning the comparison on that need is what let a
		// citation that names its step claim any digest at all: needing
		// something for disambiguation and needing it to be true are different
		// requirements, and only the first of them depends on the seq.
		if ev.Digest != "" && !sameDigest(ev.Digest, cite.digest) {
			continue
		}
		return at, cite, nil
	}
	return 0, citation{}, fmt.Errorf("%w: no such step in the run", ErrEvidenceInvalid)
}

/*
citationIn reads one step's payload for the artifact a citation names.

Three forms exist today. A run's closing answer and a published artifact both
live on run_finished; the third is the arguments of a memory suggestion, which
the agent's own suggest path has always cited and which points at a tool call
rather than at the end of the run.

The tool, the digest and the reference all have to agree on that third form. Two
of the three matching is a citation that names a real reference in the same run
and means something else.
*/
func citationIn(step domain.Step, artifact string) (citation, bool) {
	switch step.Kind {
	case domain.StepRunFinished:
		var p domain.RunFinishedPayload
		if json.Unmarshal(step.Payload, &p) != nil {
			return citation{}, false
		}
		if artifact == domain.ArtifactFinalAnswer {
			return proved(citation{ref: p.OutcomeRef, digest: p.OutcomeDigest})
		}
		for _, a := range p.Artifacts {
			if a.Name == artifact {
				return proved(citation{ref: a.Ref, digest: a.Digest, sourceRun: a.SourceRun})
			}
		}

	case domain.StepToolCalled:
		if artifact != domain.ArtifactMemorySuggestion {
			return citation{}, false
		}
		var p domain.ToolCalledPayload
		if json.Unmarshal(step.Payload, &p) != nil {
			return citation{}, false
		}
		if p.Tool != domain.ToolMemorySuggest {
			return citation{}, false
		}
		return proved(citation{ref: p.ArgsRef, digest: p.ArgsDigest})
	}
	return citation{}, false
}

/*
proved refuses a citation the ledger only half recorded.

Reference and digest are written together about the same bytes, so one without
the other is not a shape production writes.

Not load-bearing, deliberately: an empty digest would also fail the comparison
in prove, since the store's digest can never equal "". This refuses earlier and
says why — "the ledger recorded no digest" rather than "the digest disagrees",
which are different things for whoever reads the error. The guarantee lives in
prove; this is the sentence that explains it.
*/
func proved(c citation) (citation, bool) {
	return c, c.ref != "" && c.digest != ""
}

/*
sameDigest compares what the ledger recorded with what the store holds.

Two spellings of the same number are in circulation. Some payloads carry the
"sha256:" prefix and some do not, and the engine's own digest helper truncates
to sixteen hex — which is what a memory suggestion's citation has always
carried. A citation must not fail to resolve because of a prefix or because the
part it kept is shorter than the part it is being compared to.
*/
func sameDigest(a, b string) bool {
	x, ok := digestForm(a)
	if !ok {
		return false
	}
	y, ok := digestForm(b)
	if !ok {
		return false
	}
	if len(x) > len(y) {
		x, y = y, x
	}
	return y[:len(x)] == x
}

// digestForm accepts the two spellings that exist and nothing else: the whole
// SHA-256, and the sixteen hex the engine's digest helper keeps. Any other
// length would let a shorter prefix stand in as proof.
func digestForm(v string) (string, bool) {
	v = trimDigest(v)
	if len(v) != 64 && len(v) != 16 {
		return "", false
	}
	if _, err := hex.DecodeString(v); err != nil {
		return "", false
	}
	return v, true
}

func trimDigest(v string) string {
	if len(v) > 7 && v[:7] == "sha256:" {
		return v[7:]
	}
	return v
}
