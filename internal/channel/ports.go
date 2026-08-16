package channel

import (
	"context"
	"errors"

	"github.com/fuseone/agents/internal/domain"
)

/*
What the consumer asks of the rest of the platform.

Declared here rather than beside the implementations, which is the rule
everywhere in this repository and earns it twice over on this path: the
consumer is reached by a stranger's message, and the smallest possible
statement of what it may touch is part of the argument that it is safe.

They are also what keeps its own suite free of a database, a registry and a
Slack. Every one of these has a fake in the tests and a real one in the worker,
and the fake is held to the same invariants — a consumer that passes against a
permissive stub is a consumer nobody has tested.
*/

type Scopes interface {
	ScopeOf(ctx context.Context, channel, conversation string) (domain.Scope, error)
}

// Published lists what an ask in a scope could start.
type Published interface {
	List(ctx context.Context, scope domain.Scope, allVersions bool) ([]domain.AgentSummary, error)
}

// Willing answers whether an agent declared that a conversation may start it.
type Willing interface {
	StartableFromConversation(ctx context.Context, agent domain.AgentID, version domain.VersionID) (bool, error)
}

// Subjects resolves a reference to what the platform itself put in the
// conversation.
type Subjects interface {
	AboutRun(ctx context.Context, channel, conversation, ref string) (domain.RunID, bool, error)
}

// Opens turns an intention into a run.
type Opens interface {
	Open(ctx context.Context, req Request) (Opened, error)
}

/*
ErrWontStart means the opener declined, rather than failed.

An agent that is paused, stopped by a switch or still a draft will not start
now and will not start on a retry either: telling the person and closing the
ask is the right answer. A database that was away is not that, and must not
become a refusal somebody reads as final.

Wrapped by the adapter in opener.go, which is the only place that knows both
this package's contract and the trigger package's sentinels.
*/
var ErrWontStart = errors.New("channel: the agent will not start")

// Request and Opened mirror the trigger package's shapes, declared here so this
// package does not depend on it — the dependency runs the other way everywhere
// else and one edge pointing back would be the cycle.
type Request struct {
	Agent   domain.AgentID
	IdemKey string
	Trigger string
	By      domain.UserID
	Input   []byte
	Labels  domain.Labels
	Origin  *domain.RunOrigin
}

// Opened is the run an ask became.
type Opened struct {
	RunID   domain.RunID
	Created bool
}

// Answers says something back where the ask was made.
type Answers interface {
	Reply(ctx context.Context, channel, conversation, thread, text string) error
}

// Consumer opens the runs that asks became.
