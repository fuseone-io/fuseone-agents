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

It is a view "of this reference" and nothing wider. Until the platform links
an issue to the pull request that answers it, each is its own reference with
its own trail, and the screen says so.

What it carries is counts and identifiers only. The approvals a run asked for
hold tool arguments, and the steps hold whatever the case holds; neither is
anything a reader of a card's history needs, and both may carry personal data
the trail would otherwise spread to everyone who can see the card.
*/

// WorkItemLink is how a run came to be on a card's trail.
type WorkItemLink string

// WorkItemLinkClaim is a run that claimed the card through
// $fuseone.work_item.claim. It is the only link slice 1 knows.
const WorkItemLinkClaim WorkItemLink = "claim"

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

// WorkItemRunFacts is what a store reads about one visible run so the roles
// can be folded from it. CostReadable is whether the caller may read this
// run's cost; the store decides it from the caller's cost scopes.
type WorkItemRunFacts struct {
	Run          WorkItemRun
	CostMicros   int64
	CostReadable bool
	Approvals    WorkItemApprovals
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
	Ref   WorkItemRef
	Runs  []WorkItemRun
	Roles []WorkItemRole
	// Next is where the following page starts; nil on the last page.
	Next *WorkItemCursor
}

// WorkItemRoles folds the facts of a card's visible runs into one row per
// agent, in the order each agent first claimed it.
//
// A role's cost is all of its runs or absent. Summing only the runs whose
// cost the caller may read would print a smaller number that looks complete.
func WorkItemRoles(facts []WorkItemRunFacts) []WorkItemRole {
	byAgent := map[AgentID]*WorkItemRole{}
	costly := map[AgentID]bool{}
	var order []AgentID
	for _, f := range facts {
		role, ok := byAgent[f.Run.AgentID]
		if !ok {
			role = &WorkItemRole{AgentID: f.Run.AgentID, FirstClaimedAt: f.Run.ClaimedAt, CostMicros: new(int64)}
			byAgent[f.Run.AgentID], costly[f.Run.AgentID] = role, true
			order = append(order, f.Run.AgentID)
		}
		role.Runs++
		role.Approvals = role.Approvals.plus(f.Approvals)
		if f.Run.ClaimedAt.Before(role.FirstClaimedAt) {
			role.FirstClaimedAt = f.Run.ClaimedAt
		}
		if f.Run.EndedAt.After(role.LastEndedAt) {
			role.LastEndedAt = f.Run.EndedAt
		}
		costly[f.Run.AgentID] = costly[f.Run.AgentID] && f.CostReadable
		*role.CostMicros += f.CostMicros
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
