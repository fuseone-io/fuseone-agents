package draft

import (
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// SetIdentity sets the name and the scope. Human only: scope decides who may
// act, and the authorisation of both scopes is judged where it is applied.
type SetIdentity struct {
	Name  string
	Scope domain.Scope
}

func (SetIdentity) Kind() string     { return "set_identity" }
func (SetIdentity) Class() Class     { return Human }
func (SetIdentity) Fields() []string { return []string{"name", "company", "area"} }
func (o SetIdentity) apply(d *Draft) error {
	d.Spec.Name, d.Spec.Company, d.Spec.Area = o.Name, o.Scope.Company, o.Scope.Area
	return nil
}

// SetStandingApprovals decides whether authority already granted may release
// a call of this agent. Human only: it grants nothing, but it decides that.
type SetStandingApprovals struct{ Policy domain.StandingPolicy }

func (SetStandingApprovals) Kind() string     { return "set_standing_approvals" }
func (SetStandingApprovals) Class() Class     { return Human }
func (SetStandingApprovals) Fields() []string { return []string{"standingApprovals"} }
func (o SetStandingApprovals) apply(d *Draft) error {
	d.Spec.StandingApprovals = o.Policy
	return nil
}

// SetModel sets the provider, the model and the effort together.
type SetModel struct{ Provider, Model, Effort string }

func (SetModel) Kind() string     { return "set_model" }
func (SetModel) Class() Class     { return Shared }
func (SetModel) Fields() []string { return []string{"provider", "model", "effort"} }
func (o SetModel) apply(d *Draft) error {
	d.Spec.Provider, d.Spec.Model, d.Spec.Effort = o.Provider, o.Model, o.Effort
	return nil
}

// SetFallback sets the pair a run switches to; zero declares none.
type SetFallback struct{ Pair domain.ModelPair }

func (SetFallback) Kind() string     { return "set_fallback" }
func (SetFallback) Class() Class     { return Shared }
func (SetFallback) Fields() []string { return []string{"fallback"} }
func (o SetFallback) apply(d *Draft) error {
	d.Spec.Fallback = o.Pair
	return nil
}

// SetCachePrefixTTL sets how long the provider may keep the prefix cached.
type SetCachePrefixTTL struct{ TTL string }

func (SetCachePrefixTTL) Kind() string     { return "set_cache_prefix_ttl" }
func (SetCachePrefixTTL) Class() Class     { return Shared }
func (SetCachePrefixTTL) Fields() []string { return []string{"cachePrefixTtl"} }
func (o SetCachePrefixTTL) apply(d *Draft) error {
	d.Spec.CachePrefixTTL = o.TTL
	return nil
}

// SetReplyLocale pins the language the model writes in.
type SetReplyLocale struct{ Locale string }

func (SetReplyLocale) Kind() string     { return "set_reply_locale" }
func (SetReplyLocale) Class() Class     { return Shared }
func (SetReplyLocale) Fields() []string { return []string{"replyLocale"} }
func (o SetReplyLocale) apply(d *Draft) error {
	d.Spec.ReplyLocale = o.Locale
	return nil
}

// AddTool adds a tool to the pack.
type AddTool struct{ Tool domain.ToolID }

func (AddTool) Kind() string     { return "add_tool" }
func (AddTool) Class() Class     { return Shared }
func (AddTool) Fields() []string { return []string{"tools"} }
func (o AddTool) apply(d *Draft) error {
	if o.Tool == "" || slices.Contains(d.Spec.Tools, o.Tool) {
		return invalidOp(o.Kind(), "%q is empty or already in the pack", o.Tool)
	}
	d.Spec.Tools = append(d.Spec.Tools, o.Tool)
	return nil
}

// RemoveTool takes a tool out of the pack.
type RemoveTool struct{ Tool domain.ToolID }

func (RemoveTool) Kind() string     { return "remove_tool" }
func (RemoveTool) Class() Class     { return Shared }
func (RemoveTool) Fields() []string { return []string{"tools"} }
func (o RemoveTool) apply(d *Draft) error {
	i := slices.Index(d.Spec.Tools, o.Tool)
	if i < 0 {
		return invalidOp(o.Kind(), "%q is not in the pack", o.Tool)
	}
	d.Spec.Tools = slices.Delete(d.Spec.Tools, i, i+1)
	return nil
}

// SetBudget sets the ceilings.
type SetBudget struct{ Budget domain.Budget }

func (SetBudget) Kind() string     { return "set_budget" }
func (SetBudget) Class() Class     { return Shared }
func (SetBudget) Fields() []string { return []string{"budget"} }
func (o SetBudget) apply(d *Draft) error {
	d.Spec.Budget = o.Budget
	return nil
}

// SetTriggers sets what starts a run, whole.
type SetTriggers struct{ Triggers []spec.Trigger }

func (SetTriggers) Kind() string     { return "set_triggers" }
func (SetTriggers) Class() Class     { return Shared }
func (SetTriggers) Fields() []string { return []string{"triggers"} }
func (o SetTriggers) apply(d *Draft) error {
	d.Spec.Triggers = slices.Clone(o.Triggers)
	return nil
}

// SetMemoryLearning sets whether and how the agent learns from its runs.
type SetMemoryLearning struct{ Policy domain.MemoryLearningPolicy }

func (SetMemoryLearning) Kind() string     { return "set_memory_learning" }
func (SetMemoryLearning) Class() Class     { return Shared }
func (SetMemoryLearning) Fields() []string { return []string{"memoryLearning"} }
func (o SetMemoryLearning) apply(d *Draft) error {
	d.Spec.MemoryLearning = o.Policy.Normalize()
	return nil
}

// SetApprovalDelivery sets where an approval is delivered. It decides nothing
// about whether a person must answer: that is the installation's policies.
type SetApprovalDelivery struct{ Policy domain.ApprovalPolicy }

func (SetApprovalDelivery) Kind() string     { return "set_approval_delivery" }
func (SetApprovalDelivery) Class() Class     { return Shared }
func (SetApprovalDelivery) Fields() []string { return []string{"approvals"} }
func (o SetApprovalDelivery) apply(d *Draft) error {
	d.Spec.Approvals = o.Policy.Normalize()
	return nil
}

// SetEmits sets the events a finished run publishes, whole.
type SetEmits struct{ Emits spec.Emits }

func (SetEmits) Kind() string     { return "set_emits" }
func (SetEmits) Class() Class     { return Shared }
func (SetEmits) Fields() []string { return []string{"emits"} }
func (o SetEmits) apply(d *Draft) error {
	d.Spec.Emits = slices.Clone(o.Emits)
	return nil
}
