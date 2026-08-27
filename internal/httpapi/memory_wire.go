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
to assert about itself.

The labels of every citation, unioned, and the agent whose run produced them.
Both come from the same read, because they are answers to the same question —
what actually happened — and separating them would be two chances for the
memory to be filed against one run and coloured by another.

Citations from more than one agent's runs are refused. An agent-scoped memory
belongs to one agent, and picking whichever run came first would decide that
silently.
*/
func (s *Server) originOfMemoryEvidence(
	ctx context.Context, scope domain.Scope, evidence []openapi.MemoryEvidence,
) (domain.AgentID, domain.Labels, error) {
	var labels domain.Labels
	var agent domain.AgentID
	for i, ev := range evidence {
		seenAgent, seen, err := s.memoryEvidenceOrigin(ctx, scope, ev)
		if err != nil {
			return "", nil, err
		}
		if i > 0 && seenAgent != agent {
			return "", nil, fmt.Errorf("memory evidence names more than one agent")
		}
		agent, labels = seenAgent, labels.Union(seen)
	}
	return agent, labels, nil
}

func (s *Server) memoryEvidenceOrigin(
	ctx context.Context, scope domain.Scope, ev openapi.MemoryEvidence,
) (domain.AgentID, domain.Labels, error) {
	steps, err := s.store.Read(ctx, domain.RunID(ev.RunId), domain.FirstSeq)
	if err != nil || len(steps) == 0 || !scope.Contains(steps[0].Scope) {
		return "", nil, fmt.Errorf("memory evidence is outside scope or absent")
	}
	for _, step := range steps {
		if step.Kind != domain.StepRunFinished {
			continue
		}
		if labels, ok := finishedArtifactLabels(step, ev); ok {
			return steps[0].AgentID, labels, nil
		}
	}
	return "", nil, fmt.Errorf("memory evidence artifact does not match the ledger")
}

func finishedArtifactLabels(step domain.Step, ev openapi.MemoryEvidence) (domain.Labels, bool) {
	var p domain.RunFinishedPayload
	if err := json.Unmarshal(step.Payload, &p); err != nil {
		return nil, false
	}
	if ev.Artifact == domain.ArtifactFinalAnswer && ev.Digest == p.OutcomeDigest {
		return step.Labels.Clone(), true
	}
	for _, artifact := range p.Artifacts {
		if artifact.Name == ev.Artifact && artifact.Digest == ev.Digest {
			return artifact.Labels.Clone(), true
		}
	}
	return nil, false
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
		Evidence: memoryEvidenceFrom(in.Evidence), Observations: 1,
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
	agent  domain.AgentID
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

func memoryEvidenceFrom(in []openapi.MemoryEvidence) []domain.MemoryEvidence {
	out := make([]domain.MemoryEvidence, 0, len(in))
	for _, ev := range in {
		out = append(out, domain.MemoryEvidence{
			RunID: domain.RunID(ev.RunId), Artifact: ev.Artifact, Digest: ev.Digest,
		})
	}
	return out
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
