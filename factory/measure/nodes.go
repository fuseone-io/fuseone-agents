package main

import (
	"strings"
	"time"
)

type rawActor struct {
	Typename string `json:"__typename"`
	Login    string `json:"login"`
}

// actor is the actor GitHub named; a deleted account comes back as null and
// is the empty Actor.
func (a *rawActor) actor() Actor {
	if a == nil {
		return Actor{}
	}
	return Actor{Type: a.Typename, Login: a.Login}
}

// rawNode is every timeline item type the queries ask for, flattened: each
// type fills the fields it has.
type rawNode struct {
	Typename       string    `json:"__typename"`
	CreatedAt      time.Time `json:"createdAt"`
	SubmittedAt    time.Time `json:"submittedAt"`
	PreviousStatus string    `json:"previousStatus"`
	Status         string    `json:"status"`
	Project        *struct {
		Number int `json:"number"`
	} `json:"project"`
	Actor  *rawActor `json:"actor"`
	Author *rawActor `json:"author"`
	Body   string    `json:"body"`
	Label  *struct {
		Name string `json:"name"`
	} `json:"label"`
	State  string     `json:"state"`
	Commit *rawCommit `json:"commit"`
}

type rawCommit struct {
	Oid           string    `json:"oid"`
	CommittedDate time.Time `json:"committedDate"`
	Author        *struct {
		Email string `json:"email"`
		User  *struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"author"`
}

func (n rawNode) event() (Event, bool) {
	e := Event{At: n.CreatedAt, Actor: n.Actor.actor()}
	switch n.Typename {
	case "ProjectV2ItemStatusChangedEvent":
		e.Kind, e.From, e.To = KindStatus, n.PreviousStatus, n.Status
		if n.Project != nil {
			e.Project = n.Project.Number
		}
	case "IssueComment":
		e.Kind, e.Actor, e.ReturnTo = KindComment, n.Author.actor(), returnTo(n.Body)
	case "ReopenedEvent", "ClosedEvent", "MergedEvent":
		e.Kind = map[string]Kind{"ReopenedEvent": KindReopened, "ClosedEvent": KindClosed, "MergedEvent": KindMerged}[n.Typename]
	case "LabeledEvent", "UnlabeledEvent":
		e.Kind = map[bool]Kind{true: KindLabeled, false: KindUnlabeled}[n.Typename == "LabeledEvent"]
		if n.Label != nil {
			e.Label = n.Label.Name
		}
	case "PullRequestReview":
		e = Event{Kind: KindReview, At: n.SubmittedAt, Actor: n.Author.actor(), State: n.State, SHA: n.Commit.oid()}
	case "PullRequestCommit":
		return n.Commit.event()
	default:
		return Event{}, false
	}
	return e, true
}

func (c *rawCommit) oid() string {
	if c == nil {
		return ""
	}
	return c.Oid
}

// event is a commit as an act. Its time is the commit date, not the push,
// which the timeline does not carry. Its actor is the author's account: a
// User when GitHub links one; a Bot when the address is an App bot's
// noreply address (<id>+<slug>[bot]@users.noreply.github.com); otherwise
// nobody, and so unmapped.
func (c *rawCommit) event() (Event, bool) {
	if c == nil {
		return Event{}, false
	}
	e := Event{Kind: KindCommit, At: c.CommittedDate, SHA: c.Oid}
	switch a := c.Author; {
	case a == nil:
	case a.User != nil:
		e.Actor = Actor{Type: "User", Login: a.User.Login}
	default:
		e.Actor = botFromNoreply(a.Email)
	}
	return e, true
}

func botFromNoreply(email string) Actor {
	local, ok := strings.CutSuffix(email, "@users.noreply.github.com")
	_, login, plus := strings.Cut(local, "+")
	if ok && plus && strings.HasSuffix(login, "[bot]") {
		return Actor{Type: "Bot", Login: login}
	}
	return Actor{}
}

// returnTo is the lane a comment's last non-empty line names, when it reads
// "Return to: <lane>" (factory/LANES.md § Any lane → Travado); otherwise "".
// Nothing else of the body is kept.
func returnTo(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	lane, ok := strings.CutPrefix(last, "Return to:")
	if !ok {
		return ""
	}
	return strings.TrimSpace(lane)
}
