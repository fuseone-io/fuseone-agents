package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	memstore "github.com/fuseone/agents/internal/memory"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// The contract's shapes and the domain's, in both directions.
//
// Conversions, and the one reading that is not: labels come from the ledger the
// evidence names and never from what the caller sent, which is checked here
// because here is where the caller's shape stops being trusted.

/*
originOfMemoryEvidence reads the ledger for everything a creation does not get
to assert about itself: the labels of every citation, unioned, and which agent
the memory belongs to.

Both come from the same read, because they are answers to the same question —
what actually happened — and separating them would be two chances for the memory
to be filed against one run and coloured by another.

What "which agent" means depends on the namespace, and the two are opposites.
An agent-scoped memory belongs to one agent, so citations naming two of them are
refused rather than attributed to whichever came first, and a run naming none is
refused rather than widened into memory every agent reads. Shared memory belongs
to no agent, so two agents observing the same fact is not a conflict — it is what
shared memory is, and refusing it made shared memory impossible to correct once
a second agent had contributed a citation to it.
*/
func (s *Server) originOfMemoryEvidence(
	ctx context.Context, scope domain.Scope,
	namespace openapi.MemoryAssertionInputNamespace, evidence []openapi.MemoryEvidenceInput,
) (domain.AgentID, domain.Labels, []domain.MemoryEvidence, error) {
	var labels domain.Labels
	agents := map[domain.AgentID]bool{}
	cited := make([]domain.MemoryEvidence, 0, len(evidence))

	for _, ev := range evidence {
		agent, seen, digest, err := s.memoryEvidenceOrigin(ctx, scope, ev)
		if err != nil {
			return "", nil, nil, err
		}
		agents[agent] = true
		labels = labels.Union(seen)
		cited = append(cited, domain.MemoryEvidence{
			RunID: domain.RunID(ev.RunId), Artifact: citedArtifact(ev), Digest: digest,
		})
	}

	if namespace == openapi.MemoryAssertionInputNamespaceShared {
		return "", labels, cited, nil
	}
	if len(agents) != 1 {
		return "", nil, nil, fmt.Errorf(
			"memory evidence names %d agents; one is required for agent memory", len(agents))
	}
	for agent := range agents {
		if agent == "" {
			return "", nil, nil, fmt.Errorf("the run this evidence names has no agent")
		}
		return agent, labels, cited, nil
	}
	return "", nil, nil, fmt.Errorf("memory evidence names no run")
}

// citedArtifact is which of a run's outputs a citation names. The closing
// answer by default: it is what a memory taught from a run almost always cites,
// and the console's ordinary path never names anything else.
func citedArtifact(ev openapi.MemoryEvidenceInput) string {
	if named := valueOr(ev.Artifact); named != "" {
		return named
	}
	return domain.ArtifactFinalAnswer
}

func (s *Server) memoryEvidenceOrigin(
	ctx context.Context, scope domain.Scope, ev openapi.MemoryEvidenceInput,
) (domain.AgentID, domain.Labels, string, error) {
	steps, err := s.store.Read(ctx, domain.RunID(ev.RunId), domain.FirstSeq)
	if err != nil || len(steps) == 0 || !scope.Contains(steps[0].Scope) {
		return "", nil, "", fmt.Errorf("memory evidence is outside scope or absent")
	}
	for _, step := range steps {
		if step.Kind != domain.StepRunFinished {
			continue
		}
		if labels, digest, ok := finishedArtifactLabels(step, ev); ok {
			return steps[0].AgentID, labels, digest, nil
		}
	}
	return "", nil, "", fmt.Errorf("memory evidence artifact does not match the ledger")
}

/*
finishedArtifactLabels reads the step for the artifact a citation names, and
answers with what the ledger holds rather than checking what the caller sent.

The digest used to be part of the request, and matching on it was how a citation
was tied to one artifact. It was also sixty-four characters somebody had to copy
out of a screen into a form, to be compared against the record it came from —
which is a person doing by hand what the platform is about to do anyway. The
artifact name is what distinguishes them; the digest is what the ledger says
about it.
*/
func finishedArtifactLabels(
	step domain.Step, ev openapi.MemoryEvidenceInput,
) (domain.Labels, string, bool) {
	var p domain.RunFinishedPayload
	if err := json.Unmarshal(step.Payload, &p); err != nil {
		return nil, "", false
	}
	wanted := citedArtifact(ev)
	if wanted == domain.ArtifactFinalAnswer && p.OutcomeDigest != "" {
		return step.Labels.Clone(), p.OutcomeDigest, true
	}
	for _, artifact := range p.Artifacts {
		if artifact.Name == wanted {
			return artifact.Labels.Clone(), artifact.Digest, true
		}
	}
	return nil, "", false
}

