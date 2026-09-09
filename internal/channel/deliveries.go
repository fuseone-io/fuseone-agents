package channel

import (
	"context"
	"time"

	"github.com/fuseone/agents/internal/domain"
)

/*
What was said, where it went, and what became of it.

Three questions that travel together and are answered by the same table. A
delivery is the fact that a message left; a placement is where the vendor put
it, which is not always where it was sent; and a failure is a conversation that
was owed one and did not get it.

Split from channel.go because that file had become the whole vocabulary of the
package, and because these are the parts a second vendor would have to satisfy
without touching anything else.
*/

/*
Placement is where a message ended up, as the vendor names it.

Not the same as the address it was sent to. A direct message is addressed to a
person and lands in a conversation the vendor invents; the first is what makes
a delivery idempotent, because it is known before the message leaves, and the
second is what an edit needs.
*/
type Placement struct {
	// Channel is the connection the message belongs to. A conversation id
	// means nothing without it, and the token that may rewrite a message is
	// that connection's.
	Channel      string
	Conversation string
	Ref          string
}

// Placer is a driver that can say where it put a message. Optional: one that
// cannot is still a Poster, and its messages simply cannot be closed later.
type Placer interface {
	PostPlaced(ctx context.Context, c Conversation, m Message) (Placement, error)
}

// Editors replaces a message this platform posted, which is how a card stops
// offering an answer to a question that already has one.
type Editors interface {
	Edit(ctx context.Context, at Placement, m Message) error
}

// Delivery records that a message left.
type Delivery struct {
	Announcement
	// Ref is what the channel called the message, so a later stage can reply
	// in the same thread.
	Ref string
	// Placed is the vendor's own name for where this landed, when it differs
	// from the address it was sent to. Empty means they are the same.
	Placed   string
	PostedAt time.Time
}

// DeliveryFailure records that a conversation was owed a message and did not
// receive it. It is scoped because the cockpit that reads this later must not
// turn a channel incident in one area into installation-wide knowledge.
type DeliveryFailure struct {
	Announcement
	// ScopeWide means the failure happened before the reporter knew which
	// conversations were owed the message. Counting it as one conversation
	// would understate the blast radius as confidently as naming all of them.
	ScopeWide bool
	Code      string
	Scope     domain.Scope
	AgentID   domain.AgentID
	SeenAt    time.Time
}

// Deliveries is what has already been said.
type Deliveries interface {
	Record(ctx context.Context, d Delivery) error
	RecordFailure(ctx context.Context, f DeliveryFailure) error
	RecordFailures(ctx context.Context, failures []DeliveryFailure) error
	Delivered(ctx context.Context, a Announcement) (bool, error)
}
