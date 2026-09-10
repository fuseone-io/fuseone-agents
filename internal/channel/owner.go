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
should also arrive privately, and that is enough: no conversation, no room, no
mapping from a scope to one. A connection is still needed — a bot with no
workspace to speak from cannot send anything — and which one is the single thing
the room used to supply for free.

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
anybody actually has it.

The second answer decides whether the run leaves the sweep for good, so it means
one thing only: somebody was told. Not "we tried", and not "we recorded why we
could not" — a run retired on those is a run that stays unannounced after the
connection is configured, after the account is bound, after the operator fixes
exactly the thing the record complained about. Nothing would ever say so: the
sentinel is written once and the run is never looked at again.

Left unreported, it comes back. The window bounds that to a day, and a page that
failed now sorts behind runs nobody has tried, so waiting costs the other runs
nothing.
*/
func (r *Reporter) directForOwner(
	ctx context.Context, pass *fanout, report Report, places []Conversation,
) (sent, owed int, refused refusal) {
	if !r.wantsOwnApprovals(report) {
		return 0, 0, refusal{}
	}

	policy, err := pass.policyFor(ctx, r.approvals, report)
	if err != nil {
		// This side being unavailable. Nothing is owed — the next sweep reads
		// the specification again — and it is recorded, because the record is
		// how the announcement says why nobody heard. Left out, the general
		// marker spoke in its place and told an operator that nothing was
		// configured, which sends them to configure what is already there.
		return 0, 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, Conversation{}, err),
		}
	}
	if !policy.Direct {
		return 0, 0, refusal{}
	}

	from, err := pass.speakingConnection(ctx, r.connections, places)
	switch {
	case err != nil:
		return 0, 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, Conversation{}, err),
		}
	case from == "":
		// Recorded and not owed. Nobody has this approval, and the day
		// somebody configures a connection the run is still in the sweep to be
		// told about — which is the whole point of not retiring it.
		return 0, 0, r.refuse(report, Conversation{}, NewError(CodeNoConnectionChosen,
			"channel: nothing says which workspace this agent's approvals should be sent from"))
	}

	place := Conversation{Channel: from, DirectApprovals: true}
	who, err := pass.decidersAmong(ctx, report.Scope, policy.Notify)
	if err != nil {
		return 0, 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, place, err),
		}
	}
	// Nobody to tell, answered before any binding is looked up. Asked
	// afterwards, a connection whose account lookup happened to be failing
	// would answer "the configuration could not be read" about a scope where
	// the real fact — that nobody may decide in it — was already known.
	if len(who) == 0 {
		if len(policy.Notify) > 0 {
			// The owner's own list is wrong, which is a different problem from
			// nobody holding the grant, and has a different fix.
			return 0, 0, r.refuse(report, place, NewError(CodeNamedNobodyWhoDecides,
				"channel: the people this agent names cannot decide in the run's scope"))
		}
		return 0, 0, r.refuse(report, place, NewError(CodeNobodyMayDecide,
			"channel: nobody may decide in this run's scope"))
	}

	to, capped, err := pass.reachable(ctx, place, who)
	switch {
	case err != nil:
		return 0, 0, refusal{
			blocking: []error{err},
			recorded: r.failuresFor(report, place, err),
		}
	case capped:
		return 0, 0, r.refuse(report, place, NewError(CodeTooManyRecipients,
			"channel: more people can be reached about this than one approval should reach"))
	}

	// Delivered counts the people who have it, including the ones a previous
	// sweep already told: the run is theirs to answer whether the message went
	// out a moment ago or an hour ago.
	delivered := 0
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
		delivered++
		if posted {
			sent++
		}
	}
	if len(to) == 0 {
		/*
			Nobody could be told, and it is written down.

			Not retired, so the first grant or binding somebody creates is
			answered by the next sweep — and recorded, which is the half that
			was missing: the sweep takes what has not been tried before what
			has, and a run with no attempt against it is "never tried" for ever.
			With more runs than fit in a page, an older run that *could* be
			told would sit behind them until it left the window, announced to
			nobody.

			Nobody may decide is answered further up, before any binding is
			looked up: they are two facts with two fixes, one a grant and the
			other a linked account, and the second must not speak for the
			first.
		*/
		return 0, 0, r.refuse(report, place, NewError(CodeNobodyReachable,
			"channel: nobody who may decide has linked an account on this connection"))
	}
	if delivered == 0 {
		// Everybody who should have been told failed, and each failure was
		// recorded above. Kept for the next sweep.
		return 0, 0, refused
	}
	return sent, 1, refused
}

/*
policyFor answers what one version's owner asked for, once per sweep.

Remembered by version rather than by run: a page holds fifty runs and an
installation has far fewer agents than that, so the same specification was being
read and decoded once for every run of it. It is also configuration, and a pass
that re-read it could obey two different answers inside one sweep.
*/
func (f *fanout) policyFor(
	ctx context.Context, from Approvals, report Report,
) (domain.ApprovalPolicy, error) {
	key := versionOfAgent{agent: report.AgentID, version: report.Version}
	if before, asked := f.byVersion[key]; asked {
		return before.value, before.err
	}
	policy, err := from.ApprovalPolicy(ctx, report.AgentID, report.Version)
	if err != nil {
		err = WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: read the approval policy of %s: %w", report.AgentID, err))
	}
	f.byVersion[key] = answered[domain.ApprovalPolicy]{value: policy, err: err}
	return policy, err
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

/*
decidersAmong is who may decide, narrowed to the people an owner named.

The narrowing is an intersection and never a substitution: a name here that
holds no grant in the run's scope is dropped rather than messaged. Naming who is
*told* is addressing; naming who *may decide* is a grant, and an agent's
specification is not where grants are made — the button would refuse the person
it reached, which teaches them the platform is broken.

Answered before anybody is looked up, so that "none of these may decide" and
"none of these has an account" stay two different answers. Read from one empty
list at the end, an owner whose colleague simply never linked Slack would be
told their list was wrong.
*/
func (f *fanout) decidersAmong(
	ctx context.Context, scope domain.Scope, only []domain.UserID,
) ([]domain.UserID, error) {
	who, err := f.whoDecides(ctx, scope)
	if err != nil || len(only) == 0 {
		return who, err
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
	return deciding, nil
}
