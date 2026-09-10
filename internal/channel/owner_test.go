package channel_test

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
An agent asks for its approvals privately, and there is no room anywhere.

This is the case the whole path exists for. An agent published in an area nobody
mapped to a conversation produced nothing at all: no card, no message, and no
record that anybody should have been told. Its owner can now say so in the
specification, and the people who may decide are asked.
*/
func TestSweep_anAgentThatAsksPrivately_isAnsweredWithNoRoomAtAll(t *testing.T) {
	posts := &recorder{}
	r := ownerReporter(t, posts, wanting(domain.ApprovalPolicy{Direct: true}),
		oneConnection, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}}, parkedReport())

	sent, err := r.Sweep(context.Background(), 10)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if sent != 1 || !addressed(posts.sent, "U-ana") {
		t.Fatalf("sent = %d to %+v, want the decider told privately", sent, posts.sent)
	}
}

/*
And the run is retired, though no conversation was owed anything.

A run leaves the sweep when something was owed a message and nothing failed in a
way another pass would fix. Counted only in rooms, an agent answered privately
would be read as never announced: it would come back every thirty seconds for a
day, and the page would fill with runs nobody will ever be told about again.
*/
func TestSweep_anAgentAnsweredPrivately_isNotAnnouncedForEver(t *testing.T) {
	reports := &fixedReports{reports: []channel.Report{parkedReport()}}
	r := ownerReporterWith(t, reports, &recorder{},
		wanting(domain.ApprovalPolicy{Direct: true}), oneConnection,
		deciders("usr_ana"), accountBook{"acme-slack": {"usr_ana": "U-ana"}})

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(reports.done) != 1 {
		t.Fatalf("reported = %v, want the run retired from the sweep", reports.done)
	}
}

// An agent that asked for nothing gets the console alone, which is how every
// agent behaved before one could ask.
func TestSweep_anAgentThatAsksForNothing_reachesNobodyPrivately(t *testing.T) {
	posts := &recorder{}
	r := ownerReporter(t, posts, wanting(domain.ApprovalPolicy{}), oneConnection,
		deciders("usr_ana"), accountBook{"acme-slack": {"usr_ana": "U-ana"}},
		parkedReport())

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent %+v, want nothing", posts.sent)
	}
}

/*
With two workspaces and nothing to choose between them, nobody is told.

No conversation means nothing names the connection the bot speaks from. Sending
one company's run — its id, its agent, the action somebody wanted approved —
into another company's Slack is the disclosure the conversation scope exists to
prevent, and a coin toss is not a governance rule. The reason is recorded, so
the silence is a fact somebody can find.
*/
func TestSweep_moreThanOneWorkspace_tellsNobodyAndSaysWhy(t *testing.T) {
	posts := &recorder{}
	deliveries := &memoryDeliveries{}
	r := ownerReporterWith(t, &fixedReports{reports: []channel.Report{parkedReport()}},
		posts, wanting(domain.ApprovalPolicy{Direct: true}),
		connections("acme-slack", "other-slack"), deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}}).WithDeliveries(deliveries)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 0 {
		t.Fatalf("sent %+v, want nothing", posts.sent)
	}
	if !recordedFailure(deliveries, channel.CodeNoConnectionChosen) {
		t.Errorf("failures = %+v, want the reason nobody was told", deliveries.failures)
	}
}

