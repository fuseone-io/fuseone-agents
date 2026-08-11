package domain

// A policy is a row, not a program.
//
// The alternative — an expression language — expresses anything and hands the
// customer exactly the "think like a programmer" the product exists to avoid
// (PRD N5). A policy nobody in the business can read is a policy nobody in the
// business owns, and ownership is the point: every rule here has a name on it.
//
// The cost of that choice is real. Some rule somebody wants will not fit, and
// the answer is to widen this vocabulary deliberately rather than to add an
// escape hatch that quietly becomes the way everything is written.

// PolicyEffect is what a matching policy asks for.
type PolicyEffect string

const (
	PolicyAllow    PolicyEffect = "allow"
	PolicyEscalate PolicyEffect = "escalate"
	PolicyDeny     PolicyEffect = "deny"
)

// Verdict translates a policy's effect into the Gate's vocabulary.
func (e PolicyEffect) Verdict() Verdict {
	switch e {
	case PolicyDeny:
		return VerdictBlock
	case PolicyEscalate:
		return VerdictRequireApproval
	default:
		return VerdictAllow
	}
}

// PolicyMode is whether a policy's answer is obeyed.
//
// Monitor exists because turning on a rule that denies is a change nobody can
// undo for the runs it stopped. A monitored policy is evaluated and recorded
// and changes no verdict, so an operator can read what it would have done
// before it does it.
type PolicyMode string

const (
	PolicyMonitor PolicyMode = "monitor"
	PolicyEnforce PolicyMode = "enforce"
)

// PolicyReach is how far a policy applies.
type PolicyReach string

const (
	// ReachInstallation covers every agent, including ones published later.
	ReachInstallation PolicyReach = "installation"
	// ReachScopes covers named companies or areas.
	ReachScopes PolicyReach = "scopes"
	// ReachAgents covers named agents and nothing else.
	ReachAgents PolicyReach = "agents"
)

// Condition is one clause. Every clause of a policy must hold — there is no
// `or`, because a rule read as a sentence stops being readable at the first
// one, and two policies express the same thing with two names on them.
type Condition struct {
	// Field names what to read from the request: tool.id, tool.effect,
	// data.taint, agent.id, scope.area, or args.<path> into the arguments.
	Field string `json:"field"`
	// Op is one of the operators below.
	Op string `json:"op"`
	// Value is compared as a number when both sides parse as one, and as text
	// otherwise. A rule written as `args.rows > 100` must not compare "1000"
	// to "100" as strings and conclude it is smaller.
	Value string `json:"value"`
}

// The operators. Deliberately few: each one has to be renderable in the
// sentence an operator reads back, and "matches this regular expression" is
// not a sentence anybody outside engineering can check.
const (
	OpEquals      = "eq"
	OpNotEquals   = "ne"
	OpGreaterThan = "gt"
	OpLessThan    = "lt"
	OpContains    = "contains"
	OpIn          = "in"
)

// Policy is one authored rule.
type Policy struct {
	// Code identifies the policy for a lifetime — it appears in the trail, in
	// the message a denied person reads, and in support conversations. It is
	// set once and never changes, which is why editing renders it locked.
	Code  string
	Name  string
	Owner string
	// Reason is the sentence shown in the trail and to whoever is denied. It
	// is the difference between "blocked by POL-114" and knowing what to do.
	Reason string

	// Resource is a glob over the tool id: `crm.*`, `crm.reply`, `*`.
	Resource string
	// Effects narrows to what the call does to the world. Empty means any.
	Effects []Effect

	Reach PolicyReach
	// Scopes and Agents name the reach when it is not the whole installation.
	Scopes []Scope
	Agents []AgentID

	Conditions []Condition
	Effect     PolicyEffect
	Mode       PolicyMode
	// Enabled is the enforcement toggle. A disabled policy is not evaluated
	// at all, which is different from a monitored one.
	Enabled bool
}

// PolicyInput is what a policy is evaluated against.
//
// A projection of the Gate's request rather than the request itself: a policy
// must not be able to read the idempotency key or the budget internals, and a
// type that offered them would eventually have a rule written against one.
type PolicyInput struct {
	Tool   ToolID
	Effect Effect
	Agent  AgentID
	Scope  Scope
	Labels Labels
	// Args is the proposed arguments, read by `args.<path>` conditions.
	Args []byte
}

// Applies reports whether a policy covers this call at all — before any
// condition is read.
func (p Policy) Applies(in PolicyInput) bool {
	if !p.Enabled {
		return false
	}
	if !globMatches(p.Resource, string(in.Tool)) {
		return false
	}
	if len(p.Effects) > 0 && !contains(p.Effects, in.Effect) {
		return false
	}

	switch p.Reach {
	case ReachAgents:
		return contains(p.Agents, in.Agent)
	case ReachScopes:
		for _, scope := range p.Scopes {
			if scope.Contains(in.Scope) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// Matches reports whether every condition holds. A policy with no conditions
// matches everything it applies to, which is how "deny all writes to crm.*" is
// written.
func (p Policy) Matches(in PolicyInput) bool {
	if !p.Applies(in) {
		return false
	}
	for _, condition := range p.Conditions {
		if !condition.holds(in) {
			return false
		}
	}
	return true
}
