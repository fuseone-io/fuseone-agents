package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Role is what a principal may do within a scope.
//
// Four roles, deliberately. Every extra role is a combination somebody has to
// reason about at three in the morning, and the PRD's personas map cleanly
// onto these four (PRD §4, DE-05).
type Role string

const (
	// RoleAuthor describes processes and corrects examples. Never touches a
	// guardrail — that separation is what makes open authoring safe.
	RoleAuthor Role = "author"
	// RoleApprover decides on suspended actions.
	RoleApprover Role = "approver"
	// RoleCurator defines capability packs, classifies tool effects, sets
	// ceilings. The only role that can widen what agents may do.
	RoleCurator Role = "curator"
	// RoleAuditor reads everything and changes nothing.
	RoleAuditor Role = "auditor"
)

var roles = []Role{RoleAuthor, RoleApprover, RoleCurator, RoleAuditor}

func (r Role) Valid() bool { return slices.Contains(roles, r) }

func Roles() []Role { return slices.Clone(roles) }

func ParseRole(s string) (Role, error) {
	r := Role(strings.ToLower(strings.TrimSpace(s)))
	if !r.Valid() {
		return "", fmt.Errorf("unknown role %q", s)
	}
	return r, nil
}

// Permission is one thing a caller may attempt.
//
// Permissions name actions, not screens. A screen is a UI decision that
// changes; "may approve an action" is a property of the product that does not.
type Permission string

const (
	PermRunRead     Permission = "run:read"
	PermRunTrigger  Permission = "run:trigger"
	PermRunCancel   Permission = "run:cancel"
	PermApprovalAct Permission = "approval:act"

	PermAgentRead    Permission = "agent:read"
	PermAgentPublish Permission = "agent:publish"

	PermCostRead Permission = "cost:read"

	PermAuditRead   Permission = "audit:read"
	PermAuditExport Permission = "audit:export"

	// Administration. Everything that can widen what agents may do lives
	// behind the Curator, and nothing else grants it.
	PermToolRead      Permission = "tool:read"
	PermToolClassify  Permission = "tool:classify"
	PermPackWrite     Permission = "pack:write"
	PermProviderWrite Permission = "provider:write"
	PermBudgetWrite   Permission = "budget:write"
	// PermPolicyRead is separate from writing because a policy constrains
	// people who must not be able to change it. An author needs to read the
	// rule that stopped their agent; letting them edit it would make the rule
	// theirs rather than the organisation's.
	PermPolicyRead    Permission = "policy:read"
	PermPolicyWrite   Permission = "policy:write"
	PermIdentityWrite Permission = "identity:write"
	PermScopeWrite    Permission = "scope:write"
)

// grants maps each role to what it may do.
//
// The table is the authorisation model in full — there is no inheritance and
// no wildcard, so reading one row tells you everything a role can do.
var grants = map[Role][]Permission{
	RoleAuthor: {
		PermRunRead, PermRunTrigger, PermRunCancel,
		PermAgentRead, PermAgentPublish,
		// Reads the rules that constrain their agents, and changes none.
		PermPolicyRead,
		PermCostRead, PermToolRead,
	},
	RoleApprover: {
		// Reads policies because deciding an escalation means knowing which
		// rule raised it and what it was written to prevent.
		PermRunRead, PermApprovalAct, PermAgentRead, PermCostRead, PermPolicyRead,
	},
	RoleCurator: {
		PermRunRead, PermRunTrigger, PermRunCancel,
		PermAgentRead, PermAgentPublish,
		PermCostRead, PermAuditRead,
		PermToolRead, PermToolClassify, PermPackWrite,
		PermProviderWrite, PermBudgetWrite, PermPolicyRead, PermPolicyWrite,
		PermIdentityWrite, PermScopeWrite,
	},
	RoleAuditor: {
		// Reads everything within scope and changes nothing. An auditor who
		// can alter what they audit is not an auditor.
		PermRunRead, PermAgentRead, PermCostRead,
		PermAuditRead, PermAuditExport, PermToolRead, PermPolicyRead,
	},
}

// Allows reports whether the role carries the permission.
func (r Role) Allows(p Permission) bool {
	return slices.Contains(grants[r], p)
}

// Permissions lists what a role may do, sorted for display.
func (r Role) Permissions() []Permission {
	out := slices.Clone(grants[r])
	slices.Sort(out)
	return out
}

// Grant binds a principal to a role within a scope.
//
// A grant is always scoped. There is no installation-wide role: an operator
// who administers two companies holds two grants, and the audit trail names
// which one each action was taken under.
type Grant struct {
	Scope Scope
	Role  Role
}

// Principal is whoever is acting — a person, a service account, or an agent.
type Principal struct {
	ID      UserID
	Subject string
	Display string
	Kind    PrincipalKind
	Grants  []Grant

	// OnBehalfOf is set when an agent acts under a human's delegation. The
	// trail always records the pair, never the agent alone (PRD AU-05).
	OnBehalfOf UserID
}

type PrincipalKind string

const (
	PrincipalUser    PrincipalKind = "user"
	PrincipalService PrincipalKind = "service"
	PrincipalAgent   PrincipalKind = "agent"
)

// Can reports whether the principal may perform an action in a scope.
//
// Both the permission and the scope must match the same grant. Holding curator
// on one area does not grant it on another, which is what makes the company
// boundary hold once a group runs several of them (PRD §3.1).
func (p Principal) Can(perm Permission, scope Scope) bool {
	for _, g := range p.Grants {
		if g.Scope.Contains(scope) && g.Role.Allows(perm) {
			return true
		}
	}
	return false
}

// CanAnywhere reports whether the principal holds a permission in any scope.
// Use it to decide whether a listing is worth attempting; use Can for the
// resource itself.
func (p Principal) CanAnywhere(perm Permission) bool {
	for _, g := range p.Grants {
		if g.Role.Allows(perm) {
			return true
		}
	}
	return false
}

// ScopesFor lists the scopes in which the principal holds a permission. A
// listing endpoint filters by this rather than reading everything and
// discarding — the difference matters once a company has real volume.
func (p Principal) ScopesFor(perm Permission) []Scope {
	var out []Scope
	for _, g := range p.Grants {
		if g.Role.Allows(perm) && !slices.Contains(out, g.Scope) {
			out = append(out, g.Scope)
		}
	}
	return out
}

// Delegate returns the principal an agent acts as on behalf of a human.
//
// The effective grants are the intersection of the agent's capability envelope
// and the delegating human's: an agent never widens the reach of whoever
// triggered it (PRD AU-06).
func Delegate(human Principal, agent AgentID, envelope []Grant) Principal {
	var effective []Grant
	for _, e := range envelope {
		for _, h := range human.Grants {
			if e.Scope.Contains(h.Scope) && e.Role == h.Role {
				effective = append(effective, e)
				break
			}
		}
	}

	return Principal{
		ID:         UserID(agent),
		Subject:    string(agent),
		Display:    string(agent),
		Kind:       PrincipalAgent,
		Grants:     effective,
		OnBehalfOf: human.ID,
	}
}