/*
Naming somebody who cannot decide tells nobody, and says so.

Addressing is not authorising: the button is checked against the run's own scope
wherever it is pressed, so a message to somebody with no grant is a message with
a button that refuses the person who received it. Silently sending nothing would
read as "nobody was reachable", which is a different problem with a different
fix.
*/
func TestSweep_theOwnerNamesNobodyWhoDecides_tellsNobodyAndSaysWhy(t *testing.T) {
	posts := &recorder{}
	deliveries := &memoryDeliveries{}
	r := ownerReporterWith(t, &fixedReports{reports: []channel.Report{parkedReport()}},
		posts, wanting(domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_bob"}}),
		oneConnection, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana", "usr_bob": "U-bob"}}).
		WithDeliveries(deliveries)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if addressed(posts.sent, "U-bob") {
		t.Error("somebody who cannot decide was sent an approval to decide")
	}
	if !recordedFailure(deliveries, channel.CodeNamedNobodyWhoDecides) {
		t.Errorf("failures = %+v, want the reason nobody was told", deliveries.failures)
	}
}

// Naming people narrows who is told among those who may decide. It never widens
// it, and it never reaches the ones the owner left out.
func TestSweep_theOwnerNamesOneDecider_tellsThatOneAlone(t *testing.T) {
	posts := &recorder{}
	r := ownerReporter(t, posts,
		wanting(domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_ana"}}),
		oneConnection, deciders("usr_ana", "usr_bob"),
		accountBook{"acme-slack": {"usr_ana": "U-ana", "usr_bob": "U-bob"}},
		parkedReport())

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if !addressed(posts.sent, "U-ana") {
		t.Error("the person the owner named was not told")
	}
	if addressed(posts.sent, "U-bob") {
		t.Error("somebody the owner left out was told")
	}
}

/*
A person told through a room is not told again by the agent.

The two paths post to the same private conversation, and a delivery is keyed by
the run, the step and where the message went — so the second one finds it
already said. Two identical approval cards from the same bot for the same step
is the platform looking broken, and it is the failure a second beginning invites.
*/
func TestSweep_toldThroughARoomAndByTheAgent_isToldOnce(t *testing.T) {
	posts := &recorder{}
	r := channel.NewReporter(
		&fixedReports{reports: []channel.Report{parkedReport()}},
		rooms(room("C07-ops", true)), posts,
		func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).
		WithDirectApprovals(deciders("usr_ana"), accountBook{"acme-slack": {"usr_ana": "U-ana"}}).
		WithOwnerApprovals(wanting(domain.ApprovalPolicy{Direct: true}), oneConnection)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if got := countAddressed(posts.sent, "U-ana"); got != 1 {
		t.Fatalf("told %d times, want once", got)
	}
}

/*
The specification is read for the version the run pinned.

A run is governed by the version it began under. Read from whatever is published
now, a run that started this morning would be announced according to a decision
taken this afternoon — the ledger showing one thing while the notification obeyed
another.
*/
func TestSweep_theAgentsWord_isReadForTheVersionTheRunPinned(t *testing.T) {
	asked := &askedPolicies{policy: domain.ApprovalPolicy{Direct: true}}
	report := parkedReport()
	report.Version = "v-3"

	r := ownerReporter(t, &recorder{}, asked, oneConnection, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}}, report)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if asked.version != "v-3" {
		t.Errorf("asked about version %q, want the one the run pinned", asked.version)
	}
}

// --- the harness ------------------------------------------------------------

func ownerReporter(
	t *testing.T, posts *recorder, what channel.Approvals, from channel.Connections,
	who channel.Approvers, where channel.Accounts, reports ...channel.Report,
) *channel.Reporter {
	t.Helper()
	return ownerReporterWith(t, &fixedReports{reports: reports}, posts, what, from, who, where)
}

// The rooms are deliberately empty: this path is what an installation with no
// conversation at all gets.
func ownerReporterWith(
	t *testing.T, reports channel.Reports, posts *recorder,
	what channel.Approvals, from channel.Connections,
	who channel.Approvers, where channel.Accounts,
) *channel.Reporter {
	t.Helper()
	return channel.NewReporter(
		reports, rooms(), posts, func() time.Time { return noon }, nil,
	).WithDeliveries(&memoryDeliveries{}).
		WithDirectApprovals(who, where).
		WithOwnerApprovals(what, from)
}

type askedPolicies struct {
	policy  domain.ApprovalPolicy
	agent   domain.AgentID
	version domain.VersionID
}

func (a *askedPolicies) ApprovalPolicy(
	_ context.Context, agent domain.AgentID, version domain.VersionID,
) (domain.ApprovalPolicy, error) {
	a.agent, a.version = agent, version
	return a.policy, nil
}

func wanting(p domain.ApprovalPolicy) *askedPolicies { return &askedPolicies{policy: p} }

type fixedConnections []string

func (f fixedConnections) EnabledConnections(context.Context) ([]string, error) {
	return f, nil
}

func connections(names ...string) fixedConnections { return names }

var oneConnection = fixedConnections{"acme-slack"}
