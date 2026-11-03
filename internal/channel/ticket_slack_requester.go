package channel

import (
	"context"
	"log/slog"

	"github.com/fuseone/agents/internal/domain"
)

/*
requesterFor answers who asked for a ticket: the person the account is bound
to, or — in a room that accepts Slack requesters — the account itself.

The account is evidence of who asked and grants nothing: the run is still the
room's configured principal, and deciding still takes a bound person.
*/
func (h *TicketHandler) requesterFor(
	ctx context.Context, arrival Claimed, account string,
) (domain.UserID, bool, error) {
	who, linked, err := h.bindings.PrincipalFor(ctx, arrival.Channel, account)
	if err != nil || linked {
		return who, linked, err
	}
	if !arrival.Ticket.SlackRequesters {
		return "", false, nil
	}
	return domain.SlackRequester(account), true, nil
}

// WithAccounts gives the handler the vendor profiles it names Slack
// requesters from. Optional: without it they are addressed by account alone.
func (h *TicketHandler) WithAccounts(accounts AccountReader) *TicketHandler {
	h.accounts = accounts
	return h
}

// requesterName is what Slack calls a requester nobody bound. Best effort: a
// profile the connection cannot read leaves the ticket unnamed, not unopened.
func (h *TicketHandler) requesterName(ctx context.Context, connection string, who domain.UserID) string {
	account, ok := domain.SlackAccountOf(who)
	if !ok || h.accounts == nil {
		return ""
	}
	profile, err := h.accounts.Account(ctx, connection, account)
	if err != nil {
		slog.Default().Warn("a Slack requester's profile could not be read",
			"channel", connection, "account", account, "error", err)
		return ""
	}
	return profile.Name
}
