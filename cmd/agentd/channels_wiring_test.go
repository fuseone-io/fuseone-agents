package main

import (
	"context"
	"testing"
	"time"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
)

/*
The reporter this process builds obeys what an agent's owner asked for.

Every test of that behaviour builds its own reporter, so all of them stay green
while the worker wires something else — they prove that a reporter *they*
assembled obeys an owner, which nobody deploys. Dropping the policy from the
chain here turns the feature off in production and breaks nothing that is
watched.

So this one asks the assembly the process actually uses. It is a wiring test and
looks like one: fakes for everything, one parked run, and the question is
whether a private message goes out at all.
*/
func TestAnnouncingReporter_obeysTheAgentsOwnApprovalPolicy(t *testing.T) {
	t.Parallel()
	parked := channel.Report{
		RunID: "run-1", AgentID: "triage", Version: "v1",
		Event: channel.EventParked, AtSeq: 4, AwaitingDecision: true,
		Scope: domain.Scope{Company: "acme", Area: "ops"},
		At:    time.Now(),
	}
	posts := &postSpy{}

	reporter := announcingReporter(reporterParts{
		reports:     onePending{report: parked},
		deliveries:  rememberedDeliveries{},
		rooms:       noRooms{},
		poster:      posts,
		approvers:   fixedApprovers{"usr_ana"},
		accounts:    boundAccounts{"usr_ana": "U-ana"},
		policies:    askingPolicy{},
		connections: oneWorkspace{},
	})

	if _, err := reporter.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.to) != 1 || posts.to[0] != "U-ana" {
		t.Fatalf("posted to %v, want the decider told privately", posts.to)
	}
}

type onePending struct{ report channel.Report }

func (o onePending) Unreported(context.Context, time.Time, int) ([]channel.Report, error) {
	return []channel.Report{o.report}, nil
}
func (onePending) Reported(context.Context, channel.Report, time.Time) error { return nil }

type rememberedDeliveries struct{}

func (rememberedDeliveries) Record(context.Context, channel.Delivery) error { return nil }
func (rememberedDeliveries) RecordFailure(context.Context, channel.DeliveryFailure) error {
	return nil
}
func (rememberedDeliveries) RecordFailures(context.Context, []channel.DeliveryFailure) error {
	return nil
}
func (rememberedDeliveries) Delivered(context.Context, channel.Announcement) (bool, error) {
	return false, nil
}

type noRooms struct{}

func (noRooms) For(context.Context, domain.Scope) ([]channel.Conversation, error) {
	return nil, nil
}

type postSpy struct{ to []string }

func (p *postSpy) Post(
	_ context.Context, c channel.Conversation, _ channel.Message,
) (string, error) {
	p.to = append(p.to, c.ID)
	return "1.1", nil
}

type fixedApprovers []domain.UserID

func (f fixedApprovers) ApproversIn(context.Context, domain.Scope) ([]domain.UserID, error) {
	return f, nil
}

type boundAccounts map[domain.UserID]string

func (b boundAccounts) AccountsOn(
	_ context.Context, _ string, who []domain.UserID,
) (map[domain.UserID]string, error) {
	out := map[domain.UserID]string{}
	for _, one := range who {
		if account, bound := b[one]; bound {
			out[one] = account
		}
	}
	return out, nil
}

type askingPolicy struct{}

func (askingPolicy) ApprovalPolicy(
	context.Context, domain.AgentID, domain.VersionID,
) (domain.ApprovalPolicy, error) {
	return domain.ApprovalPolicy{Direct: true}, nil
}

type oneWorkspace struct{}

func (oneWorkspace) EnabledConnections(context.Context) ([]string, error) {
	return []string{"acme-slack"}, nil
}
