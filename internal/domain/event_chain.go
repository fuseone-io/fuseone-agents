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

A constant, not a setting. A configurable ceiling would have to be sealed in
the chain itself: read from configuration at each hop, a change in the middle
of a chain would judge its runs by two different limits.
*/
const MaxEventHops = 8

// TriggerEvent is the trigger recorded on a run an event opened.
const TriggerEvent = "event"

const (
	// ReasonEventHopLimit: the chain is already MaxEventHops long.
	ReasonEventHopLimit = "event_hop_limit"
	// ReasonEventChainUnknown: an event opened the run but sealed no chain,
	// so how far it stands from its root cannot be known.
	ReasonEventChainUnknown = "event_chain_unknown"
)

// ChainPosition is a run's place in a chain, judged from what its start
// sealed: what opened it, and the chain an event carried.
type ChainPosition struct {
	Trigger string
	Chain   *EventChain
}

/*
Next is the chain a run opened by this run's event stands in — or, when this
run may publish nothing further, why.

Only a run no event opened is a root. One an event opened without a chain
was opened by a dispatcher that did not count: an older binary still running
during a rolling update. Read as a root it would restart the count at every
such hop, so it fails closed instead.
*/
func (p ChainPosition) Next(source RunID) (EventChain, *EventsWithheld) {
	if p.Trigger != TriggerEvent {
		return EventChain{Root: source, Hop: 1}, nil
	}
	if p.Chain == nil {
		return EventChain{}, &EventsWithheld{Reason: ReasonEventChainUnknown, Limit: MaxEventHops}
	}
	if p.Chain.Hop >= MaxEventHops {
		return EventChain{}, &EventsWithheld{
			Reason: ReasonEventHopLimit, Root: p.Chain.Root, Hop: p.Chain.Hop, Limit: MaxEventHops,
		}
	}
	return EventChain{Root: p.Chain.Root, Hop: p.Chain.Hop + 1}, nil
}

// EventsWithheld is sealed on run_finished when the run's declared events
// will not be published, so the trail says why the next agent never started
// rather than leaving a gap somebody has to explain. Root and Hop are empty
// when the chain is unknown.
type EventsWithheld struct {
	Reason string `json:"reason"`
	Root   RunID  `json:"root,omitempty"`
	Hop    int    `json:"hop,omitempty"`
	Limit  int    `json:"limit"`
}
