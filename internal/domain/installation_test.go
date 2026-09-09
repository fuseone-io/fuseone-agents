package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
Authority above any company.

Creating a company cannot be a permission held inside one: the administrator of
acme would mint another company and grant themselves in it, which is not a
tightening anybody would notice. So there is a scope above them all.

It is a named sentinel and emphatically not the zero value. `domain.Scope{}` is
what a struct starts as, what a decoding failure leaves behind, and what half
the calls in this repository pass when a scope is not the point — if that meant
"everything", then every one of those would be a grant of everything, and the
bug would be silent and total. The zero value has to stay the least authority
there is.
*/
func TestContains_installation_reachesEveryScope(t *testing.T) {
	t.Parallel()
	everywhere := domain.Scope{Company: domain.Installation}

	for _, scope := range []domain.Scope{
		{Company: "acme", Area: "finance"},
		{Company: "outra"},
		{Company: "acme"},
	} {
		if !everywhere.Contains(scope) {
			t.Errorf("the installation scope does not reach %s", scope)
		}
	}
}

func TestContains_theZeroScope_reachesNothing(t *testing.T) {
	t.Parallel()
	// The property the whole design rests on. A struct that was never filled
	// in must grant nothing at all.
	var nothing domain.Scope

	for _, scope := range []domain.Scope{
		{Company: "acme", Area: "finance"},
		{Company: "acme"},
		{Company: domain.Installation},
	} {
		if nothing.Contains(scope) {
			t.Errorf("the zero scope reached %s", scope)
		}
	}
}

func TestContains_installationScopeMustHaveNoArea(t *testing.T) {
	t.Parallel()

	malformed := domain.Scope{Company: domain.Installation, Area: "cx"}
	if malformed.Contains(domain.Scope{Company: "acme", Area: "cx"}) {
		t.Error("a malformed installation scope with an area reached everything")
	}
}

// The other direction: holding a company does not let somebody act on the
// installation, which is what stops one company's administrator creating a
// second one.
func TestContains_aCompany_doesNotReachTheInstallation(t *testing.T) {
	t.Parallel()
	acme := domain.Scope{Company: "acme"}

	if acme.Contains(domain.Scope{Company: domain.Installation}) {
		t.Error("a company grant reached the installation")
	}
}

func TestInstallation_isNotAName_anybodyCouldRegister(t *testing.T) {
	t.Parallel()
	// It has to be a string no company can be called, or somebody registers a
	// company with that name and holds the installation.
	if _, ok := domain.ParseScope(string(domain.Installation) + "/anything"); ok {
		t.Error("the sentinel parses as an ordinary company")
	}
	if err := domain.ValidCompanyID(string(domain.Installation)); err == nil {
		t.Error("the sentinel is accepted as a company somebody could create")
	}
}

/*
Naming the scope above every company, rather than spelling it out.

Written by hand each time, this comparison drifts: a check on the company alone
accepts the sentinel carrying an area, which contains nothing at all — the
containment rule short circuits on the sentinel and requires the area to be
empty, so that shape reaches nothing while looking like it reaches everything.

The zero scope has to keep failing it. `Scope{}` is what a struct starts as and
what a failed decode leaves behind, and if it answered yes here every one of
those would be the installation.
*/
func TestIsInstallation_answersOnlyForTheScopeAboveEveryCompany(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		scope domain.Scope
		want  bool
	}{
		{"the sentinel", domain.Scope{Company: domain.Installation}, true},
		{"the zero scope", domain.Scope{}, false},
		{"a company", domain.Scope{Company: "acme"}, false},
		{"an area", domain.Scope{Company: "acme", Area: "ops"}, false},
		// Storable, and it contains nothing: Contains short circuits on the
		// sentinel and answers false unless the area is empty.
		{"the sentinel with an area", domain.Scope{Company: domain.Installation, Area: "cx"}, false},
	} {
		if got := c.scope.IsInstallation(); got != c.want {
			t.Errorf("%s: IsInstallation() = %v, want %v", c.name, got, c.want)
		}
	}
}

// And it agrees with what containment already believes, which is the rule it
// exists to make readable.
func TestIsInstallation_agreesWithWhatContainsEverything(t *testing.T) {
	t.Parallel()

	for _, scope := range []domain.Scope{
		{Company: domain.Installation},
		{Company: domain.Installation, Area: "cx"},
		{},
		{Company: "acme"},
	} {
		reaches := scope.Contains(domain.Scope{Company: "beta", Area: "fin"}) &&
			scope.Contains(domain.Scope{Company: "acme", Area: "ops"})
		if scope.IsInstallation() != reaches {
			t.Errorf("%+v: IsInstallation() = %v but reaches everything = %v",
				scope, scope.IsInstallation(), reaches)
		}
	}
}
