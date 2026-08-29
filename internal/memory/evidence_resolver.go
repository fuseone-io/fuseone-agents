package memory

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

// Proving a citation against the ledger.
//
// A memory says it saw something; this is what checks that the run said so too.
// The order is the whole design: the ledger first, then the scope, then the step
// and the artifact, and only then the content store — a citation that cannot
// name a real step never becomes a reference somebody resolves.

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

// EvidenceOrigin is one proved citation and the agent of the run that cited
// it. AgentID is response metadata, not part of MemoryEvidence: it decides who
// may recall an agent-scoped memory and is never persisted as if the caller had
// supplied it.
type EvidenceOrigin struct {
	Evidence domain.MemoryEvidence
	AgentID  domain.AgentID
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
	origins, err := r.ResolveWithOrigins(ctx, scope, in)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MemoryEvidence, 0, len(origins))
	for _, origin := range origins {
		out = append(out, origin.Evidence)
	}
	return out, nil
}

// ResolveWithOrigins proves the same citations as Resolve and also returns
// whose run each one names. Both answers come from the same cached ledger read;
// a creation used to prove the citation and then read the run again only to
// recover this agent.
func (r *Resolver) ResolveWithOrigins(
	ctx context.Context, scope domain.Scope, in []domain.MemoryEvidence,
) ([]EvidenceOrigin, error) {
	runs := map[domain.RunID][]domain.Step{}
	out := make([]EvidenceOrigin, 0, len(in))

	for _, ev := range in {
		steps, err := r.stepsOf(ctx, runs, scope, ev.RunID)
		if err != nil {
			return nil, err
		}
		resolved, err := r.prove(ctx, steps, ev)
		if err != nil {
			return nil, err
		}
		out = append(out, EvidenceOrigin{
			Evidence: resolved,
			AgentID:  steps[0].AgentID,
		})
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
