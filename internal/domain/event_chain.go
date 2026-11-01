package domain

/*
EventChain is where an event-opened run stands in the chain of runs that
events opened one after another (#203).

Without it a cycle across two agents — A publishes what B listens to, B
publishes what A listens to — runs until a budget, a pause or a person stops
it: every hop gets a fresh idempotency key from its new source run, so nothing
recognises the second lap as a repeat of the first.

Sealed on run_started, so the count travels in the ledger with the run rather
than in any worker's memory: a dispatcher on another replica, or one that
restarted, reads the same hop from the same step, and a redelivered event
reaches the run its key already opened without counting again.
*/
type EventChain struct {
	// Root is the run no event opened, where the chain began.
	Root RunID `json:"root"`
	// Hop is how many events separate this run from the root: 1 for a run
	// the root's event opened.
	Hop int `json:"hop"`
}

/*
MaxEventHops is how far a chain may reach from its root.

Eight is generous for composition anyone designs on purpose — a pipeline of
a handful of agents, or a review loop that goes round a few times — and small
enough that a cycle nobody intended stops after a few runs rather than after
a day's budget. The factory's Developer ⇄ Reviewer exchange gets four rounds.
*/
const MaxEventHops = 8

// ReasonEventHopLimit is why a run's events were not published: its chain is
// already MaxEventHops long.
const ReasonEventHopLimit = "event_hop_limit"

// Next is the chain a run opened by this run's event stands in, and whether
// it may be opened at all. A nil chain is a run no event opened: it is the
// root.
func (c *EventChain) Next(source RunID) (EventChain, bool) {
	if c == nil {
		return EventChain{Root: source, Hop: 1}, true
	}
	return EventChain{Root: c.Root, Hop: c.Hop + 1}, !c.Exhausted()
}

// Exhausted reports whether a run at this point may publish nothing further.
func (c *EventChain) Exhausted() bool {
	return c != nil && c.Hop >= MaxEventHops
}

// Withheld is what a run at this point records about the events it will not
// publish, or nil when it may publish them.
func (c *EventChain) Withheld() *EventsWithheld {
	if !c.Exhausted() {
		return nil
	}
	return &EventsWithheld{
		Reason: ReasonEventHopLimit, Root: c.Root, Hop: c.Hop, Limit: MaxEventHops,
	}
}

// EventsWithheld is sealed on run_finished when the run's declared events
// will not be published, so the trail says why the next agent never started
// rather than leaving a gap somebody has to explain.
type EventsWithheld struct {
	Reason string `json:"reason"`
	Root   RunID  `json:"root"`
	Hop    int    `json:"hop"`
	Limit  int    `json:"limit"`
}
