package memory

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
ErrEvidenceInvalid means the citation is wrong, not that the platform is unwell.

The reconciliation job repairs rows one at a time and has to tell those apart:
a citation that does not match the ledger is refused and the job moves on, an
unavailable dependency is tried again. Collapsing them would make the job skip
rows it should have retried and record that it had checked them.

So every semantic refusal below wraps this, and every infrastructure failure
wraps the error it came from instead.
*/
var ErrEvidenceInvalid = errors.New("memory: evidence does not match the ledger")

/*
ErrEvidenceSourceAbsent means the source a citation rests on has been taken:
the run is no longer in the ledger, or the bytes it produced were erased.

Distinct from an invalid citation because the answers differ. A citation that
never matched is a mistake to refuse; a source that existed and is gone has a
status of its own, and the platform ends the memory rather than leaving it
readable. Filing an erasure under the first left active memory whose bytes we
know were deleted, with a sweep that would never converge it — and that state is
reachable without anybody doing anything strange, since erasing content and
marking the memories are two transactions and only the first may have run.

A reference that never held anything is the other case and stays invalid: the
bytes were not taken, the citation names somewhere they never were, and ending a
memory on that would record a retention event for a mistake.

Distinct from infrastructure because the store answered. It said the source is
not there, which is a fact, not a failure to reach it.
*/
var ErrEvidenceSourceAbsent = errors.New("memory: the source the evidence names is gone")

/*
EvidenceLedger and EvidenceContent are what proving a citation needs, declared
here because this is what uses them.

Deliberately two small interfaces rather than the stores themselves: the
resolver reads and never writes, and a type that cannot append cannot be the
thing that quietly starts appending.
*/
type EvidenceLedger interface {
	Read(ctx context.Context, run domain.RunID, from int64) ([]domain.Step, error)
}

type EvidenceContent interface {
	Metadata(ctx context.Context, ref string) (domain.ContentMetadata, error)
}

/*
Resolver turns a citation somebody typed into a citation the ledger vouches for.

Kept apart from Store on purpose: storing a memory and proving where it came
from are two jobs, and the one that must never write is safer as a type that
cannot. It is also why this reads through interfaces rather than holding a pool.

Nothing a caller sends about the content survives. The run, the step and the
artifact are what a caller chooses; the digest, the labels and the source run
are read out of the ledger, and a caller that sent its own gets them replaced.
That is the whole point: a citation is checked against what was recorded, never
against what it claims about itself.
*/
type Resolver struct {
	ledger  EvidenceLedger
	content EvidenceContent
}

func NewResolver(ledger EvidenceLedger, content EvidenceContent) *Resolver {
	return &Resolver{ledger: ledger, content: content}
}

/*
Resolve proves every citation and returns them complete.

The whole set at once because citations cluster: three steps of one run is the
ordinary case once a step is citable, and resolving them one at a time would
fold the same run three times to answer the same question. The cache lives in
the call rather than on the Resolver, so a long-lived resolver never answers
from a run somebody has since advanced.
*/
func (r *Resolver) Resolve(
	ctx context.Context, scope domain.Scope, in []domain.MemoryEvidence,
) ([]domain.MemoryEvidence, error) {
	runs := map[domain.RunID][]domain.Step{}
	out := make([]domain.MemoryEvidence, 0, len(in))

	for _, ev := range in {
		steps, err := r.stepsOf(ctx, runs, scope, ev.RunID)
		if err != nil {
			return nil, err
		}
		resolved, err := r.prove(ctx, steps, ev)
		if err != nil {
			return nil, err
		}
		out = append(out, resolved)
	}
	return out, nil
}

// stepsOf reads a run once per resolution and checks it belongs to the caller's
// scope before anything else looks at it.
func (r *Resolver) stepsOf(
	ctx context.Context, cache map[domain.RunID][]domain.Step,
	scope domain.Scope, run domain.RunID,
) ([]domain.Step, error) {
	if held, ok := cache[run]; ok {
		return held, nil
	}
	steps, err := r.ledger.Read(ctx, run, domain.FirstSeq)
	if errors.Is(err, domain.ErrRunNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrEvidenceSourceAbsent, run)
	}
	if err != nil {
		return nil, fmt.Errorf("memory: read run %s for evidence: %w", run, err)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrEvidenceSourceAbsent, run)
	}
	if !scope.Contains(steps[0].Scope) {
		return nil, fmt.Errorf("%w: run %s is outside the scope", ErrEvidenceInvalid, run)
	}
	cache[run] = steps
	return steps, nil
}

/*
prove finds the step a citation names and checks it says what the citation says.

The order matters and is the point: the ledger decides which reference this is
before the content store is asked anything. Asking the store first would let a
caller name any reference it knew and have the answer confirm it.
*/
func (r *Resolver) prove(
	ctx context.Context, steps []domain.Step, ev domain.MemoryEvidence,
) (domain.MemoryEvidence, error) {
	at, cite, err := citedStep(steps, ev)
	if err != nil {
		return domain.MemoryEvidence{}, err
	}

	meta, err := r.content.Metadata(ctx, cite.ref)
	if err != nil {
		// Absent content is a wrong citation; anything else is the store being
		// away, and the difference is what a retry is decided on.
		if errors.Is(err, domain.ErrContentAbsent) {
			return domain.MemoryEvidence{}, fmt.Errorf("%w: nothing stored at the reference", ErrEvidenceInvalid)
		}
		return domain.MemoryEvidence{}, fmt.Errorf("memory: read content metadata: %w", err)
	}
	if meta.Erased {
		return domain.MemoryEvidence{}, fmt.Errorf("%w: the content was erased", ErrEvidenceSourceAbsent)
	}
	if !sameDigest(cite.digest, meta.Digest) {
		return domain.MemoryEvidence{}, fmt.Errorf("%w: the digest disagrees with the ledger", ErrEvidenceInvalid)
	}

	return domain.MemoryEvidence{
		RunID:       ev.RunID,
		SourceRunID: cite.sourceRun,
		Seq:         steps[at].Seq,
		Artifact:    ev.Artifact,
		Digest:      meta.Digest,
		Labels:      labelsUpTo(steps, at),
	}, nil
}

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

// labelsUpTo folds the run to the step cited. What a step produced is not what
// the run knew: a clean tool result inside a poisoned run is still a fact the
// poison reached, and remembering it as clean is the inference the Gate refuses
// to make.
func labelsUpTo(steps []domain.Step, at int) domain.Labels {
	var out domain.Labels
	for i := 0; i <= at; i++ {
		out = out.Union(steps[i].Labels)
	}
	return out
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
