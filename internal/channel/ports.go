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

/*
Mapping is what an administrator configured a conversation to be.

One question rather than one per field, because they are one lookup and the
consumer needs them together. Read separately they can also disagree: a scope
from one version of the configuration and an agent from the next is a
combination nobody ever wrote down, and an edit is exactly when a channel is
busiest with people trying again.
*/
type Mapping interface {
	Resolve(ctx context.Context, channel, conversation string) (Mapped, error)
}

// Mapped is one conversation's configuration, read at one instant.
type Mapped struct {
	Scope domain.Scope
	// Agent is the agent this conversation starts, or empty when nobody chose
	// one. Empty is not a failure: a conversation may be open to whatever its
	// scope publishes, which was the only arrangement before a conversation
	// could name an agent for mentions.
	Agent domain.AgentID
	// Mode is what may start a run here, as ConversationMode reads it. It is a
	// boundary and not a label: a conversation that only watches configured
	// sources must not be startable by anybody who can type in it.
	Mode string
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

// ThreadContextPolicy answers whether a conversation chose to send surrounding
// thread text into the run input. It is separate from scope resolution because
// it is not authority: the conversation already speaks for a scope, and this
// only controls how much untrusted evidence accompanies the ask.
type ThreadContextPolicy interface {
	IncludeThreadContext(ctx context.Context, channel, conversation string) (bool, error)
}

// ThreadReader reads earlier messages from a vendor thread.
type ThreadReader interface {
	Thread(ctx context.Context, channel, conversation, thread, before string) (ThreadContext, error)
}

// ThreadContext is the bounded evidence read from a vendor thread.
type ThreadContext struct {
	Conversation string          `json:"conversation,omitempty"`
	Thread       string          `json:"thread,omitempty"`
	Messages     []ThreadMessage `json:"messages,omitempty"`
	Truncated    bool            `json:"truncated,omitempty"`
	Unavailable  string          `json:"unavailable,omitempty"`
}

// ThreadMessage is one message as evidence. Source is the vendor account that
// wrote it, not a platform principal and not authority.
type ThreadMessage struct {
	Ref    string `json:"ref"`
	Source string `json:"source,omitempty"`
	Text   string `json:"text"`
}

// Opens turns an intention into a run.
type Opens interface {
	Open(ctx context.Context, req Request) (Opened, error)
}

// Outcomes reads the closing answer a finished run recorded.
type Outcomes interface {
	FinishedOutcome(ctx context.Context, run domain.RunID) (domain.RunFinishedPayload, error)
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
	ReplyOutcome(ctx context.Context, channel, conversation, thread, text string) error
}

// Consumer opens the runs that asks became.
