package spec_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/spec"
)

/*
A run opened on a candidate plans the seal it names and nothing else (NT-018 phase 2).

The seal's bytes are read and checked before any of them is used: the row must name the
run's agent, version and scope, the claim must hold a digest, the bytes must hash to it and
parse to that agent in that scope. Anything else is fuseone:candidate-unreadable, and never
a sibling seal of the same version, never the published version.
*/

const sealedDefinition = `---
id: triage
name: Ticket triage
company: acme
area: cx
provider: openai
model: test-model
effort: high
tools:
  - crm.sealed
budget:
  micros: 500000
  steps: 60
---

Answer from the sealed text.
`

const sealA, sealB = "0b7c3e52-8a43-4b55-9a3f-0d6a3c5f9e11", "1c8d4f63-9b54-4c66-8b4f-1e7b4d6f0f22"

type memorySeals map[string]spec.SealRow

func (m memorySeals) Seal(_ context.Context, id string) (spec.SealRow, error) {
	row, ok := m[id]
	if !ok {
		return spec.SealRow{}, spec.ErrSealNotFound
	}
	return row, nil
}

type candidateFixture struct {
	candidates *spec.Candidates
	content    *engine.MemoryContent
	seals      memorySeals
	sealed     spec.Spec
	providers  experimentProviders
}

func newCandidateFixture(t *testing.T) candidateFixture {
	t.Helper()
	sealed, err := spec.Parse("triage.agent.md", []byte(sealedDefinition))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	content := engine.NewMemoryContent()
	put := func(owner string) string {
		ref, err := content.PutFor(context.Background(), "authoring", owner, 0, []byte(sealedDefinition))
		if err != nil {
			t.Fatalf("PutFor: %v", err)
		}
		return ref
	}
	sum := sha256.Sum256([]byte(sealedDefinition))
	row := spec.SealRow{
		Agent: sealed.ID, Version: sealed.Version, Scope: domain.Scope{Company: "acme", Area: "cx"},
		Digest: hex.EncodeToString(sum[:]),
	}
	a, b := row, row
	a.ContentRef, b.ContentRef = put(sealA), put(sealB)
	seals := memorySeals{sealA: a, sealB: b}
	providers := newExperimentProviders(t)
	// Nothing is published: a candidate that fell back to the published version would
	// find none and fail loudly rather than pass by accident.
	resolver := spec.NewResolver(spec.Local(spec.NewStore()), providers.registry, nil)
	return candidateFixture{
		candidates: resolver.Candidates(seals, content), content: content, seals: seals,
		sealed: sealed, providers: providers,
	}
}

func (f candidateFixture) claim() domain.Claim {
	return domain.Claim{RunID: "run-cand", AgentID: f.sealed.ID, VersionID: f.sealed.Version,
		Scope: domain.Scope{Company: "acme", Area: "cx"}}
}

func candidateOpening(seal string, config *domain.ExperimentConfig) domain.RunStartedPayload {
	return domain.RunStartedPayload{Trigger: "simulation", Simulated: true, Simulation: "exp-1",
		Case: "case-1", Experiment: "exp-1", ExperimentConfig: config, SealID: seal}
}

func TestResolveCandidate_plansTheSealedDefinition(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)

	resolved, err := f.candidates.ResolveCandidate(context.Background(), f.claim(),
		candidateOpening(sealA, &domain.ExperimentConfig{Effort: "low"}))
	if err != nil {
		t.Fatalf("ResolveCandidate: %v", err)
	}
	if tools := resolved.Start.Pack.Tools(); !slices.Equal(tools, []domain.ToolID{"crm.sealed"}) {
		t.Errorf("pack = %v, want the sealed tools", tools)
	}
	plan(t, resolved.Planner)
	if model, effort := f.providers.primary.last(); model != "test-model" || effort != "low" {
		t.Errorf("planned on %q at %q, want the sealed model at the experiment's effort", model, effort)
	}
}

