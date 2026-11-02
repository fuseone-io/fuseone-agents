package trigger

import (
	"encoding/json"
	"strconv"

	"github.com/fuseone/agents/internal/domain"
)

/*
The card a GitHub delivery is about (#204, slice 3).

A run a signed delivery opened is attributed to the issue or pull request the
delivery names. Only the structure GitHub itself writes is read: the
repository's full_name, and the number of the issue or the pull request the
event is about. The body stays untrusted as a run's input; these fields are
read here because GitHub signed them and no person types them — a title, a
comment or a branch name that spells a reference is never looked at.

Which object holds the number is the event's, from a closed table: a body that
carries both an issue and a pull request is read by the one its event names,
and an event outside the table attributes nothing. issue_comment reads the
issue even when the comment is on a pull request, whose number is the same.
*/

// githubNumbered is, per event, which object of the body holds the number.
var githubNumbered = map[string]string{
	"issues":                      "issue",
	"issue_comment":               "issue",
	"pull_request":                "pull_request",
	"pull_request_review":         "pull_request",
	"pull_request_review_comment": "pull_request",
	"pull_request_review_thread":  "pull_request",
}

// GitHubWorkItem is the reference a delivery of event names, through the
// claim's own parser, or false when it names none: an event outside the
// table, a missing or malformed number, or a repository that does not make a
// canonical reference. False is never a refusal — the run opens without it.
func GitHubWorkItem(event string, body []byte) (domain.WorkItemRef, bool) {
	object, ok := githubNumbered[event]
	if !ok {
		return "", false
	}
	var envelope struct {
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Issue       *githubNumber `json:"issue"`
		PullRequest *githubNumber `json:"pull_request"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return "", false
	}
	numbered := envelope.Issue
	if object == "pull_request" {
		numbered = envelope.PullRequest
	}
	if numbered == nil || numbered.Number == nil {
		return "", false
	}
	ref := "github:" + envelope.Repository.FullName + "#" + strconv.FormatUint(*numbered.Number, 10)
	return domain.ParseWorkItemRef(ref)
}

// githubNumber is the number of an issue or a pull request. An unsigned
// integer, so text, fractions and negatives fail to decode rather than being
// coerced.
type githubNumber struct {
	Number *uint64 `json:"number"`
}
