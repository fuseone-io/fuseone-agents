package channel_test

import (
	"context"
	"errors"
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

A run leaves the sweep when somebody was told. Counted only in rooms, an agent
answered privately would be read as never announced: it would come back every
thirty seconds for a day, and the page would fill with runs nobody will ever be
told about again.
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

/*
A run nobody could be told about stays in the sweep.

Retiring it writes the sentinel that says it was announced everywhere, once and
for ever — so configuring the connection a minute later, or binding the account,
or opening a room, announces nothing. The record would say why nobody was told
and the run would never be looked at again.

Left unreported it comes back. The window bounds that to a day, and a page that
failed sorts behind runs nobody has tried, so waiting costs the other runs
nothing.
*/
func TestSweep_nobodyCouldBeTold_leavesTheRunInTheSweep(t *testing.T) {
	for _, one := range []struct {
		name     string
		policy   domain.ApprovalPolicy
		from     channel.Connections
		who      channel.Approvers
		accounts channel.Accounts
	}{
		{
			name:   "no connection to speak from",
			policy: domain.ApprovalPolicy{Direct: true},
			from:   connections(), who: deciders("usr_ana"),
			accounts: accountBook{"acme-slack": {"usr_ana": "U-ana"}},
		},
		{
			name:   "two connections and nothing to choose",
			policy: domain.ApprovalPolicy{Direct: true},
			from:   connections("acme-slack", "other-slack"), who: deciders("usr_ana"),
			accounts: accountBook{"acme-slack": {"usr_ana": "U-ana"}},
		},
		{
			name:   "the decider has linked no account",
			policy: domain.ApprovalPolicy{Direct: true},
			from:   oneConnection, who: deciders("usr_ana"),
			accounts: accountBook{},
		},
		{
			name:   "the people named cannot decide",
			policy: domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_bob"}},
			from:   oneConnection, who: deciders("usr_ana"),
			accounts: accountBook{"acme-slack": {"usr_bob": "U-bob"}},
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			reports := &fixedReports{reports: []channel.Report{parkedReport()}}
			posts := &recorder{}
			r := ownerReporterWith(t, reports, posts, wanting(one.policy),
				one.from, one.who, one.accounts)

			if _, err := r.Sweep(context.Background(), 10); err != nil {
				t.Fatalf("Sweep: %v", err)
			}
			if len(posts.sent) != 0 {
				t.Fatalf("sent %+v, want nothing", posts.sent)
			}
			if len(reports.done) != 0 {
				t.Fatalf("reported = %v, want the run kept for a sweep that can tell somebody",
					reports.done)
			}
		})
	}
}

