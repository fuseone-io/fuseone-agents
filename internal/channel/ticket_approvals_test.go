package channel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/fuseone/agents/internal/channel"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

func TestSweep_aTicketApprovalReachesItsThreadAndOnlyItsNamedDeciders(t *testing.T) {
	posts := &recorder{}
	report := parkedReport()
	report.Ticket = domain.TicketRef{Key: "ticket-1", Revision: 2}
	who := deciders("usr_broadcast").andNameable("usr_manager")
	routes := &fixedTicketRoutes{route: ticket.ApprovalRoute{
		Origin: ticket.Origin{
			Connection: "acme-slack", Conversation: "C07-ops", Root: "171.1",
		},
		Recipients: []domain.UserID{"usr_manager"},
	}}
	r := directReporter(t, posts, who, accountBook{"acme-slack": {
		"usr_broadcast": "U-broadcast", "usr_manager": "U-manager",
	}}, report).WithTicketApprovals(routes).
		WithOwnerApprovals(alwaysDirect{}, ticketConnection{})

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 2 {
		t.Fatalf("sent = %+v, want the ticket room and its named manager", posts.sent)
	}
	if room := posts.sent[0].conversation; room.ID != "C07-ops" || room.Thread != "171.1" {
		t.Errorf("room = %+v, want the ticket's exact thread", room)
	}
	if !addressed(posts.sent, "U-manager") {
		t.Error("the manager named by the trusted addressing source was not told")
	}
	if addressed(posts.sent, "U-broadcast") {
		t.Error("a generic broadcast recipient was added to the ticket")
	}
	if routes.calls != 1 {
		t.Errorf("ticket route read %d times, want one snapshot per sweep", routes.calls)
	}
}

func TestSweep_aTicketRecipientWhoseGrantWasRemoved_isNotTold(t *testing.T) {
	posts := &recorder{}
	report := parkedReport()
	report.Ticket = domain.TicketRef{Key: "ticket-1", Revision: 2}
	r := directReporter(t, posts, deciders("usr_someone_else"),
		accountBook{"acme-slack": {"usr_manager": "U-manager"}}, report).
		WithTicketApprovals(&fixedTicketRoutes{route: ticket.ApprovalRoute{
			Origin: ticket.Origin{
				Connection: "acme-slack", Conversation: "C07-ops", Root: "171.1",
			},
			Recipients: []domain.UserID{"usr_manager"},
		}})

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if addressed(posts.sent, "U-manager") {
		t.Error("a ticket recipient kept addressing authority after losing approval:act")
	}
}

func TestSweep_aTicketRouteThatCannotBeRead_postsNothingAndRetries(t *testing.T) {
	posts := &recorder{}
	reports := &fixedReports{reports: []channel.Report{parkedReport()}}
	reports.reports[0].Ticket = domain.TicketRef{Key: "ticket-1", Revision: 1}
	r := directReporterWith(t, reports, posts, deciders("usr_manager"),
		accountBook{"acme-slack": {"usr_manager": "U-manager"}}).
		WithTicketApprovals(&fixedTicketRoutes{err: errors.New("database unavailable")})

	if _, err := r.Sweep(context.Background(), 10); err == nil {
		t.Fatal("Sweep succeeded without knowing the ticket's approved destination")
	}
	if len(posts.sent) != 0 || len(reports.done) != 0 {
		t.Fatalf("sent=%+v done=%+v, want no disclosure and a retry", posts.sent, reports.done)
	}
}

/*
A ticket with a review room keeps its decision out of the support thread.

The card goes to the room's thread and nowhere else, which is what lets an
answer be corrected before the person who asked reads a word of it.
*/
func TestSweep_aTicketWithAReviewRoom_isDecidedThereAndNotInTheSupportThread(t *testing.T) {
	posts := &recorder{}
	report := parkedReport()
	report.Ticket = domain.TicketRef{Key: "ticket-1", Revision: 2}
	r := directReporter(t, posts, deciders("usr_manager"),
		accountBook{"acme-slack": {"usr_manager": "U-manager"}}, report).
		WithTicketApprovals(&fixedTicketRoutes{route: ticket.ApprovalRoute{
			Origin: ticket.Origin{
				Connection: "acme-slack", Conversation: "C07-ops", Root: "171.1",
			},
			Review: ticket.ReviewRoom{Conversation: "C07-agents", Root: "900.1"},
		}})

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 1 {
		t.Fatalf("sent = %+v, want one card", posts.sent)
	}
	if room := posts.sent[0].conversation; room.ID != "C07-agents" || room.Thread != "900.1" {
		t.Fatalf("room = %+v, want the review room's thread", room)
	}
}

