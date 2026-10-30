package channel

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Telling the people who may decide, as well as the room.

A card in a channel is addressed to whoever happens to be looking. Somebody who
could release the run in seconds has to notice it among everything else, and in
a busy conversation that is how a run sits parked for hours with several people
who were entitled to answer it.

**It addresses and it does not authorise.** The button carries the run and the
step, and pressing it takes the console's path with the console's checks against
the run's own scope. Being sent one and being allowed to answer it are separate
facts, and the second is never decided here — a recipient without the grant is
refused in a direct message exactly as they would be in a channel.

**And it never replaces the room.** The ledger records every decision, but the
ambient visibility that makes somebody notice a run has been waiting for two
hours is not in the ledger, it is in the channel. So the room is told first, and
a conversation that could not be reached is not quietly answered by a private
message instead.
*/

/*
MaxDirectRecipients bounds one approval's fan-out, and past it nobody is told
privately.

An approver grant held at the installation scope covers every run in it, so the
people who may decide can be a hundred. Messaging an arbitrary twenty of them is
worse than messaging none: nobody can tell whether they were meant to be asked,
and the eighty who were not have no way to know they were skipped. The room
still hears, which is what the room is for, and the reason is recorded.
*/
const MaxDirectRecipients = 20

// Approvers answers who may decide in a scope, declared here by the consumer.
//
// It addresses a message and grants nothing. The same person pressing the
// button is checked against the run's own scope by the console's own path,
// which is why being on this list and being refused are both ordinary.
type Approvers interface {
	// ApproversIn is who a run is announced to when nobody was named.
	// Deliberately narrower than the act: an administrator granted at the
	// installation may decide everywhere, and announcing by the act would tell
	// every administrator about every parked run there is.
	ApproversIn(ctx context.Context, scope domain.Scope) ([]domain.UserID, error)
	// DecidersIn is who may decide, which is what a name an owner chose is
	// checked against. Naming is one message to one person, not a broadcast,
	// so the question here is the one the button will ask.
	DecidersIn(ctx context.Context, scope domain.Scope) ([]domain.UserID, error)
}

// wanted reports whether this announcement is one to send privately.
//
// The same condition that draws the buttons. A private message about a run that
// finished carries no decision, and one about a stop with no step names nothing
// the button could answer.
func (f *fanout) wanted(report Report, place Conversation) bool {
	return f != nil && f.approvers != nil && f.accounts != nil &&
		!report.Ticket.Valid() && place.DirectApprovals &&
		report.AwaitingDecision && report.AtSeq > 0
}

/*
direct tells the people who may decide, privately, as well as the room.

Called after the room has heard and never instead of it. Each recipient is a
conversation of its own as far as the record is concerned, so the deduplication
that stops a room being told twice stops a person being messaged twice, and a
partial fan-out retried by the next sweep skips whoever already heard.
*/
func (r *Reporter) direct(
	ctx context.Context, pass *fanout, report Report, place Conversation,
) (sent int, refused refusal) {
	if !pass.wanted(report, place) {
		return 0, refusal{}
	}

	to, capped, err := pass.recipients(ctx, report, place)
	switch {
	case err != nil:
		// Not knowing who to tell, or where, is this side being unavailable.
		// Recorded and kept: nobody was told and nothing about that is
		// settled, so the run waits for a sweep that can read its own
		// configuration.
		return 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, place, err),
		}
	case capped:
		return 0, r.refuse(report, place, NewError(CodeTooManyRecipients,
			"channel: more people can be reached about this than one approval should reach"))
	}

	// One message for every recipient: a private card is the same card.
	msg := r.message(ctx, report)
	for _, person := range to {
		posted, err := r.post(ctx, report, person, msg)
		if err != nil {
			refused.recorded = append(refused.recorded, r.failuresFor(report, person, err)...)
			if !degrades(err) {
				refused.blocking = append(refused.blocking,
					fmt.Errorf("channel: tell %s privately: %w", person.ID, err))
			}
			continue
		}
		if posted {
			sent++
		}
	}
	return sent, refused
}

/*
placed posts and answers with where the message ended up.

A driver that cannot say is taken at its word that the address and the place
are the same, which is true of every room and of every driver that has no
private messages to send. Only the ones that do have to answer, and only their
messages can be closed afterwards.
*/
func placed(ctx context.Context, p Poster, c Conversation, m Message) (Placement, error) {
	if knows, ok := p.(Placer); ok {
		at, err := knows.PostPlaced(ctx, c, m)
		// The connection is the caller's knowledge, not the driver's: a driver
		// speaks for one and has never been told which.
		at.Channel = c.Channel
		return at, err
	}
	ref, err := p.Post(ctx, c, m)
	return Placement{Channel: c.Channel, Conversation: c.ID, Ref: ref}, err
}

/*
tell says it in one conversation, and privately to whoever may decide it.

The room comes first and the private cards never stand in for it: a
conversation that could not be reached is retried, and answering it with a
direct message meanwhile would put the run in front of one person and nobody
else.

Only a failure another sweep could survive holds the run open. Anything else is
recorded and left behind, or an installation that never granted the app
permission to open a direct message would keep every parked run unreported for
a day and re-send the channel card it already delivered.
*/
func (r *Reporter) tell(
	ctx context.Context, pass *fanout, report Report, place Conversation, msg Message,
) (sent int, refused refusal) {
	posted, err := r.post(ctx, report, place, msg)
	if err != nil {
		return 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, place, err),
		}
	}
	if posted {
		sent++
	}

	if pass.isTicketRoom(report, place) {
		privately, refusedPrivately := r.directForTicket(ctx, pass, report)
		return sent + privately, refusedPrivately
	}
	privately, refusedPrivately := r.direct(ctx, pass, report, place)
	return sent + privately, refusedPrivately
}
