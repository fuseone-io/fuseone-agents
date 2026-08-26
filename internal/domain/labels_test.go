package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestScopeLabels_carryCompanyAndAreaTogether(t *testing.T) {
	t.Parallel()

	labels := domain.ScopeLabels(domain.Scope{Company: "acme", Area: "platform"})

	if !labels.Has("company:acme") || !labels.Has("area:acme/platform") {
		t.Fatalf("labels = %v, want company and area", labels)
	}
}

func TestLabels_scopeBoundaryUsesTheSameContainmentAsGrants(t *testing.T) {
	t.Parallel()

	platform := domain.ScopeLabels(domain.Scope{Company: "acme", Area: "platform"})
	for _, target := range []domain.Scope{
		{Company: "acme", Area: "platform"},
		{Company: "acme"},
		{Company: domain.Installation},
	} {
		if got, blocked := platform.ScopeBoundaryViolation(target); blocked {
			t.Errorf("target %s blocked by %v", target, got)
		}
	}
}

func TestLabels_scopeBoundaryBlocksSiblingAreasAndOtherCompanies(t *testing.T) {
	t.Parallel()

	labels := domain.ScopeLabels(domain.Scope{Company: "acme", Area: "platform"})
	for _, target := range []domain.Scope{
		{Company: "acme", Area: "finance"},
		{Company: "globex", Area: "platform"},
		{},
	} {
		if _, blocked := labels.ScopeBoundaryViolation(target); !blocked {
			t.Errorf("target %s accepted acme/platform data", target)
		}
	}
}

func TestLabels_companyWideDataDoesNotEnterAnAreaByAccident(t *testing.T) {
	t.Parallel()

	labels := domain.ScopeLabels(domain.Scope{Company: "acme"})

	if _, blocked := labels.ScopeBoundaryViolation(domain.Scope{Company: "acme", Area: "platform"}); !blocked {
		t.Fatal("company-wide data entered an area scope")
	}
}

func TestLabels_malformedReservedScopeLabelFailsClosed(t *testing.T) {
	t.Parallel()

	labels := domain.NewLabels("company:*")

	violation, blocked := labels.ScopeBoundaryViolation(domain.Scope{Company: domain.Installation})
	if !blocked {
		t.Fatal("malformed reserved label passed")
	}
	if violation.Origin != (domain.Scope{}) {
		t.Fatalf("origin = %v, want zero for malformed label", violation.Origin)
	}
}