/*
memoryAssertionInput is the caller's half of a memory, and only that half.

The agent, the labels, the counters and the expiry come from the platform.
Every one of them changes what the memory means — which runs may recall it, how
the Gate treats what it says, how it ranks against its neighbours, and how long
it lasts — and a caller able to assert any of them about itself could write a
memory that outranks, outlives and out-trusts the ones the platform derived.

The counters start at one because a person recording something has seen it once.
They move afterwards through observations the agent actually makes.
*/
func memoryAssertionInput(
	in openapi.MemoryAssertionInput, scope domain.Scope, derived memoryOrigin,
) domain.MemoryAssertion {
	expires := derived.now.Add(memstore.DefaultMemoryTTL)
	return domain.MemoryAssertion{
		Scope: scope, AgentID: derived.agent,
		Kind: in.Kind, Subject: in.Subject, Signature: in.Signature, Claim: in.Claim,
		Evidence: derived.cited, Observations: 1,
		Confirmed: 1, Labels: derived.labels, Status: domain.MemoryActive,
		ExpiresAt: &expires,
	}
}

/*
memoryOrigin is what the ledger and the clock say about a creation, as opposed
to what the request says.

The agent is read from the run the evidence names rather than accepted from the
body. Every creation cites a run — evidence is required and names one — so there
is always a ledger answer, and taking it means an agent-scoped memory cannot be
filed against an agent whose run never produced it.
*/
type memoryOrigin struct {
	agent domain.AgentID
	// cited is the citation as the ledger describes it, digest included. What
	// the request named is a run and an artifact; what a memory carries is what
	// that run actually produced.
	cited  []domain.MemoryEvidence
	labels domain.Labels
	now    time.Time
}

func memoryAssertions(in []domain.MemoryAssertion) []openapi.MemoryAssertion {
	out := make([]openapi.MemoryAssertion, 0, len(in))
	for _, a := range in {
		out = append(out, memoryAssertion(a))
	}
	return out
}

func memoryAssertion(a domain.MemoryAssertion) openapi.MemoryAssertion {
	return openapi.MemoryAssertion{
		Id: string(a.ID), Scope: openapi.Scope{
			Company: string(a.Scope.Company), Area: string(a.Scope.Area),
		},
		AgentId: string(a.AgentID), Kind: a.Kind, Subject: a.Subject,
		Signature: a.Signature, Claim: a.Claim,
		Evidence: memoryEvidenceTo(a.Evidence), Observations: a.Observations,
		Confirmed: a.Confirmed, Labels: []string(a.Labels),
		Status: openapi.MemoryStatus(a.Status), ExpiresAt: a.ExpiresAt,
		CreatedBy: string(a.CreatedBy), CreatedAt: a.CreatedAt,
		UpdatedBy: string(a.UpdatedBy), UpdatedAt: a.UpdatedAt,
	}
}

func memorySuggestions(in []domain.MemorySuggestion) []openapi.MemorySuggestion {
	out := make([]openapi.MemorySuggestion, 0, len(in))
	for _, s := range in {
		out = append(out, memorySuggestion(s))
	}
	return out
}

func memorySuggestion(s domain.MemorySuggestion) openapi.MemorySuggestion {
	return openapi.MemorySuggestion{
		Id: s.ID, AssertionId: s.AssertionID, Scope: openapi.Scope{
			Company: string(s.Scope.Company), Area: string(s.Scope.Area),
		},
		AgentId: string(s.AgentID), Kind: s.Kind, Subject: s.Subject,
		Signature: s.Signature, Claim: s.Claim,
		Evidence: memoryEvidenceTo(s.Evidence), Observations: s.Observations,
		Labels: []string(s.Labels), Status: openapi.MemorySuggestionStatus(s.Status),
		ExpiresAt: s.ExpiresAt, CreatedBy: string(s.CreatedBy), CreatedAt: s.CreatedAt,
		UpdatedBy: string(s.UpdatedBy), UpdatedAt: s.UpdatedAt,
	}
}

func memoryEvidenceTo(in []domain.MemoryEvidence) []openapi.MemoryEvidence {
	out := make([]openapi.MemoryEvidence, 0, len(in))
	for _, ev := range in {
		out = append(out, openapi.MemoryEvidence{
			RunId: string(ev.RunID), Artifact: ev.Artifact, Digest: ev.Digest,
		})
	}
	return out
}

/*
memoryRefusal says which kind of no the store gave, or zero when it is not one.

Three answers used to be one. A body the server would not accept, a state that
contradicts the write, and a database that is not answering all left here as
400 with a sentence — so the console offered "check what you typed" to somebody
whose installation was down, and the same thing to somebody whose memory holds
two rows claiming one identity, which is the only one of the three a person can
go and fix.

Zero is deliberately the answer for an error this package does not recognise.
An unrecognised failure is the installation's, and it belongs in the logs as a
failure rather than on the screen as the caller's mistake.
*/