/*
Somebody who may decide and has linked no account is not the owner's list being
wrong.

Both end in nobody being messaged, and they are fixed by different people in
different places: one by whoever binds a Slack account, the other by the owner
editing the agent. Read from one empty list at the end, an owner whose colleague
simply never linked Slack is told their list is wrong.
*/
func TestSweep_aDeciderWithNoAccount_isNotRecordedAsNamingTheWrongPeople(t *testing.T) {
	deliveries := &memoryDeliveries{}
	r := ownerReporterWith(t, &fixedReports{reports: []channel.Report{parkedReport()}},
		&recorder{},
		wanting(domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_ana"}}),
		oneConnection, deciders("usr_ana"), accountBook{}).WithDeliveries(deliveries)

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if recordedFailure(deliveries, channel.CodeNamedNobodyWhoDecides) {
		t.Error("a decider with no linked account was recorded as somebody who cannot decide")
	}
}

/*
One sweep asks each version once, and the connections once.

A page holds fifty runs and an installation has far fewer agents than that, so
the same specification was read and decoded once per run — every thirty seconds,
including for the agents that asked for nothing. Configuration re-read inside one
pass can also answer twice: a sweep obeying two different policies for one
version is a sweep nobody can explain.
*/
func TestSweep_twoRunsOfOneVersion_askItsOwnerOnce(t *testing.T) {
	asked := &askedPolicies{policy: domain.ApprovalPolicy{Direct: true}}
	from := &countingConnections{names: []string{"acme-slack"}}
	second := parkedReport()
	second.RunID = "run-second"

	r := ownerReporterWith(t,
		&fixedReports{reports: []channel.Report{parkedReport(), second}}, &recorder{},
		asked, from, deciders("usr_ana"),
		accountBook{"acme-slack": {"usr_ana": "U-ana"}})

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if asked.calls != 1 {
		t.Errorf("read the policy %d times, want once for the version", asked.calls)
	}
	if from.calls != 1 {
		t.Errorf("listed the connections %d times, want once for the sweep", from.calls)
	}
}

/*
A refusal is remembered for the pass, exactly like an answer.

Remembering only the successes turns an outage into an amplifier: a page of
fifty runs asks fifty times, every thirty seconds. Worse than the cost, a pass
that re-asks can be told two different things inside one sweep — the set of
people one announcement reaches changing halfway through reaching them, which is
the whole reason the pass exists.
*/
func TestSweep_whileTheConfigurationIsUnreadable_asksOncePerSweep(t *testing.T) {
	policies := &failingPolicies{}
	from := &countingConnections{names: []string{"acme-slack"}}
	who := &failingApprovers{}
	second := parkedReport()
	second.RunID = "run-second"
	page := []channel.Report{parkedReport(), second}

	t.Run("the specification cannot be read", func(t *testing.T) {
		r := ownerReporterWith(t, &fixedReports{reports: page}, &recorder{},
			policies, from, deciders("usr_ana"),
			accountBook{"acme-slack": {"usr_ana": "U-ana"}})

		if _, err := r.Sweep(context.Background(), 10); err == nil {
			t.Fatal("Sweep swallowed a configuration that could not be read")
		}
		if policies.calls != 1 {
			t.Errorf("read the policy %d times, want once for the version", policies.calls)
		}
	})

	t.Run("who may decide cannot be read", func(t *testing.T) {
		r := ownerReporterWith(t, &fixedReports{reports: page}, &recorder{},
			wanting(domain.ApprovalPolicy{Direct: true}), from, who,
			accountBook{"acme-slack": {"usr_ana": "U-ana"}})

		if _, err := r.Sweep(context.Background(), 10); err == nil {
			t.Fatal("Sweep swallowed an approver list that could not be read")
		}
		if who.calls != 1 {
			t.Errorf("asked who decides %d times, want once for the scope", who.calls)
		}
	})
}

/*
Why nobody was told is two facts, not one.

Both end in silence and neither is fixed in the same place: one wants somebody
granted Approver, the other wants that person to link their account — a
different screen, and usually a different person. The failure record carries the
code and nothing else, so a code covering both would send whoever reads the
cockpit to guess.
*/
func TestSweep_nobodyTold_recordsWhichKindOfNobody(t *testing.T) {
	for _, one := range []struct {
		name     string
		who      channel.Approvers
		accounts channel.Accounts
		want     string
	}{
		{
			name: "nobody may decide", who: deciders(),
			accounts: accountBook{"acme-slack": {"usr_ana": "U-ana"}},
			want:     channel.CodeNobodyMayDecide,
		},
		{
			name: "nobody has linked an account", who: deciders("usr_ana"),
			accounts: accountBook{},
			want:     channel.CodeNobodyReachable,
		},
	} {
		t.Run(one.name, func(t *testing.T) {
			deliveries := &memoryDeliveries{}
			r := ownerReporterWith(t, &fixedReports{reports: []channel.Report{parkedReport()}},
				&recorder{}, wanting(domain.ApprovalPolicy{Direct: true}),
				oneConnection, one.who, one.accounts).WithDeliveries(deliveries)

			if _, err := r.Sweep(context.Background(), 10); err != nil {
				t.Fatalf("Sweep: %v", err)
			}
			if !recordedFailure(deliveries, one.want) {
				t.Fatalf("failures = %+v, want %s", deliveries.failures, one.want)
			}
		})
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
	calls   int
}

func (a *askedPolicies) ApprovalPolicy(
	_ context.Context, agent domain.AgentID, version domain.VersionID,
) (domain.ApprovalPolicy, error) {
	a.agent, a.version = agent, version
	a.calls++
	return a.policy, nil
}

func wanting(p domain.ApprovalPolicy) *askedPolicies { return &askedPolicies{policy: p} }

type fixedConnections []string

func (f fixedConnections) EnabledConnections(context.Context) ([]string, error) {
	return f, nil
}

func connections(names ...string) fixedConnections { return names }

var oneConnection = fixedConnections{"acme-slack"}

type countingConnections struct {
	names []string
	calls int
}

func (c *countingConnections) EnabledConnections(context.Context) ([]string, error) {
	c.calls++
	return c.names, nil
}

type failingPolicies struct{ calls int }

func (f *failingPolicies) ApprovalPolicy(
	context.Context, domain.AgentID, domain.VersionID,
) (domain.ApprovalPolicy, error) {
	f.calls++
	return domain.ApprovalPolicy{}, errUnavailable
}

var errUnavailable = errors.New("the configuration store is away")
