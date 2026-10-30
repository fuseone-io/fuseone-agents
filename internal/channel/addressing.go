package channel

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
)

/*
Turning people into private conversations on one connection.

Who may decide is a list of users, and a message needs a place. This is where
one becomes the other: which workspace the bot speaks from, who in it has an
account bound, and when there are more of them than one approval should reach.
*/

// Accounts answers where people can be reached on one connection. Absence is
// the ordinary answer: most of a workspace has never been bound.
type Accounts interface {
	AccountsOn(ctx context.Context, channelName string, who []domain.UserID) (map[domain.UserID]string, error)
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

	return f.reachable(ctx, place, who)
}

/*
reachable turns the people who may decide into the private conversations to
post in, or answers that there are too many.

Split from recipients because the owner's path narrows *who* before asking
*where*, and the cap has to be applied after both — counted in messages rather
than in candidates.
*/
func (f *fanout) reachable(
	ctx context.Context, place Conversation, who []domain.UserID,
) (to []Conversation, capped bool, err error) {
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
	// Nothing to ask means nothing to fail. Checked after the question rather
	// than before it, so a lookup that failed for one scope does not answer for
	// another where there was nobody to look up at all.
	if len(missing) == 0 {
		return known, nil
	}
	if err := f.failedChannel[channelName]; err != nil {
		return nil, err
	}

	found, err := f.accounts.AccountsOn(ctx, channelName, missing)
	if err != nil {
		err = WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: where to reach people on %s: %w", channelName, err))
		f.failedChannel[channelName] = err
		return nil, err
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
speakingConnection answers which workspace the bot speaks from.

The run's own conversations answer it first. A scope with a room has already
been told which workspace speaks for it — by somebody who configured that room —
and ignoring it to consult the installation's list would refuse an installation
with two connections for being ambiguous when nothing about it was.

Failing that, one enabled connection is the answer. None means this installation
has no bot to speak with at all. More than one is genuinely ambiguous, and it is
refused rather than guessed: sending one company's run — its id, its agent, the
action somebody wanted approved — into another company's Slack is the disclosure
the conversation scope exists to prevent, and a coin toss is not a governance
rule.

The installation's list is read once per sweep, like everything else this pass
remembers. The rooms are not: they are the run's own, and differ from run to
run.

Empty and no error is "nobody could be told, and it is not a failure of this
side"; the caller records why.
*/
func (f *fanout) speakingConnection(
	ctx context.Context, from Connections, places []Conversation,
) (string, error) {
	// The rooms answer first, and their silence is not the same as their
	// disagreement. Read as one empty string, two rooms on two workspaces fell
	// through to the installation's list — and an installation with a single
	// enabled connection then answered a question its own configuration had
	// just said was ambiguous.
	if named, chosen := connectionAmong(places); chosen {
		return named, nil
	}
	if !f.askedConnections {
		f.askedConnections = true
		enabled, err := from.EnabledConnections(ctx)
		if err != nil {
			f.connectionsErr = WrapError(CodeConfigurationReadFailed,
				fmt.Errorf("channel: read the enabled connections: %w", err))
		}
		if len(enabled) == 1 {
			f.connection = enabled[0]
		}
	}
	return f.connection, f.connectionsErr
}

/*
connectionAmong answers what a scope's rooms say about the workspace, and
whether they said anything at all.

Three states, not two. No rooms is "ask somewhere else". One connection is the
answer. Rooms naming two is an answer as well — *nobody*, because two rooms on
two workspaces is the ambiguity this whole rule is about, arriving from the
conversations instead of from the installation. Collapsed into one empty string,
that third state fell through to the installation's list, and an installation
with a single enabled connection answered a question its own configuration had
just said was ambiguous.

Nothing about a room being configured says which of two workspaces a private
message belongs in.
*/
func connectionAmong(places []Conversation) (named string, chosen bool) {
	for _, place := range places {
		switch {
		case place.Channel == "" || place.Channel == named:
			continue
		case named != "":
			return "", true
		default:
			named = place.Channel
		}
	}
	return named, named != ""
}
