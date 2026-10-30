package channel

import (
	"errors"
)

/*
How a failure to tell somebody privately is classified and recorded.

This is the half that decides whether a sweep tries again. A failure either
holds the run open for the next pass or is written down and left behind, and
the line between the two is drawn here rather than in the code that sends.
*/

/*
terminal are the refusals that mean the same thing on the next sweep.

Named one by one, and the list is the whole of it: anything not here keeps its
run open. An app never granted permission to open a direct message, a person
who cannot be messaged at all, a driver that cannot do it — none improves by
being asked again, and treating one as temporary would hold every parked run
unreported for a day while re-attempting every recipient every thirty seconds.

The inversion is the point. Written as "anything that is not a rate limit is
final", a reset connection, an answer nobody could read and an error with no
code at all were all permanent — and a message lost to a network blip was
recorded as delivered and never sent again. Being unable to name why something
failed is a reason to try, not a reason to stop.
*/
var terminal = map[string]bool{
	CodeMissingScope:            true,
	CodeConversationUnavailable: true,
	CodeUnsupportedCapability:   true,
	CodeTooManyRecipients:       true,
	// Both wait on somebody configuring something: a connection to send from,
	// or people who may actually decide. Another sweep in thirty seconds reads
	// the same answer.
	CodeNoConnectionChosen:    true,
	CodeNamedNobodyWhoDecides: true,
}

// degrades reports that another sweep would learn nothing new.
func degrades(err error) bool {
	var known *Error
	if !errors.As(err, &known) {
		return false
	}
	return terminal[known.Code]
}

// refusal is what a fan-out could not do: what is worth another sweep, and
// what an operator should be able to read afterwards.
//
// Two lists rather than one, because they answer different people. Blocking
// holds the run open for the next pass; recorded is the trail of conversations
// that were owed something and did not get it, which is what somebody counts
// when a channel goes quiet.
type refusal struct {
	blocking []error
	recorded []DeliveryFailure
}

// refuse records why nobody was told privately, without holding the run open.
// Neither cause improves with another sweep: one is configuration nobody can
// read, the other is a scope with more approvers than an approval should reach.
func (r *Reporter) refuse(report Report, place Conversation, cause error) refusal {
	return refusal{recorded: r.failuresFor(report, place, cause)}
}