func TestResolveCandidate_anythingThatDoesNotMatchTheSeal_isUnreadable(t *testing.T) {
	t.Parallel()
	cases := map[string]func(f *candidateFixture) (domain.Claim, string){
		"an erased seal": func(f *candidateFixture) (domain.Claim, string) {
			if _, err := f.content.Erase(context.Background(), "authoring", sealA, "test"); err != nil {
				t.Fatalf("Erase: %v", err)
			}
			return f.claim(), sealA
		},
		"a seal nobody wrote": func(f *candidateFixture) (domain.Claim, string) {
			return f.claim(), "9f9f9f9f-9b54-4c66-8b4f-1e7b4d6f0f22"
		},
		"a malformed seal id": func(f *candidateFixture) (domain.Claim, string) {
			return f.claim(), "not-a-seal"
		},
		"a run of another agent": func(f *candidateFixture) (domain.Claim, string) {
			c := f.claim()
			c.AgentID = "other"
			return c, sealA
		},
		"a run of another version": func(f *candidateFixture) (domain.Claim, string) {
			c := f.claim()
			c.VersionID = "v000000000000"
			return c, sealA
		},
		"a run in another scope": func(f *candidateFixture) (domain.Claim, string) {
			c := f.claim()
			c.Scope.Area = "sales"
			return c, sealA
		},
		"bytes that do not hash to the claim": func(f *candidateFixture) (domain.Claim, string) {
			row := f.seals[sealA]
			row.Digest = "ab" + row.Digest[2:]
			f.seals[sealA] = row
			return f.claim(), sealA
		},
		"a claim without a digest": func(f *candidateFixture) (domain.Claim, string) {
			row := f.seals[sealA]
			row.Digest = ""
			f.seals[sealA] = row
			return f.claim(), sealA
		},
		// The run and the bytes agree; only the row records another scope.
		"a row recorded in another scope": func(f *candidateFixture) (domain.Claim, string) {
			row := f.seals[sealA]
			row.Scope.Area = "sales"
			f.seals[sealA] = row
			return f.claim(), sealA
		},
		"a row whose scope is not the sealed text's": func(f *candidateFixture) (domain.Claim, string) {
			row := f.seals[sealA]
			row.Scope.Area = "sales"
			f.seals[sealA] = row
			c := f.claim()
			c.Scope.Area = "sales"
			return c, sealA
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newCandidateFixture(t)
			claim, seal := setup(&f)

			_, err := f.candidates.ResolveCandidate(context.Background(), claim, candidateOpening(seal, nil))
			if !errors.Is(err, domain.ErrCandidateUnreadable) {
				t.Fatalf("err = %v, want ErrCandidateUnreadable", err)
			}
		})
	}
}

// A run is pinned to the seal it opened with: erasing that seal reaches it even though a
// sibling of the same version still holds the same bytes.
func TestResolveCandidate_neverFallsBackToASiblingSeal(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	if _, err := f.content.Erase(context.Background(), "authoring", sealA, "test"); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	if _, err := f.candidates.ResolveCandidate(context.Background(), f.claim(),
		candidateOpening(sealA, nil)); !errors.Is(err, domain.ErrCandidateUnreadable) {
		t.Fatalf("seal A: err = %v, want ErrCandidateUnreadable", err)
	}
	if _, err := f.candidates.ResolveCandidate(context.Background(), f.claim(),
		candidateOpening(sealB, nil)); err != nil {
		t.Fatalf("seal B is readable and must still resolve: %v", err)
	}
}

type failingContent struct{}

func (failingContent) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("connection reset")
}

// A store that cannot answer is not a seal that is gone: the run parks and is tried again,
// instead of ending for good on a transient fault.
func TestResolveCandidate_aContentStoreFault_isNotUnreadable(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	resolver := spec.NewResolver(spec.Local(spec.NewStore()), f.providers.registry, nil)
	candidates := resolver.Candidates(f.seals, failingContent{})

	_, err := candidates.ResolveCandidate(context.Background(), f.claim(), candidateOpening(sealA, nil))
	if err == nil || errors.Is(err, domain.ErrCandidateUnreadable) {
		t.Fatalf("err = %v, want a fault that is not ErrCandidateUnreadable", err)
	}
}

// Every turn reads the seal again: nothing of it is kept between resolutions, so the turn
// after an erasure commits finds the tombstone and plans nothing (NT-018, "Tests that must
// exist").
func TestResolveCandidate_theTurnAfterAnErasure_findsItUnreadable(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	opening := candidateOpening(sealA, nil)
	if _, err := f.candidates.ResolveCandidate(context.Background(), f.claim(), opening); err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if _, err := f.content.Erase(context.Background(), "authoring", sealA, "test"); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	if _, err := f.candidates.ResolveCandidate(context.Background(), f.claim(), opening); !errors.Is(err, domain.ErrCandidateUnreadable) {
		t.Fatalf("next turn: err = %v, want ErrCandidateUnreadable", err)
	}
}
