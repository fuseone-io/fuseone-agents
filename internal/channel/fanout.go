package channel

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

/*
What one sweep remembers about configuration, and the two questions it answers
from it: who is announced to, and who may act.

Configuration changes by an administrative act, never in step with a sweep. Read
again for every run, the set of people one sweep reaches could change halfway
through it, sending a run's approval to one list and its retry to another — so
each answer is read once and kept for the pass, failures included.
*/

/*
answered is what a pass remembers, including a refusal.

Remembering only the successes was an amplifier: during an outage a page of
fifty runs asked fifty times, every thirty seconds, and could be told different
things inside one sweep — the set of people one announcement reaches changing
halfway through reaching them, which is the whole reason this pass exists.

The error is the answer until the next sweep.
*/
type answered[T any] struct {
	value T
	err   error
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
	tickets   ticket.ApprovalRoutes
	byScope   map[domain.Scope]answered[[]domain.UserID]
	mayDecide map[domain.Scope]answered[[]domain.UserID]
	byChannel map[string]map[domain.UserID]string
	// Reading who is reachable fills in one person at a time, so a failure is
	// remembered for the connection rather than for the people it was about.
	failedChannel map[string]error
	// The connection an agent's own approvals are sent from, and whether it has
	// been asked for. Empty is an answer — no connection, or more than one —
	// which is why the asking is tracked separately from the answer.
	connection       string
	askedConnections bool
	connectionsErr   error
	// What each version's owner asked for. Two runs of one agent in a page ask
	// once, and every run of it asks once per sweep rather than per report.
	byVersion map[versionOfAgent]answered[domain.ApprovalPolicy]
	byTicket  map[domain.TicketRef]answered[ticket.ApprovalRoute]
}

// versionOfAgent is what an approval policy is stored against: a run is pinned
// to a version, so two versions of one agent are two answers.
type versionOfAgent struct {
	agent   domain.AgentID
	version domain.VersionID
}

func (r *Reporter) newFanout() *fanout {
	return &fanout{
		approvers: r.approvers, accounts: r.accounts, tickets: r.tickets,
		byScope:       map[domain.Scope]answered[[]domain.UserID]{},
		mayDecide:     map[domain.Scope]answered[[]domain.UserID]{},
		byChannel:     map[string]map[domain.UserID]string{},
		failedChannel: map[string]error{},
		byVersion:     map[versionOfAgent]answered[domain.ApprovalPolicy]{},
		byTicket:      map[domain.TicketRef]answered[ticket.ApprovalRoute]{},
	}
}

func (f *fanout) whoDecides(ctx context.Context, scope domain.Scope) ([]domain.UserID, error) {
	if before, asked := f.byScope[scope]; asked {
		return before.value, before.err
	}
	who, err := f.approvers.ApproversIn(ctx, scope)
	if err != nil {
		err = WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: who may approve in %s: %w", scope, err))
	}
	f.byScope[scope] = answered[[]domain.UserID]{value: who, err: err}
	return who, err
}

// whoMayDecide is the wider question, asked once per scope for the same reason:
// a set of people that changed halfway through a sweep would send a run's
// approval to one list and its retry to another.
func (f *fanout) whoMayDecide(ctx context.Context, scope domain.Scope) ([]domain.UserID, error) {
	if before, asked := f.mayDecide[scope]; asked {
		return before.value, before.err
	}
	who, err := f.approvers.DecidersIn(ctx, scope)
	if err != nil {
		err = WrapError(CodeConfigurationReadFailed,
			fmt.Errorf("channel: who may decide in %s: %w", scope, err))
	}
	f.mayDecide[scope] = answered[[]domain.UserID]{value: who, err: err}
	return who, err
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
decidersAmong is who may decide, narrowed to the people an owner named.

The narrowing is an intersection and never a substitution: a name here that
holds no grant in the run's scope is dropped rather than messaged. Naming who is
*told* is addressing; naming who *may decide* is a grant, and an agent's
specification is not where grants are made — the button would refuse the person
it reached, which teaches them the platform is broken.

Intersected against who may decide, and not against who is announced to. Those
differ by the administrator, who holds the act and is left out of the broadcast
because one granted at the installation would be told about every parked run
there is. Narrowed against the broadcast, the one colleague an owner is most
likely to name was dropped in silence: not refused and not recorded, with the
run marked announced and nobody told.

Answered before anybody is looked up, so that "none of these may decide" and
"none of these has an account" stay two different answers. Read from one empty
list at the end, an owner whose colleague simply never linked Slack would be
told their list was wrong.
*/
func (f *fanout) decidersAmong(
	ctx context.Context, scope domain.Scope, only []domain.UserID,
) ([]domain.UserID, error) {
	if len(only) == 0 {
		return f.whoDecides(ctx, scope)
	}
	who, err := f.whoMayDecide(ctx, scope)
	if err != nil {
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
