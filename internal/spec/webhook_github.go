package spec

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

// Webhooks is every webhook this version declares, as the door needs it: the
// path without its leading slash, where deliveries come from, and which of
// them start a run.
//
// Empty and never nil, for the same reason as CronSchedules: a nil slice
// reaches the prune clause as NULL and keeps a withdrawn path answering.
func Webhooks(s Spec) []domain.WebhookTrigger {
	out := []domain.WebhookTrigger{}
	for _, t := range s.Triggers {
		if t.Type != TriggerWebhook || t.Path == "" {
			continue
		}
		out = append(out, domain.WebhookTrigger{
			Path:   strings.TrimPrefix(t.Path, "/"),
			From:   t.From,
			Events: slices.Clone(t.Events),
		})
	}
	return out
}

// githubEvent is how GitHub names an event, optionally with an action:
// `pull_request`, `pull_request.labeled`, `projects_v2_item.edited`.
var githubEvent = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)?$`)

/*
sourceProblems reports a webhook source the door could not act on.

Each of these would publish and then misbehave in silence. A source nobody
serves is verified as an ordinary hook and refuses every real delivery; events
on a hook with no source are never read, so the author's filter is a comment; a
GitHub hook that names no event would start a run on every delivery GitHub
sends, which on a busy repository is a bill; and a name GitHub never sends is a
filter nothing passes.

`ping` is refused because it never starts a run: GitHub sends it once, when the
hook is created, and the door answers it without opening anything.
*/
func sourceProblems(t Trigger) []string {
	if t.Type != TriggerWebhook {
		if t.From != "" || len(t.Events) > 0 {
			return []string{fmt.Sprintf("a %s trigger has no source or events: only a webhook does", t.Type)}
		}
		return nil
	}
	switch t.From {
	case "":
		if len(t.Events) > 0 {
			return []string{"a webhook's events are read only when it says where its deliveries " +
				"come from: add `from: github`, or remove the events"}
		}
		return nil
	case domain.WebhookFromGitHub:
		return githubEventProblems(t.Events)
	default:
		return []string{fmt.Sprintf("webhook source %q is not one this platform serves: github", t.From)}
	}
}

func githubEventProblems(events []string) []string {
	if len(events) == 0 {
		return []string{"a GitHub webhook names the events that start a run: without them every " +
			"delivery GitHub sends would start one"}
	}
	var problems []string
	for _, e := range events {
		switch {
		case e == "ping":
			problems = append(problems, "ping never starts a run: GitHub sends it when the hook is "+
				"created, and it is answered without one")
		case !githubEvent.MatchString(e):
			problems = append(problems, fmt.Sprintf(
				"%q is not how GitHub names an event: write `event` or `event.action`, "+
					"for example pull_request.labeled", e))
		}
	}
	return problems
}

// WebhookPaths is every path this version declares, without its leading slash.
func WebhookPaths(s Spec) []string {
	out := []string{}
	for _, w := range Webhooks(s) {
		out = append(out, w.Path)
	}
	return out
}