// The room exists and its thread does not yet. The card waits: falling back to
// the support thread would publish the draft the room was configured to hold.
func TestSweep_aTicketWhoseReviewThreadIsNotOpenYet_saysNothingAndStaysOwed(t *testing.T) {
	posts := &recorder{}
	reports := &fixedReports{reports: []channel.Report{parkedReport()}}
	reports.reports[0].Ticket = domain.TicketRef{Key: "ticket-1", Revision: 1}
	r := directReporterWith(t, reports, posts, deciders("usr_manager"),
		accountBook{"acme-slack": {"usr_manager": "U-manager"}}).
		WithTicketApprovals(&fixedTicketRoutes{route: ticket.ApprovalRoute{
			Origin: ticket.Origin{
				Connection: "acme-slack", Conversation: "C07-ops", Root: "171.1",
			},
			Review: ticket.ReviewRoom{Conversation: "C07-agents"},
		}})

	sent, err := r.Sweep(context.Background(), 10)
	if err != nil || sent != 0 {
		t.Fatalf("Sweep = (%d, %v), want a quiet retry", sent, err)
	}
	if len(posts.sent) != 0 || len(reports.done) != 0 {
		t.Fatalf("sent=%+v done=%+v, want the report still owed", posts.sent, reports.done)
	}
}

/*
A ticket's room hears everything about it, and the support thread hears none of
it until an answer is approved.

The card that leaked was not an approval: a run of a ticket stopped, and the
ordinary announcement path published it at the top of the room where the person
who asked is reading. Whatever the event, a run that belongs to a ticket is
reported where the ticket is worked.
*/
func TestSweep_anyEventOfATicketRun_isReportedOnlyInItsRoom(t *testing.T) {
	for name, event := range map[string]channel.Event{
		"failed":  channel.EventFailed,
		"drifted": channel.EventDrifted,
	} {
		t.Run(name, func(t *testing.T) {
			posts := &recorder{}
			report := parkedReport()
			report.Event, report.AwaitingDecision = event, false
			report.Ticket = domain.TicketRef{Key: "ticket-1", Revision: 2}
			r := directReporter(t, posts, deciders("usr_manager"),
				accountBook{"acme-slack": {"usr_manager": "U-manager"}}, report).
				WithTicketApprovals(&fixedTicketRoutes{route: ticket.ApprovalRoute{
					Origin: ticket.Origin{
						Connection: "acme-slack", Conversation: "C07-ops", Root: "171.1",
					},
					Review: ticket.ReviewRoom{Conversation: "C07-agents", Root: "900.1"},
				}})

			if _, err := r.Sweep(context.Background(), 10); err != nil {
				t.Fatalf("Sweep: %v", err)
			}
			if len(posts.sent) != 1 {
				t.Fatalf("sent = %+v, want one card", posts.sent)
			}
			room := posts.sent[0].conversation
			if room.ID != "C07-agents" || room.Thread != "900.1" {
				t.Fatalf("room = %+v, want the review thread", room)
			}
		})
	}
}

// Without a room, a ticket's run is still not announced at the top of the
// support channel: it goes under the request it belongs to.
func TestSweep_aTicketRunWithNoReviewRoom_isReportedUnderItsOwnThread(t *testing.T) {
	posts := &recorder{}
	report := parkedReport()
	report.Event, report.AwaitingDecision = channel.EventFailed, false
	report.Ticket = domain.TicketRef{Key: "ticket-1", Revision: 2}
	r := directReporter(t, posts, deciders("usr_manager"),
		accountBook{"acme-slack": {"usr_manager": "U-manager"}}, report).
		WithTicketApprovals(&fixedTicketRoutes{route: ticket.ApprovalRoute{
			Origin: ticket.Origin{
				Connection: "acme-slack", Conversation: "C07-ops", Root: "171.1",
			},
		}})

	if _, err := r.Sweep(context.Background(), 10); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(posts.sent) != 1 || posts.sent[0].conversation.Thread != "171.1" {
		t.Fatalf("sent = %+v, want one card in the ticket's thread", posts.sent)
	}
}

type fixedTicketRoutes struct {
	route ticket.ApprovalRoute
	err   error
	calls int
}

func (f *fixedTicketRoutes) ApprovalRoute(
	context.Context, domain.TicketRef,
) (ticket.ApprovalRoute, error) {
	f.calls++
	return f.route, f.err
}

type alwaysDirect struct{}

func (alwaysDirect) ApprovalPolicy(
	context.Context, domain.AgentID, domain.VersionID,
) (domain.ApprovalPolicy, error) {
	return domain.ApprovalPolicy{Direct: true}, nil
}

type ticketConnection struct{}

func (ticketConnection) EnabledConnections(context.Context) ([]string, error) {
	return []string{"acme-slack"}, nil
}
