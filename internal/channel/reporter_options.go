package channel

import "github.com/fuseone/agents/internal/ticket"

/*
WithDirectApprovals lets a conversation also tell the people who may decide.

Optional because most of this platform's outbound path has nothing to do with
approvals, and a reporter without it simply never sends a private message —
which is what an installation that has not opted in gets anyway.
*/
func (r *Reporter) WithDirectApprovals(who Approvers, where Accounts) *Reporter {
	r.approvers, r.accounts = who, where
	return r
}

/*
WithOwnerApprovals lets an agent's own specification ask for private approvals,
with no conversation involved.

Optional, like the rest of the private path. Without it an agent that asked is
simply not obeyed, which is what an installation running an older worker gets —
and is why the preference is stored versioned rather than acted on at write
time: the record says what was asked, whatever a given process can do about it.
*/
func (r *Reporter) WithOwnerApprovals(what Approvals, from Connections) *Reporter {
	r.approvals, r.connections = what, from
	return r
}

// WithTicketApprovals routes one ticket's approval to its immutable thread
// and to the recipients selected by the configured addressing source.
func (r *Reporter) WithTicketApprovals(routes ticket.ApprovalRoutes) *Reporter {
	r.tickets = routes
	return r
}

// WithConversations replaces where announcements go. Used by tests that need a
// shape the default map does not describe.
func (r *Reporter) WithConversations(c Conversations) *Reporter {
	r.conversations = c
	return r
}

// WithDeliveries records what has been said. Without it nothing is remembered
// and every sweep repeats itself, which is why it is not optional in practice.
func (r *Reporter) WithDeliveries(d Deliveries) *Reporter {
	r.deliveries = d
	return r
}

// WithBaseURL is where a reader goes to act on what they were told. A
// notification about an approval that does not link to the approval is a
// notification that makes somebody go looking.
func (r *Reporter) WithBaseURL(base string) *Reporter {
	r.baseURL = base
	return r
}
