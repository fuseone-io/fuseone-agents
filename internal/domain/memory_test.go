package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestMemoryAssertionValidate(t *testing.T) {
	valid := domain.MemoryAssertion{
		Scope:     domain.Scope{Company: "acme", Area: "ops"},
		Kind:      "diagnosis",
		Subject:   "checkout latency",
		Signature: "grafana:checkout:5xx",
		Claim:     "checkout returned 5xx while the database pool was exhausted",
		Evidence: []domain.MemoryEvidence{{
			RunID: "run-1", Artifact: "summary", Digest: "sha256:1234",
		}},
		Observations: 3,
		Confirmed:    2,
		Status:       domain.MemoryActive,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid memory assertion refused: %v", err)
	}

	cases := map[string]func(domain.MemoryAssertion) domain.MemoryAssertion{
		"scope required": func(a domain.MemoryAssertion) domain.MemoryAssertion {
			a.Scope = domain.Scope{}
			return a
		},
		"kind is closed": func(a domain.MemoryAssertion) domain.MemoryAssertion {
			a.Kind = "bad.kind"
			return a
		},
		"evidence is required": func(a domain.MemoryAssertion) domain.MemoryAssertion {
			a.Evidence = nil
			return a
		},
		"counts are measured not guessed": func(a domain.MemoryAssertion) domain.MemoryAssertion {
			a.Confirmed = a.Observations + 1
			return a
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := mutate(valid).Validate(); err == nil {
				t.Fatalf("invalid memory assertion was accepted")
			}
		})
	}
}

func TestMemoryAssertionID_usesPlatformScopeAndAgent(t *testing.T) {
	base := domain.MemoryAssertion{
		Scope:     domain.Scope{Company: "acme", Area: "ops"},
		AgentID:   "triage",
		Kind:      "diagnosis",
		Subject:   "checkout latency",
		Signature: "grafana:checkout:5xx",
	}
	otherScope := base
	otherScope.Scope.Area = "security"
	otherAgent := base
	otherAgent.AgentID = "billing"

	if domain.MemoryAssertionID(base) == domain.MemoryAssertionID(otherScope) {
		t.Fatal("memory id ignored scope; that would couple two areas")
	}
	if domain.MemoryAssertionID(base) == domain.MemoryAssertionID(otherAgent) {
		t.Fatal("memory id ignored agent; that would couple two agents")
	}
}

func TestMemoryFindLimit(t *testing.T) {
	if got := domain.MemoryFindLimit(0); got != domain.MaxMemoryFindLimit {
		t.Fatalf("default limit = %d", got)
	}
	if got := domain.MemoryFindLimit(domain.MaxMemoryFindLimit + 10); got != domain.MaxMemoryFindLimit {
		t.Fatalf("capped limit = %d", got)
	}
	if got := domain.MemoryFindLimit(3); got != 3 {
		t.Fatalf("explicit limit = %d", got)
	}
}

func TestMemoryStatusValid(t *testing.T) {
	if !domain.MemorySourceErased.Valid() {
		t.Fatal("source_erased should be a known status")
	}
	if domain.MemoryStatus("fresh").Valid() {
		t.Fatal("unknown memory status accepted")
	}
}
