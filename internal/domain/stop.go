package domain

import (
	"fmt"
	"time"
)

/*
Stopping things without a deploy (PRD FO-06).

Three levels, from the narrow to the total: one agent, everything in a scope,
and the whole installation. An incident does not arrive scoped to the agent
somebody happens to be looking at, and the person reaching for this is usually
not the person who wrote the specification.

The PRD names the middle level "per pack". This implementation has no named
packs — an author lists tools directly in the specification — so the middle
level is the scope, which is the grouping the rest of the platform is built
on and the one an operator actually says out loud: stop everything in
billing. Recorded in docs/NT-002 rather than left as a silent substitution.
*/

// StopLevel is how wide a stop reaches.
type StopLevel string

const (
	StopInstallation StopLevel = "installation"
	StopScope        StopLevel = "scope"
	StopAgent        StopLevel = "agent"
)

func (l StopLevel) Valid() bool {
	switch l {
	case StopInstallation, StopScope, StopAgent:
		return true
	}
	return false
}

// Stop is one switch, off.
//
// It carries a reason because somebody else will find it. A platform that
// stopped for no stated reason is one where the first question in the incident
// call is "did we do this on purpose?".
type Stop struct {
	Level  StopLevel
	Scope  Scope
	Agent  AgentID
	Reason string
	By     UserID
	At     time.Time
}

// Key names the switch, so the same target cannot be stopped twice into two
// rows that disagree.
func (s Stop) Key() string {
	switch s.Level {
	case StopInstallation:
		return "installation"
	case StopScope:
		return fmt.Sprintf("scope:%s/%s", s.Scope.Company, s.Scope.Area)
	default:
		return fmt.Sprintf("agent:%s", s.Agent)
	}
}

// Covers reports whether this stop reaches a run of this agent in this scope.
//
// A scope stop reaches the scopes inside it: stopping a company stops its
// areas, which is the whole point of the hierarchy and the reading somebody
// pressing it at 3am will assume.
func (s Stop) Covers(scope Scope, agent AgentID) bool {
	switch s.Level {
	case StopInstallation:
		return true
	case StopScope:
		return s.Scope.Contains(scope)
	case StopAgent:
		return s.Agent == agent
	}
	return false
}
