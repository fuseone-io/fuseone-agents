package channel

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ticket"
)

const MaxTicketRecipients = 20

var slackMention = regexp.MustCompile(`<@([A-Z0-9]+)>`)

type TicketAddresses interface {
	PrincipalsOn(context.Context, string, []string) (map[string]domain.UserID, error)
}

type TicketDeciders interface {
	DecidersIn(context.Context, domain.Scope) ([]domain.UserID, error)
}

func (h *TicketHandler) address(
	ctx context.Context, arrival Claimed, held ticket.Ticket,
) (TicketResult, error) {
	accounts, bounded := mentionedAccounts(arrival.Text)
	if !bounded {
		return handled("ticket_too_many_recipients"), nil
	}
	if len(accounts) == 0 {
		return handled("ticket_address_without_recipients"), nil
	}
	recipients, err := h.allowedRecipients(ctx, arrival.Channel, held.Scope, accounts)
	if err != nil {
		return TicketResult{}, err
	}
	if slices.Equal(recipients, held.Current.Recipients) {
		return handled("ticket_recipients_unchanged"), nil
	}
	updated, _, err := h.store.Address(ctx, ticket.AddressInput{
		Ref: held.Current.Ref, EventID: arrival.EventID,
		By: held.AddressedBy, Recipients: recipients, At: h.now().UTC(),
	})
	if err != nil {
		return TicketResult{}, err
	}
	return h.openRevision(ctx, updated, updated.Current)
}

func (h *TicketHandler) allowedRecipients(
	ctx context.Context, connection string, scope domain.Scope, accounts []string,
) ([]domain.UserID, error) {
	principals, err := h.addresses.PrincipalsOn(ctx, connection, accounts)
	if err != nil {
		return nil, fmt.Errorf("channel: resolve ticket recipients: %w", err)
	}
	allowed, err := h.deciders.DecidersIn(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("channel: resolve ticket deciders: %w", err)
	}
	canDecide := make(map[domain.UserID]struct{}, len(allowed))
	for _, one := range allowed {
		canDecide[one] = struct{}{}
	}
	recipients := make([]domain.UserID, 0, len(accounts))
	for _, account := range accounts {
		if principal := principals[account]; principal != "" {
			if _, ok := canDecide[principal]; ok {
				recipients = append(recipients, principal)
			}
		}
	}
	slices.Sort(recipients)
	return slices.Compact(recipients), nil
}

func mentionedAccounts(text string) ([]string, bool) {
	matches := slackMention.FindAllStringSubmatch(text, MaxTicketRecipients+1)
	if len(matches) > MaxTicketRecipients {
		return nil, false
	}
	accounts := make([]string, 0, len(matches))
	for _, match := range matches {
		accounts = append(accounts, match[1])
	}
	slices.Sort(accounts)
	return slices.Compact(accounts), true
}
