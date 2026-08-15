package spec

import "strings"

/*
What a version's triggers amount to for the two tables that make them happen.

Here rather than beside either caller, because there are two: an installation
that keeps its agents in git publishes by committing a file, and one that uses
the console publishes by pressing a button. Both end at the same tables, and a
version read one way and the other way has to yield the same triggers — the
same copy in two packages is how they stop agreeing.
*/

// CronSchedules is every schedule this version declares.
//
// Empty and never nil. A nil slice reaches a prune clause as NULL, and
// `schedule <> all(NULL)` is NULL rather than true — so an agent that withdrew
// every schedule would quietly keep firing all of them.
func CronSchedules(s Spec) []string {
	out := []string{}
	for _, t := range s.Triggers {
		if t.Type == "cron" && t.Schedule != "" {
			out = append(out, t.Schedule)
		}
	}
	return out
}

// WebhookPaths is every path this version declares, without its leading slash.
func WebhookPaths(s Spec) []string {
	out := []string{}
	for _, t := range s.Triggers {
		if t.Type == "webhook" && t.Path != "" {
			out = append(out, strings.TrimPrefix(t.Path, "/"))
		}
	}
	return out
}
