package httpapi

import (
	"context"
	"errors"
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
originOfMemoryEvidence proves what a creation cites, and reads who it belongs to.

The proving is the Resolver's, which is the whole reason it exists: it reads the
run, checks the scope, finds the step and the artifact, asks the content store
whether the bytes are still there, and answers with the digest, the step, the
run that produced it, and the labels the run had accumulated by then.

This used to do a smaller version of that by hand — matching an artifact name in
the run_finished payload and trusting what it found. It never asked the content
store, so a reference to bytes that were erased or never stored sustained an
active memory. And the citation it built carried no step and no labels, which is
the legacy shape the whole of PR 1 exists to fill in: a memory created that way
is one the reconciliation job has to repair, and that job runs on a release
rather than after every creation. A citation that carries no labels also cannot
explain the taint the assertion inherited, so the eviction rule could drop the
one record that showed where it came from.

The agent is the one thing the Resolver does not answer, so it is still read
here — from the first step of the run, which is where a run says whose it is.
*/
func (s *Server) originOfMemoryEvidence(
	ctx context.Context, scope domain.Scope,
	namespace openapi.MemoryAssertionInputNamespace, evidence []openapi.MemoryEvidenceInput,
) (domain.AgentID, domain.Labels, []domain.MemoryEvidence, error) {
	if s.memoryEvidence == nil {
		return "", nil, nil, errNoEvidenceResolver
	}
	refs := make([]domain.MemoryEvidence, 0, len(evidence))
	for _, ev := range evidence {
		refs = append(refs, domain.MemoryEvidence{
			RunID: domain.RunID(ev.RunId), Artifact: citedArtifact(ev),
		})
	}
	cited, err := s.memoryEvidence.Resolve(ctx, scope, refs)
	if err != nil {
		return "", nil, nil, fmt.Errorf("memory evidence: %w", err)
	}

	var labels domain.Labels
	agents := map[domain.AgentID]bool{}
	for i, ev := range cited {
		agent, err := s.agentOfRun(ctx, scope, refs[i].RunID)
		if err != nil {
			return "", nil, nil, err
		}
		agents[agent] = true
		labels = labels.Union(ev.Labels)
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

// errNoEvidenceResolver is an installation that wired memory without what
// proves it. A refusal rather than a fallback: writing a memory without
// checking the run it cites is what this path exists to prevent.
var errNoEvidenceResolver = errors.New(
	"httpapi: no evidence resolver to prove this memory against")

// agentOfRun is whose run this was, which the ledger records on the step that
// opened it. The Resolver answers everything else about a citation and not
// this, because it is a fact about the run rather than about the bytes.
func (s *Server) agentOfRun(
	ctx context.Context, scope domain.Scope, run domain.RunID,
) (domain.AgentID, error) {
	steps, err := s.store.Read(ctx, run, domain.FirstSeq)
	if err != nil || len(steps) == 0 || !scope.Contains(steps[0].Scope) {
		return "", fmt.Errorf("memory evidence is outside scope or absent")
	}
	return steps[0].AgentID, nil
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
