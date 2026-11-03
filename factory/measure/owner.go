package main

import "time"

// Owner is who a stretch of board time or a caught-later signal belongs to:
// one of the factory's roles, a person, or nobody the map can name.
type Owner string

const (
	OwnerTriage    Owner = "triage"
	OwnerArchitect Owner = "architect"
	OwnerDeveloper Owner = "developer"
	OwnerReviewer  Owner = "reviewer"
	OwnerQA        Owner = "qa"
	OwnerMerger    Owner = "merger"
	OwnerPerson    Owner = "person"
	// OwnerUnattributed is time or a signal no rule gives to anyone, such as
	// Doing with no sub-state label on its pull request.
	OwnerUnattributed Owner = "unattributed"
	// OwnerUnmapped is an actor that is neither a person nor a listed bot.
	OwnerUnmapped Owner = "unmapped"
)

// roles is the closed set the bot map must list, in the order the factory's
// flow meets them.
func roles() []Owner {
	return []Owner{OwnerTriage, OwnerArchitect, OwnerDeveloper, OwnerReviewer, OwnerQA, OwnerMerger}
}

// owners is every owner a report row can carry, in print order.
func owners() []Owner {
	return append(roles(), OwnerPerson, OwnerUnattributed)
}

func (o Owner) isRole() bool {
	for _, r := range roles() {
		if o == r {
			return true
		}
	}
	return false
}

// Actor is who GitHub says did something: its GraphQL __typename (User, Bot,
// Mannequin, …; empty for a deleted account) and its login.
type Actor struct{ Type, Login string }

// Kind is the kind of a timeline event the measures read.
type Kind int

const (
	KindStatus Kind = iota + 1
	KindComment
	KindReopened
	KindClosed
	KindMerged
	KindLabeled
	KindUnlabeled
	KindReview
	KindCommit
	// KindDismissed is a review's dismissal: State holds the state the review
	// had before, ReviewID the review it dismissed.
	KindDismissed
)

// Event is one timeline item, reduced to what a measure reads. A comment's
// body is never kept: only the lane its last line names, if it ends with
// "Return to: <lane>".
type Event struct {
	Kind     Kind
	At       time.Time
	Actor    Actor
	Project  int    // KindStatus: the project's number; 0 when it was not readable
	From, To string // KindStatus: previousStatus and status
	Label    string // KindLabeled, KindUnlabeled
	State    string // KindReview: APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
	SHA      string // KindReview, KindCommit
	ReviewID string // KindReview: its node id; KindDismissed: the review dismissed
	ReturnTo string // KindComment
}

// Status is one commit status, as REST lists it.
type Status struct {
	SHA, Context, State string
	At                  time.Time
	Creator             Actor
}

// Item is an issue or a pull request with everything read about it.
type Item struct {
	Number   int
	IsPR     bool
	OnBoard  bool
	Author   Actor     // pull requests
	Merged   bool      // pull requests
	Created  time.Time // pull requests: when it was opened; zero when not read
	Linked   []int     // issue: the pull requests that close it; pull request: the issues it closes
	Events   []Event
	Statuses []Status // pull requests: factory/qa statuses on the SHAs it carried
}

// Snapshot is everything one report reads. Issues and pull requests share
// one number space in a repository.
type Snapshot struct {
	Items map[int]*Item
}
