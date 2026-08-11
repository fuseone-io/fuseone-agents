package domain_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

// A policy is read by the person who owns it, and it decides whether an agent
// touches a real system. Both of those make the failure mode the same: a rule
// that does something other than what it says.

func policy(over func(*domain.Policy)) domain.Policy {
	p := domain.Policy{
		Code: "POL-114", Name: "Sem exportação em massa", Owner: "Governança",
		Resource: "*", Reach: domain.ReachInstallation,
		Effect: domain.PolicyDeny, Mode: domain.PolicyEnforce, Enabled: true,
	}
	if over != nil {
		over(&p)
	}
	return p
}

func call(over func(*domain.PolicyInput)) domain.PolicyInput {
	in := domain.PolicyInput{
		Tool: "crm.reply", Effect: domain.EffectWrite, Agent: "triage",
		Scope: domain.Scope{Company: "acme", Area: "cx"},
	}
	if over != nil {
		over(&in)
	}
	return in
}

func TestMatches_noConditions_coversEverythingItAppliesTo(t *testing.T) {
	t.Parallel()

	// How "deny every write to crm" is written. A rule needing a condition to
	// mean anything would make the simplest policy the hardest to author.
	p := policy(func(p *domain.Policy) {
		p.Resource = "crm.*"
		p.Effects = []domain.Effect{domain.EffectWrite}
	})

	if !p.Matches(call(nil)) {
		t.Error("a policy with no conditions did not match the call it covers")
	}
	if p.Matches(call(func(in *domain.PolicyInput) { in.Effect = domain.EffectRead })) {
		t.Error("it matched a read, which it does not cover")
	}
}

func TestMatches_disabledPolicy_isNotEvaluatedAtAll(t *testing.T) {
	t.Parallel()

	// Different from monitoring. A disabled policy produces no decision to
	// record; a monitored one produces a decision nothing obeys.
	p := policy(func(p *domain.Policy) { p.Enabled = false })

	if p.Matches(call(nil)) {
		t.Error("a disabled policy matched")
	}
}

func TestMatches_resourceGlob_coversAPrefixAndNotANeighbour(t *testing.T) {
	t.Parallel()

	p := policy(func(p *domain.Policy) { p.Resource = "crm.*" })

	if !p.Matches(call(nil)) {
		t.Error("crm.* did not cover crm.reply")
	}
	if p.Matches(call(func(in *domain.PolicyInput) { in.Tool = "kb.search" })) {
		t.Error("crm.* covered kb.search")
	}
}

func TestMatches_reachByAgent_coversOnlyTheNamedOnes(t *testing.T) {
	t.Parallel()

	p := policy(func(p *domain.Policy) {
		p.Reach = domain.ReachAgents
		p.Agents = []domain.AgentID{"billing"}
	})

	if p.Matches(call(nil)) {
		t.Error("a policy naming billing matched triage")
	}
	if !p.Matches(call(func(in *domain.PolicyInput) { in.Agent = "billing" })) {
		t.Error("it did not match the agent it names")
	}
}

func TestMatches_reachByScope_widensDownwardsAndNoFurther(t *testing.T) {
	t.Parallel()

	// Policy inherits downwards and never widens (PRD §3.1). A rule set on
	// the company covers its areas; one set on an area covers only itself.
	company := policy(func(p *domain.Policy) {
		p.Reach = domain.ReachScopes
		p.Scopes = []domain.Scope{{Company: "acme"}}
	})
	if !company.Matches(call(nil)) {
		t.Error("a company-wide policy did not reach an area inside it")
	}

	area := policy(func(p *domain.Policy) {
		p.Reach = domain.ReachScopes
		p.Scopes = []domain.Scope{{Company: "acme", Area: "marketing"}}
	})
	if area.Matches(call(nil)) {
		t.Error("a policy set on marketing reached cx")
	}
}

func TestMatches_everyConditionMustHold(t *testing.T) {
	t.Parallel()

	// There is no `or`. A rule stops being readable at the first one, and two
	// policies say the same thing with two names on them.
	p := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{
			{Field: "args.rows", Op: domain.OpGreaterThan, Value: "100"},
			{Field: "data.taint", Op: domain.OpIn, Value: "untrusted"},
		}
	})

	both := call(func(in *domain.PolicyInput) {
		in.Args = []byte(`{"rows":500}`)
		in.Labels = domain.Labels{"untrusted"}
	})
	if !p.Matches(both) {
		t.Error("both conditions held and the policy did not match")
	}

	one := call(func(in *domain.PolicyInput) { in.Args = []byte(`{"rows":500}`) })
	if p.Matches(one) {
		t.Error("one condition held and the policy matched anyway")
	}
}

