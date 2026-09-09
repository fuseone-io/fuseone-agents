package channel

import (
	"context"
	"errors"
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
	ApproversIn(ctx context.Context, scope domain.Scope) ([]domain.UserID, error)
}

// Accounts answers where people can be reached on one connection. Absence is
// the ordinary answer: most of a workspace has never been bound.
type Accounts interface {
	AccountsOn(ctx context.Context, channelName string, who []domain.UserID) (map[domain.UserID]string, error)
}

/*
fanout is one sweep's worth of private addressing.

Both questions are asked once and remembered for the pass. Who may decide is a
property of the run's scope; where they are reachable is configuration that
changes by an administrative act. Re-reading either between recipients would let
the set of people one announcement reaches change halfway through reaching them.
*/
type fanout struct {
	approvers Approvers
	accounts  Accounts
	byScope   map[domain.Scope][]domain.UserID
	byChannel map[string]map[domain.UserID]string
}

func (r *Reporter) newFanout() *fanout {
	return &fanout{
		approvers: r.approvers, accounts: r.accounts,
		byScope:   map[domain.Scope][]domain.UserID{},
		byChannel: map[string]map[domain.UserID]string{},
	}
}

// wanted reports whether this announcement is one to send privately.
//
// The same condition that draws the buttons. A private message about a run that
// finished carries no decision, and one about a stop with no step names nothing
// the button could answer.
func (f *fanout) wanted(report Report, place Conversation) bool {
	return f != nil && f.approvers != nil && f.accounts != nil &&
		place.DirectApprovals && report.AwaitingDecision && report.AtSeq > 0
}

/*
recipients is the private conversations this announcement is owed.

Empty when nobody may decide, when nobody who may is bound on this connection,
or when there are more of them than one approval should ever reach. The cap is
all or nothing, and the second return says whether it was hit — a caller has to
record why nobody was told, or the silence is indistinguishable from there being
nobody to tell.
*/
func (f *fanout) recipients(
	ctx context.Context, report Report, place Conversation,
) (to []Conversation, capped bool, err error) {
	who, err := f.whoDecides(ctx, report.Scope)
	if err != nil {
		return nil, false, err
	}
	if len(who) == 0 {
		return nil, false, nil
	}

	where, err := f.whereReachable(ctx, place.Channel, who)
	if err != nil {
		return nil, false, err
	}
	for _, one := range who {
		account := where[one]
		// Nobody bound is skipped and not failed: no conversation was owed
		// anything, and it is the ordinary state of most of a workspace. An
		// empty account must never travel — it is the shape that means a run
		// was said everywhere, which the store refuses to record.
		if account == "" {
			continue
		}
		to = append(to, Conversation{
			Channel: place.Channel, ID: account, Label: account,
			Wants: place.Wants, DirectApprovals: true,
		})
	}
	// Counted in messages, not in candidates. Applied to the people who may
	// decide, twenty-one approvers with one Slack account between them would
	// send nothing — somebody nobody bound silencing the one person who could
	// have been reached, which is the opposite of what the cap is for.
	if len(to) > MaxDirectRecipients {
		return nil, true, nil
	}
	return to, false, nil
}

func (f *fanout) whoDecides(ctx context.Context, scope domain.Scope) ([]domain.UserID, error) {
	if who, asked := f.byScope[scope]; asked {
		return who, nil
	}
	who, err := f.approvers.ApproversIn(ctx, scope)
	if err != nil {
		return nil, WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: who may approve in %s: %w", scope, err))
	}
	f.byScope[scope] = who
	return who, nil
}

func (f *fanout) whereReachable(
	ctx context.Context, channelName string, who []domain.UserID,
) (map[domain.UserID]string, error) {
	known := f.byChannel[channelName]
	missing := make([]domain.UserID, 0, len(who))
	for _, one := range who {
		if _, read := known[one]; !read {
			missing = append(missing, one)
		}
	}
	if len(missing) == 0 {
		return known, nil
	}

	found, err := f.accounts.AccountsOn(ctx, channelName, missing)
	if err != nil {
		return nil, WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: where to reach people on %s: %w", channelName, err))
	}
	if known == nil {
		known = map[domain.UserID]string{}
		f.byChannel[channelName] = known
	}
	for _, one := range missing {
		// Remembered as looked-up whether or not they were found, so an
		// unbound person is not asked about again on the next report.
		known[one] = found[one]
	}
	return known, nil
}

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

	for _, person := range to {
		posted, err := r.post(ctx, report, person)
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

// refuse records why nobody was told privately, without holding the run open.
// Neither cause improves with another sweep: one is configuration nobody can
// read, the other is a scope with more approvers than an approval should reach.
func (r *Reporter) refuse(report Report, place Conversation, cause error) refusal {
	return refusal{recorded: r.failuresFor(report, place, cause)}
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
	ctx context.Context, pass *fanout, report Report, place Conversation,
) (sent int, refused refusal) {
	posted, err := r.post(ctx, report, place)
	if err != nil {
		return 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, place, err),
		}
	}
	if posted {
		sent++
	}

	privately, refusedPrivately := r.direct(ctx, pass, report, place)
	return sent + privately, refusedPrivately
}
