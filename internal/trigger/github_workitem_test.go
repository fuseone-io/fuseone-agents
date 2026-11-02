package trigger_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/trigger"
)

/*
The card a signed GitHub delivery is about (#204, slice 3), read from the
structural fields GitHub itself writes — repository.full_name and the issue's
or pull request's number — and from nothing a person typed into it.
*/

func requireWorkItem(t *testing.T, event, body string, want domain.WorkItemRef) {
	t.Helper()
	got, ok := trigger.GitHubWorkItem(event, []byte(body))
	if !ok || got != want {
		t.Fatalf("GitHubWorkItem(%s) = %q, %v; want %q", event, got, ok, want)
	}
}

func requireNoWorkItem(t *testing.T, event, body string) {
	t.Helper()
	if got, ok := trigger.GitHubWorkItem(event, []byte(body)); ok || got != "" {
		t.Fatalf("GitHubWorkItem(%s) = %q, %v; want none", event, got, ok)
	}
}

func TestGitHubWorkItem_issue(t *testing.T) {
	t.Parallel()
	// The title and body name another card; only the structure counts.
	requireWorkItem(t, "issues", `{"action":"opened","repository":{"full_name":"FuseOne-IO/Agents"},
		"issue":{"number":204,"title":"see github:evil/repo#1","body":"github:evil/repo#2"}}`,
		"github:fuseone-io/agents#204")
}

func TestGitHubWorkItem_pr(t *testing.T) {
	t.Parallel()
	for _, event := range []string{
		"pull_request", "pull_request_review", "pull_request_review_comment", "pull_request_review_thread",
	} {
		requireWorkItem(t, event, `{"action":"opened","repository":{"full_name":"o/r"},
			"pull_request":{"number":240,"head":{"ref":"github:o/r#9"}}}`, "github:o/r#240")
	}
}

// A comment on a pull request is an issue_comment whose issue is the pull
// request: GitHub numbers both from one sequence.
func TestGitHubWorkItem_issueCommentOnPR(t *testing.T) {
	t.Parallel()
	requireWorkItem(t, "issue_comment", `{"action":"created","repository":{"full_name":"o/r"},
		"issue":{"number":240,"pull_request":{"url":"https://api.github.com/repos/o/r/pulls/240"}},
		"comment":{"body":"github:o/r#1"}}`, "github:o/r#240")
}

func TestGitHubWorkItem_unknownEvent(t *testing.T) {
	t.Parallel()
	body := `{"repository":{"full_name":"o/r"},"issue":{"number":1},"pull_request":{"number":2}}`
	for _, event := range []string{"push", "ping", "workflow_run", "check_run", "", "Issues"} {
		requireNoWorkItem(t, event, body)
	}
	// The event decides which object is read, never whichever is present.
	requireNoWorkItem(t, "issues", `{"repository":{"full_name":"o/r"},"pull_request":{"number":2}}`)
	requireNoWorkItem(t, "pull_request", `{"repository":{"full_name":"o/r"},"issue":{"number":1}}`)
}

func TestGitHubWorkItem_nonCanonical(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"no repository":     `{"issue":{"number":1}}`,
		"no number":         `{"repository":{"full_name":"o/r"},"issue":{}}`,
		"number as text":    `{"repository":{"full_name":"o/r"},"issue":{"number":"1"}}`,
		"fractional number": `{"repository":{"full_name":"o/r"},"issue":{"number":1.5}}`,
		"zero":              `{"repository":{"full_name":"o/r"},"issue":{"number":0}}`,
		"negative":          `{"repository":{"full_name":"o/r"},"issue":{"number":-3}}`,
		"padded name":       `{"repository":{"full_name":" o/r"},"issue":{"number":1}}`,
		"three segments":    `{"repository":{"full_name":"o/r/x"},"issue":{"number":1}}`,
		"smuggled ref":      `{"repository":{"full_name":"o/r#7 github:x/y"},"issue":{"number":1}}`,
		"no owner":          `{"repository":{"full_name":"/r"},"issue":{"number":1}}`,
		"name as object":    `{"repository":{"full_name":{"x":1}},"issue":{"number":1}}`,
		"not JSON":          `not json`,
		"oversized number":  `{"repository":{"full_name":"o/r"},"issue":{"number":123456789012}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requireNoWorkItem(t, "issues", body)
		})
	}
}
