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

/*
Every decision names the built-in checks that made it (NT-019).

The policy hash names the authored set, and in production WithPolicies puts
that hash on the decision in place of the built-in one. So a change to the
built-in checks, such as checkScope, was invisible on the trail: two workers
ruling differently under one policy hash. The identity travels on its own.
*/
func TestEvaluate_namesTheBuiltinChecks_onEveryVerdictAndWhateverThePolicySet(t *testing.T) {
	t.Parallel()
	// Every path out of Evaluate builds its own Decision: the plain allow, a
	// check that objects, and the release of a granted approval.
	paths := map[string]func(*Request){
		"allowed":           func(*Request) {},
		"blocked":           func(r *Request) { r.Tool = "crm.erase" },
		"requires approval": func(r *Request) { r.Tool, r.Effect, r.Stage = "crm.note", domain.EffectWrite, domain.StageCopilot },
		"approval granted": func(r *Request) {
			r.Tool, r.Effect, r.Stage, r.ApprovalGranted = "crm.note", domain.EffectWrite, domain.StageCopilot, true
		},
		"blocked by scope": func(r *Request) { r.ToolScope = domain.Scope{Company: "acme", Area: "finance"} },
	}
	want := map[string]domain.Verdict{
		"allowed": domain.VerdictAllow, "blocked": domain.VerdictBlock,
		"requires approval": domain.VerdictRequireApproval, "approval granted": domain.VerdictAllow,
		"blocked by scope": domain.VerdictBlock,
	}
	for name, g := range map[string]*Gate{
		"built-in only":    New(),
		"with an authored": New().WithPolicies(Policies{Hash: "pol_authored"}),
	} {
		for path, shape := range paths {
			r := request()
			shape(&r)
			d := evaluate(t, g, r)
			if d.Verdict != want[path] {
				t.Fatalf("%s/%s: verdict = %v (%s), want %v: the path is not the one named", name, path, d.Verdict, d.Rule, want[path])
			}
			if d.Builtin != BuiltinVersion() || d.Builtin == "" {
				t.Errorf("%s/%s: builtin = %q, want %q", name, path, d.Builtin, BuiltinVersion())
			}
		}
	}
	if d := evaluate(t, New().WithPolicies(Policies{Hash: "pol_authored"}), request()); d.PolicyHash != "pol_authored" {
		t.Errorf("policy hash = %q, want the authored set's, which replay resolves", d.PolicyHash)
	}
}

// The built-in identity is pinned, so a change to the checks or their version
// is a change somebody makes on purpose, here, and not by accident.
func TestBuiltinVersion_isPinned(t *testing.T) {
	t.Parallel()
	const pinned = "sha256:aa4f8391ff793a99"
	if got := BuiltinVersion(); got != pinned {
		t.Errorf("BuiltinVersion = %q; update the pin with a policyVersion bump when the checks change", got)
	}
}
