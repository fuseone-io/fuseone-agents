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
	requireWorkItem(t, "issues", `{"action":"opened","repository":{"full_name":"FuseOne-IO/Agents","html_url":"https://github.com/FuseOne-IO/Agents"},
		"issue":{"number":204,"title":"see github:evil/repo#1","body":"github:evil/repo#2"}}`,
		"github:fuseone-io/agents#204")
}

func TestGitHubWorkItem_pr(t *testing.T) {
	t.Parallel()
	for _, event := range []string{
		"pull_request", "pull_request_review", "pull_request_review_comment", "pull_request_review_thread",
	} {
		requireWorkItem(t, event, `{"action":"opened","repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},
			"pull_request":{"number":240,"head":{"ref":"github:o/r#9"}}}`, "github:o/r#240")
	}
}

// A comment on a pull request is an issue_comment whose issue is the pull
// request: GitHub numbers both from one sequence.
func TestGitHubWorkItem_issueCommentOnPR(t *testing.T) {
	t.Parallel()
	requireWorkItem(t, "issue_comment", `{"action":"created","repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},
		"issue":{"number":240,"pull_request":{"url":"https://api.github.com/repos/o/r/pulls/240"}},
		"comment":{"body":"github:o/r#1"}}`, "github:o/r#240")
}

func TestGitHubWorkItem_unknownEvent(t *testing.T) {
	t.Parallel()
	body := `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{"number":1},"pull_request":{"number":2}}`
	for _, event := range []string{"push", "ping", "workflow_run", "check_run", "", "Issues"} {
		requireNoWorkItem(t, event, body)
	}
	// The event decides which object is read, never whichever is present.
	requireNoWorkItem(t, "issues", `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"pull_request":{"number":2}}`)
	requireNoWorkItem(t, "pull_request", `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{"number":1}}`)
}

func TestGitHubWorkItem_nonCanonical(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"no repository":     `{"issue":{"number":1}}`,
		"no number":         `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{}}`,
		"number as text":    `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{"number":"1"}}`,
		"fractional number": `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{"number":1.5}}`,
		"zero":              `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{"number":0}}`,
		"negative":          `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{"number":-3}}`,
		"padded name":       `{"repository":{"full_name":" o/r","html_url":"https://github.com/ o/r"},"issue":{"number":1}}`,
		"three segments":    `{"repository":{"full_name":"o/r/x","html_url":"https://github.com/o/r/x"},"issue":{"number":1}}`,
		"smuggled ref":      `{"repository":{"full_name":"o/r#7 github:x/y","html_url":"https://github.com/o/r#7 github:x/y"},"issue":{"number":1}}`,
		"no owner":          `{"repository":{"full_name":"/r","html_url":"https://github.com//r"},"issue":{"number":1}}`,
		"name as object":    `{"repository":{"full_name":{"x":1}},"issue":{"number":1}}`,
		"not JSON":          `not json`,
		"oversized number":  `{"repository":{"full_name":"o/r","html_url":"https://github.com/o/r"},"issue":{"number":123456789012}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requireNoWorkItem(t, "issues", body)
		})
	}
}

/*
The github: grammar names no host, so it means github.com. A delivery from
GitHub Enterprise names a repository on another host, and attributed by
full_name and number it would land on the github.com card of the same name.
Attribution happens only when the signed body's repository.html_url is
exactly https://github.com/<full_name>; anything else names no card.
*/
func TestGitHubWorkItem_fromGitHubDotCom_attributed(t *testing.T) {
	t.Parallel()
	requireWorkItem(t, "issues", `{"repository":{"full_name":"Acme/R","html_url":"https://github.com/Acme/R"},
		"issue":{"number":42}}`, "github:acme/r#42")
}

func TestGitHubWorkItem_notProvablyGitHubDotCom_namesNothing(t *testing.T) {
	t.Parallel()
	for name, url := range map[string]string{
		"enterprise":           `"html_url":"https://ghe.example/acme/r"`,
		"enterprise, github.*": `"html_url":"https://github.example.com/acme/r"`,
		"look-alike host":      `"html_url":"https://github.com.evil.example/acme/r"`,
		"userinfo":             `"html_url":"https://x@github.com/acme/r"`,
		"explicit port":        `"html_url":"https://github.com:443/acme/r"`,
		"plain http":           `"html_url":"http://github.com/acme/r"`,
		"another repository":   `"html_url":"https://github.com/acme/other"`,
		"a deeper path":        `"html_url":"https://github.com/acme/r/issues"`,
		"no html_url":          `"id":1`,
		"html_url not text":    `"html_url":{"host":"github.com"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requireNoWorkItem(t, "issues",
				`{"repository":{"full_name":"acme/r",`+url+`},"issue":{"number":42}}`)
		})
	}
}
