package channel

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
The approval an agent's owner asked to be told about, with no room involved.

The private card was born inside the loop over conversations: a room was owed an
announcement, and the people who may decide were told as well. That is right
where a room exists, and it is the whole answer to nothing where one does not —
an agent published in an area nobody mapped produced no card, no message, and no
record that anybody should have been told.

This is the other beginning. The agent's own specification says its approvals
should also arrive privately, and that is enough: no conversation, no channel,
no mapping from a scope to a room. What it needs instead is the one thing the
room used to supply for free — which workspace the bot speaks from.

It sends the same card, to the same people, under the same cap, recorded under
the same key. A person who was already told through a room is not told twice:
the delivery is keyed by the run, the step and where the message went, and this
path posts to exactly the same place.
*/

// wantsOwnApprovals reports whether this announcement is one the agent asked to
// have sent privately on its own account.
//
// The same condition that draws the buttons, and then the agent's word. A
// message about a run that finished carries no decision, and one about a stop
// with no step names nothing a button could answer.
func (r *Reporter) wantsOwnApprovals(report Report) bool {
	return r.approvals != nil && r.connections != nil &&
		r.approvers != nil && r.accounts != nil &&
		report.AwaitingDecision && report.AtSeq > 0
}

/*
directForOwner tells the people the agent's owner asked for, and says whether
anything was owed at all.

The second answer is what keeps the run from waiting for ever. A run is retired
from the sweep once something was owed a message and nothing failed in a way
another sweep would fix — so an agent that asked for private approvals owes one
here, whether it ends in messages or in a recorded reason nobody can improve by
waiting.
*/
func (r *Reporter) directForOwner(
	ctx context.Context, pass *fanout, report Report,
) (sent, owed int, refused refusal) {
	if !r.wantsOwnApprovals(report) {
		return 0, 0, refusal{}
	}

	policy, err := r.approvals.ApprovalPolicy(ctx, report.AgentID, report.Version)
	if err != nil {
		// This side being unavailable. Nothing is owed and nothing is
		// recorded: the next sweep reads the specification again.
		return 0, 0, refusal{blocking: []error{WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: read the approval policy of %s: %w", report.AgentID, err))}}
	}
	if !policy.Direct {
		return 0, 0, refusal{}
	}

	from, err := r.speakingConnection(ctx)
	switch {
	case err != nil:
		return 0, 0, refusal{blocking: []error{err}}
	case from == "":
		// Owed, and answered with a reason. Whether it is one connection too
		// many or none at all, another sweep reads the same configuration.
		return 0, 1, r.refuse(report, Conversation{}, NewError(CodeNoConnectionChosen,
			"channel: nothing says which workspace this agent's approvals should be sent from"))
	}

	place := Conversation{Channel: from, DirectApprovals: true}
	to, capped, err := pass.recipientsAmong(ctx, report, place, policy.Notify)
	switch {
	case err != nil:
		return 0, 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, place, err),
		}
	case capped:
		return 0, 1, r.refuse(report, place, NewError(CodeTooManyRecipients,
			"channel: more people can be reached about this than one approval should reach"))
	case len(to) == 0 && len(policy.Notify) > 0:
		// The owner named people and not one of them may decide here. Silence
		// would read as "nobody was reachable", which is a different problem
		// with a different fix.
		return 0, 1, r.refuse(report, place, NewError(CodeNamedNobodyWhoDecides,
			"channel: the people this agent names cannot decide in the run's scope"))
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
	// Owed either way. Nobody bound on this connection is the ordinary state of
	// most of a workspace, and leaving the run pending for it would fill the
	// sweep with runs nobody will ever be told about.
	return sent, 1, refused
}

/*
speakingConnection answers which workspace the bot speaks from when no
conversation names one.

One enabled connection is the answer. None means this installation has no bot to
speak with at all. More than one is genuinely ambiguous, and it is refused
rather than guessed: sending one company's run — its id, its agent, the action
somebody wanted approved — into another company's Slack is the disclosure the
conversation scope exists to prevent, and a coin toss is not a governance rule.

Empty and no error is "nobody could be told, and it is not a failure of this
side"; the caller records why.
*/
func (r *Reporter) speakingConnection(ctx context.Context) (string, error) {
	enabled, err := r.connections.EnabledConnections(ctx)
	if err != nil {
		return "", WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: read the enabled connections: %w", err))
	}
	if len(enabled) != 1 {
		return "", nil
	}
	return enabled[0], nil
}

/*
recipientsAmong is recipients, narrowed to the people an owner named.

The narrowing is an intersection and never a substitution: a name here that
holds no grant in the run's scope is dropped rather than messaged. Naming who
is *told* is addressing; naming who *may decide* is a grant, and an agent's
specification is not where grants are made — the button would refuse the person
it reached, which teaches them the platform is broken.
*/
func (f *fanout) recipientsAmong(
	ctx context.Context, report Report, place Conversation, only []domain.UserID,
) (to []Conversation, capped bool, err error) {
	if len(only) == 0 {
		return f.recipients(ctx, report, place)
	}

	who, err := f.whoDecides(ctx, report.Scope)
	if err != nil {
		return nil, false, err
	}
	named := make(map[domain.UserID]bool, len(only))
	for _, one := range only {
		named[one] = true
	}
	deciding := make([]domain.UserID, 0, len(who))
	for _, one := range who {
		if named[one] {
			deciding = append(deciding, one)
		}
	}
	if len(deciding) == 0 {
		return nil, false, nil
	}
	return f.reachable(ctx, place, deciding)
}
