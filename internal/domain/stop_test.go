package domain_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

func TestStop_scopeStop_reachesTheScopesInsideIt(t *testing.T) {
	t.Parallel()

	// Stopping a company stops its areas. Anything else makes the widest
	// switch somebody reaches for in an incident the narrowest one they have.
	stop := domain.Stop{Level: domain.StopScope, Scope: domain.Scope{Company: "acme"}}

	if !stop.Covers(domain.Scope{Company: "acme", Area: "cx"}, "triage") {
		t.Error("a company-wide stop did not reach an area inside it")
	}
	if stop.Covers(domain.Scope{Company: "outra", Area: "cx"}, "triage") {
		t.Error("the stop reached another company")
	}
}

func TestStop_installation_reachesEverything(t *testing.T) {
	t.Parallel()

	stop := domain.Stop{Level: domain.StopInstallation}

	if !stop.Covers(domain.Scope{Company: "qualquer", Area: "coisa"}, "qualquer") {
		t.Error("the installation switch left something running")
	}
}

func TestStop_key_isStableForTheSameTarget(t *testing.T) {
	t.Parallel()

	// Two people stopping the same thing must write one row, not two that
	// disagree about whether it is stopped.
	a := domain.Stop{Level: domain.StopScope, Scope: domain.Scope{Company: "acme", Area: "cx"}, By: "ana"}
	b := domain.Stop{Level: domain.StopScope, Scope: domain.Scope{Company: "acme", Area: "cx"}, By: "bruno"}

	if a.Key() != b.Key() {
		t.Errorf("keys differ: %q and %q", a.Key(), b.Key())
	}
}
