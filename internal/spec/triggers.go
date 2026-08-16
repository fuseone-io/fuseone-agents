package spec

import (
	"context"
	"strings"

	"github.com/fuseone/agents/internal/domain"
)

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
		if t.Type == TriggerCron && t.Schedule != "" {
			out = append(out, t.Schedule)
		}
	}
	return out
}

// WebhookPaths is every path this version declares, without its leading slash.
func WebhookPaths(s Spec) []string {
	out := []string{}
	for _, t := range s.Triggers {
		if t.Type == TriggerWebhook && t.Path != "" {
			out = append(out, strings.TrimPrefix(t.Path, "/"))
		}
	}
	return out
}

/*
StartableFromConversation reports whether an ask in a conversation may start
this agent.

A bare declaration with no field, and deliberately so. The other three triggers
are self-contained — cron carries its expression, webhook its path, event its
name — and a channel cannot close that way, because a conversation carries a
scope and the conversation-to-scope map is administrative. An author writing
`conversation: C07-ops` would be choosing which conversation may start their
agent, and the author is precisely the person who does not govern which
conversations belong to which area (NT-005 §9).

The agent declares willingness; the administration declares reach. That is the
shape tools already have — the author asks for one, the Curator decides what it
may do — and it exists for the same reason: describing a process must not grant
any power.

It earns its place in the other direction too. An agent that does not declare
it cannot be started by any message at all, however the conversations are
mapped, and being able to say "this one is internal, never startable by text"
is a property worth being able to state.
*/
func StartableFromConversation(s Spec) bool {
	for _, t := range s.Triggers {
		if t.Type == TriggerChannel {
			return true
		}
	}
	return false
}

/*
StartableFromConversation, asked of a published version.

The same predicate over what the registry holds, which is what the channel
consumer needs: it has an agent and the version pinned to the ask, and no
business knowing how a spec is stored. A method here rather than an adapter
beside the consumer because the reading is this package's, and the consumer
declares only the shape it calls.

A version nobody published comes back as an error and never as "not willing".
The two are said to different people — unwilling is a sentence about somebody's
agent, a failed read is a sentence about us — and confusing them sends an
author to add a trigger to a spec that already has one.
*/
func (r *Registry) StartableFromConversation(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (bool, error) {
	s, err := r.Get(ctx, agent, version)
	if err != nil {
		return false, err
	}
	return StartableFromConversation(s), nil
}

// The trigger types this platform serves. A type outside this set is refused
// rather than ignored: every reader filters for what it knows, so an
// unrecognised one publishes, prints back on the screen as configured, and
// fires nothing — with no error state that describes it.
const (
	TriggerCron    = "cron"
	TriggerWebhook = "webhook"
	TriggerEvent   = "event"
	TriggerChannel = "channel"
)
