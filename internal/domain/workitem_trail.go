package domain

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

/*
The trail of one card (#204): every run that worked on it, and per role what
those runs asked people and what they cost.

It is the view of one card: the issue and the pull requests linked to it
(#204, slice 2b). Resolution is one level deep — the root of a reference is
its alias's root, or the reference itself — so asked by a pull request, the
trail is its root's and says what it resolved from. A pull request nobody
linked is still its own reference with its own trail.

What it carries is counts and identifiers only. The approvals a run asked for
hold tool arguments, and the steps hold whatever the case holds; neither is
anything a reader of a card's history needs, and both may carry personal data
the trail would otherwise spread to everyone who can see the card.
*/

// WorkItemLink is how a run came to be on a card's trail.
type WorkItemLink string

const (
	// WorkItemLinkClaim is a run that claimed the card itself through
	// $fuseone.work_item.claim.
	WorkItemLinkClaim WorkItemLink = "claim"
	// WorkItemLinkAlias is a run that claimed a pull request linked to the
	// card through $fuseone.work_item.link.
	WorkItemLinkAlias WorkItemLink = "alias"
)

// WorkItemRun is one run on the trail: who, where, how far it got, and when
// it claimed the card. It is a summary, never the run's steps.
type WorkItemRun struct {
	RunID   RunID
	AgentID AgentID
	Scope   Scope
	Phase   string
	// StartedAt is when the run opened; EndedAt is zero while it is going.
	StartedAt time.Time
	EndedAt   time.Time
	ClaimedAt time.Time
	Link      WorkItemLink
	// Ref is what the run claimed, or inherited: the card, or the alias it
	// came by.
	Ref WorkItemRef
	// ClaimedRef is, on a run on the trail by its inheritance, the card it
	// claimed itself, when it claimed one: the run is on that card's trail
	// too (#204, slice 3).
	ClaimedRef WorkItemRef
	// InheritedRef is, on a run on the trail by its claim, the card it
	// inherited by how it was opened, when it inherited one.
	InheritedRef WorkItemRef
}

// WorkItemApprovals counts what a run, or a role's runs, asked of people:
// approvals requested, decided each way, and runs a person abandoned.
type WorkItemApprovals struct {
	Requested int
	Approved  int
	Refused   int
	Abandoned int
}

func (a WorkItemApprovals) plus(b WorkItemApprovals) WorkItemApprovals {
	return WorkItemApprovals{
		Requested: a.Requested + b.Requested,
		Approved:  a.Approved + b.Approved,
		Refused:   a.Refused + b.Refused,
		Abandoned: a.Abandoned + b.Abandoned,
	}
}

// WorkItemRoleShare is part of one role's runs on a trail, all in one scope:
// the unit a store aggregates, so the summary over a card with thousands of
// runs is a handful of rows rather than the runs themselves. CostReadable is
// whether the caller may read this scope's cost; the store decides it from
// the caller's cost scopes.
type WorkItemRoleShare struct {
	AgentID        AgentID
	Scope          Scope
	Runs           int
	Approvals      WorkItemApprovals
	FirstClaimedAt time.Time
	LastEndedAt    time.Time
	CostMicros     int64
	CostReadable   bool
}

// WorkItemRole is one agent's share of the trail. CostMicros is nil when the
// caller may not read the cost of every one of the role's runs.
type WorkItemRole struct {
	AgentID        AgentID
	Runs           int
	Approvals      WorkItemApprovals
	FirstClaimedAt time.Time
	// LastEndedAt is zero when none of the role's runs has ended.
	LastEndedAt time.Time
	CostMicros  *int64
}

// WorkItemTrail is one page of a card's trail. Roles cover every visible run
// of the card and come with the first page only; later pages carry runs.
type WorkItemTrail struct {
	// Ref is the card the trail is of: the reference asked for, or its root.
	Ref WorkItemRef
	// ResolvedFrom is the reference asked for when it was an alias the
	// caller may read, resolved to Ref; empty otherwise.
	ResolvedFrom WorkItemRef
	Runs         []WorkItemRun
	Roles        []WorkItemRole
	// Next is where the following page starts; nil on the last page.
	Next *WorkItemCursor
}

// WorkItemRoles folds the shares of a card's visible runs into one row per
// agent, in the order each agent first claimed it.
//
// A role's cost is all of its runs or absent. Summing only the runs whose
// cost the caller may read would print a smaller number that looks complete.
func WorkItemRoles(shares []WorkItemRoleShare) []WorkItemRole {
	byAgent := map[AgentID]*WorkItemRole{}
	costly := map[AgentID]bool{}
	var order []AgentID
	for _, s := range shares {
		role, ok := byAgent[s.AgentID]
		if !ok {
			role = &WorkItemRole{AgentID: s.AgentID, FirstClaimedAt: s.FirstClaimedAt, CostMicros: new(int64)}
			byAgent[s.AgentID], costly[s.AgentID] = role, true
			order = append(order, s.AgentID)
		}
		role.Runs += s.Runs
		role.Approvals = role.Approvals.plus(s.Approvals)
		if s.FirstClaimedAt.Before(role.FirstClaimedAt) {
			role.FirstClaimedAt = s.FirstClaimedAt
		}
		if s.LastEndedAt.After(role.LastEndedAt) {
			role.LastEndedAt = s.LastEndedAt
		}
		costly[s.AgentID] = costly[s.AgentID] && s.CostReadable
		*role.CostMicros += s.CostMicros
	}
	out := make([]WorkItemRole, 0, len(order))
	for _, agent := range order {
		role := *byAgent[agent]
		if !costly[agent] {
			role.CostMicros = nil
		}
		out = append(out, role)
	}
	slices.SortFunc(out, func(a, b WorkItemRole) int {
		return cmp.Or(a.FirstClaimedAt.Compare(b.FirstClaimedAt), strings.Compare(string(a.AgentID), string(b.AgentID)))
	})
	return out
}

// CompareWorkItemRuns is the trail's order: oldest claim first, a shared
// instant broken by run id.
func CompareWorkItemRuns(a, b WorkItemRun) int {
	return cmp.Or(a.ClaimedAt.Compare(b.ClaimedAt), strings.Compare(string(a.RunID), string(b.RunID)))
}