func TestCondition_comparesNumbersAsNumbers(t *testing.T) {
	t.Parallel()

	// "1000" is not less than "100", however it sorts. A rule written to stop
	// a bulk export would let the biggest one through.
	p := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{{Field: "args.rows", Op: domain.OpGreaterThan, Value: "100"}}
	})

	if !p.Matches(call(func(in *domain.PolicyInput) { in.Args = []byte(`{"rows":1000}`) })) {
		t.Error("1000 was not read as greater than 100")
	}
	if p.Matches(call(func(in *domain.PolicyInput) { in.Args = []byte(`{"rows":50}`) })) {
		t.Error("50 was read as greater than 100")
	}
}

func TestCondition_aListCountsAsItsLength(t *testing.T) {
	t.Parallel()

	// `args.rows > 100` where rows is the rows themselves, which is how a
	// caller sending data rather than a count would write it.
	p := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{{Field: "args.rows", Op: domain.OpGreaterThan, Value: "2"}}
	})

	if !p.Matches(call(func(in *domain.PolicyInput) { in.Args = []byte(`{"rows":[1,2,3,4]}`) })) {
		t.Error("a list of four was not read as greater than two")
	}
}

func TestCondition_absentField_failsAnEqualityAndHoldsAnInequality(t *testing.T) {
	t.Parallel()

	// A field that is not there is not equal to anything. The alternative —
	// an absent field failing every comparison — makes "deny unless marked
	// reviewed" silently never fire, which is the dangerous direction.
	equals := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{{Field: "args.reviewed", Op: domain.OpEquals, Value: "true"}}
	})
	if equals.Matches(call(nil)) {
		t.Error("an absent field satisfied an equality")
	}

	differs := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{{Field: "args.reviewed", Op: domain.OpNotEquals, Value: "true"}}
	})
	if !differs.Matches(call(nil)) {
		t.Error("an absent field did not satisfy an inequality")
	}
}

func TestCondition_unknownOperator_doesNotHold(t *testing.T) {
	t.Parallel()

	// A rule nobody can evaluate must not pass. Passing would turn a typo in
	// a stored policy into an allow.
	p := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{{Field: "tool.id", Op: "roughly", Value: "crm.reply"}}
	})

	if p.Matches(call(nil)) {
		t.Error("a policy with an unimplemented operator matched")
	}
}

func TestCondition_taintHoldsWhenAnyLabelIsListed(t *testing.T) {
	t.Parallel()

	p := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{{Field: "data.taint", Op: domain.OpIn, Value: "untrusted,personal"}}
	})

	if !p.Matches(call(func(in *domain.PolicyInput) {
		in.Labels = domain.Labels{"personal"}
	})) {
		t.Error("a call carrying one of the listed labels did not match")
	}
	if p.Matches(call(func(in *domain.PolicyInput) { in.Labels = domain.Labels{"reviewed"} })) {
		t.Error("a call carrying none of them matched")
	}
}

func TestPolicyEffect_translatesToTheGatesVocabulary(t *testing.T) {
	t.Parallel()

	for effect, want := range map[domain.PolicyEffect]domain.Verdict{
		domain.PolicyDeny:     domain.VerdictBlock,
		domain.PolicyEscalate: domain.VerdictRequireApproval,
		domain.PolicyAllow:    domain.VerdictAllow,
	} {
		if got := effect.Verdict(); got != want {
			t.Errorf("%s became %v, want %v", effect, got, want)
		}
	}
}

// The sentence is generated from the fields the Gate reads, so the screen
// cannot describe a rule the engine does not run.

func TestSentence_readsBackTheRuleTheGateWillEvaluate(t *testing.T) {
	t.Parallel()

	p := policy(func(p *domain.Policy) {
		p.Resource = "customer.*"
		p.Effects = []domain.Effect{domain.EffectRead, domain.EffectWrite}
		p.Conditions = []domain.Condition{
			{Field: "args.rows", Op: domain.OpGreaterThan, Value: "100"},
			{Field: "data.taint", Op: domain.OpIn, Value: "untrusted"},
		}
	})

	want := "customer.* · read, write · args.rows > 100 · data.taint em untrusted → negar"
	if got := p.Sentence(); got != want {
		t.Errorf("sentence =\n  %s\nwant\n  %s", got, want)
	}
}

func TestSentence_saysWhenAPolicyIsOnlyWatching(t *testing.T) {
	t.Parallel()

	// A rule reading "→ negar" while denying nothing is the most misleading
	// thing this screen could show.
	p := policy(func(p *domain.Policy) { p.Mode = domain.PolicyMonitor })

	if got := p.Sentence(); !strings.Contains(got, "monitorando") {
		t.Errorf("sentence = %q, want it to say the rule is not enforcing", got)
	}
}

func TestSentence_anUnimplementedOperatorReadsAsItself(t *testing.T) {
	t.Parallel()

	// It does not hold either. The two have to agree, or an author reads a
	// sentence describing a rule that silently never fires.
	p := policy(func(p *domain.Policy) {
		p.Conditions = []domain.Condition{{Field: "tool.id", Op: "roughly", Value: "crm.reply"}}
	})

	if got := p.Sentence(); !strings.Contains(got, "roughly") {
		t.Errorf("sentence = %q, want the operator visible", got)
	}
}
