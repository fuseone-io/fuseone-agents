package domain

import (
	"slices"
	"time"
)

/*
Who holds what, and what that lets them attempt.

Split from the table beside it because they answer different questions and grow
for different reasons: access.go is the authorisation model — the roles, the
permissions and the one table binding them — and this is the person carrying a
grant through a request. The table changes when the product gains a capability;
this changes when identity or delegation does.
*/

// Grant binds a principal to a role within a scope.
//
// A grant is always scoped. Installation-wide administration is represented by
// the installation scope, not by an unbounded role: role says what, scope says
// where, and the audit trail names both.
type Grant struct {
	Scope Scope
	Role  Role
}

// HeldGrant is a grant plus where it came from.
//
// The difference decides what a screen may offer. A grant an identity provider
// asserts is re-derived on every sign-in, so revoking it in the console would
// last until its holder signs in again — a button that undoes itself is worse
// than no button, and the group is the thing to change.
type HeldGrant struct {
	Grant
	Asserted bool
	By       string
}

// Person is somebody the installation knows about, as an operator sees them.
type Person struct {
	ID      string
	Kind    PrincipalKind
	Display string
	Email   string
	// Provider is the identity provider that vouched for them, empty for a
	// service account or the administrator who claimed the installation.
	Provider string
	// Username is the handle they sign in with, when they have one. Empty for
	// everybody who arrives through a provider, which is how a screen tells
	// the two apart without guessing from Provider.
	Username string
	Grants   []HeldGrant
	LastSeen time.Time
	Disabled bool
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
