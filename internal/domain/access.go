package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Role is what a principal may do within a scope.
//
// Five roles, deliberately. The first four keep duties separate; Admin is the
// operational shortcut for the person who is responsible for the installation
// rather than for one duty inside it.
type Role string

const (
	// RoleAdmin administers the installation. It is still scoped: held at the
	// installation scope it reaches every company, held lower it reaches only
	// that lower scope.
	RoleAdmin Role = "admin"
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

var roles = []Role{RoleAdmin, RoleAuthor, RoleApprover, RoleCurator, RoleAuditor}

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
	PermBrandWrite    Permission = "brand:write"
	// PermPolicyRead is separate from writing because a policy constrains
	// people who must not be able to change it. An author needs to read the
	// rule that stopped their agent; letting them edit it would make the rule
	// theirs rather than the organisation's.
	PermPolicyRead    Permission = "policy:read"
	PermPolicyWrite   Permission = "policy:write"
	PermIdentityWrite Permission = "identity:write"
	PermScopeWrite    Permission = "scope:write"
	// PermDataErase is the authority to destroy content — setting how long an
	// installation keeps it, and erasing a subject's on request.
	//
	// Its own permission rather than folded into administration, because it is
	// the one operation here that cannot be undone by anybody. Every other
	// administrative change can be changed back; this one leaves a tombstone
	// and a digest, and the bytes are gone.
	PermDataErase Permission = "data:erase"
	// PermCompanyWrite creates and withdraws companies, and it is the one
	// permission that means nothing inside a company.
	//
	// Held in a company it would let that company's administrator mint another
	// and grant themselves in it, which is not a tightening anybody would
	// notice. So it is only ever checked against the scope above them all
	// (domain.Installation), and a grant anywhere else does not carry it
	// however senior the role.
	PermCompanyWrite Permission = "company:write"
)

// grants maps each role to what it may do.
//
// The table is the authorisation model in full — there is no inheritance and
// no wildcard, so reading one row tells you everything a role can do.
var grants = map[Role][]Permission{
	RoleAdmin: {
		PermRunRead, PermRunTrigger, PermRunCancel, PermApprovalAct,
		PermAgentRead, PermAgentPublish,
		PermCostRead,
		PermAuditRead, PermAuditExport,
		PermToolRead, PermToolClassify, PermPackWrite,
		PermProviderWrite, PermBudgetWrite, PermBrandWrite, PermPolicyRead, PermPolicyWrite,
		PermIdentityWrite, PermScopeWrite, PermDataErase, PermCompanyWrite,
	},
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
		PermProviderWrite, PermBudgetWrite, PermBrandWrite, PermPolicyRead, PermPolicyWrite,
		PermIdentityWrite, PermScopeWrite, PermDataErase,
		// The role says what; the scope says where. A curator of one company
		// holds this and can use it nowhere, because it is only ever asked
		// about the installation.
		PermCompanyWrite,
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

/*
RolesAllowing lists the roles a permission is reachable through.

The grants table read the other way round, and derived rather than restated:
"who may decide" is a question the platform has to answer to offer a person on a
screen, and answering it with a role name is a guess that was wrong the day the
administrator gained the approval act. A second list would drift from the table
silently, and the symptom would be somebody offered a decision the button then
refuses.

In the order roles are declared, so an answer that decides who is offered or
told does not reshuffle between two calls.
*/
func RolesAllowing(p Permission) []Role {
	out := make([]Role, 0, len(roles))
	for _, r := range roles {
		if r.Allows(p) {
			out = append(out, r)
		}
	}
	return out
}

// Permissions lists what a role may do, sorted for display.
func (r Role) Permissions() []Permission {
	out := slices.Clone(grants[r])
	slices.Sort(out)
	return out
}
