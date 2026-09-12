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
