package gate

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
A run reaches only the tools of its own scope, or of a scope above it (NT-019).

The check is the Gate's, so the refusal is a verdict the ledger records, not a
call that fails somewhere underneath without a reason anybody can read.
*/
func TestEvaluate_toolScope(t *testing.T) {
	t.Parallel()
	for _, one := range []struct {
		name    string
		tool    domain.Scope
		allowed bool
	}{
		{"the installation's tool", domain.Scope{Company: domain.Installation}, true},
		{"the company's tool", domain.Scope{Company: "acme"}, true},
		{"the run's own area", domain.Scope{Company: "acme", Area: "cx"}, true},
		{"a sibling area's tool", domain.Scope{Company: "acme", Area: "finance"}, false},
		{"another company's tool", domain.Scope{Company: "globex"}, false},
		// Nobody claimed the id: its origin is unknown, so it reaches nothing.
		{"a tool of unknown origin", domain.Scope{}, false},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()
			r := request()
			r.ToolScope = one.tool

			d := evaluate(t, New(), r)

			if one.allowed && d.Verdict != domain.VerdictAllow {
				t.Errorf("verdict = %v (%s: %s), want allow", d.Verdict, d.Rule, d.Reason)
			}
			if !one.allowed && (d.Verdict != domain.VerdictBlock || d.Rule != RuleScope) {
				t.Errorf("verdict = %v by %q, want block by %q", d.Verdict, d.Rule, RuleScope)
			}
		})
	}
}
