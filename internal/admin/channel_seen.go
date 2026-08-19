package admin

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ChannelAccountSeen is a Slack account this installation has seen interact
// with a channel. It is a convenience for binding, never authority.
type ChannelAccountSeen struct {
	Channel      string
	Account      string
	Conversation string
	LastSeen     time.Time
}

// MarkAccountSeen remembers that a signed channel event named an account.
//
// No audit event is written here. Seeing an account is not an administrative
// decision, and writing one event per mention would turn the audit trail into
// a telemetry sink. Binding the account is the governed act and is recorded.
func (c *Channels) MarkAccountSeen(
	ctx context.Context, channelName, account, conversation string, at time.Time,
) error {
	channelName = strings.TrimSpace(channelName)
	account = strings.TrimSpace(account)
	if channelName == "" || account == "" {
		return nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	_, err := c.pool.Exec(ctx, `
insert into channel_accounts_seen (channel, account, conversation, last_seen)
values ($1, $2, $3, $4)
on conflict (channel, account) do update
set conversation = case
        when excluded.last_seen >= channel_accounts_seen.last_seen
        then excluded.conversation
        else channel_accounts_seen.conversation
    end,
    last_seen = greatest(channel_accounts_seen.last_seen, excluded.last_seen)`,
		channelName, account, strings.TrimSpace(conversation), at.UTC())
	if err != nil {
		return fmt.Errorf("admin: mark channel account seen: %w", err)
	}
	return nil
}

// SeenAccounts lists the recent account hints for the channel cards.
//
// Bounded per channel so one very noisy workspace cannot make the integrations
// page draw an unbounded directory. A hidden old row is still harmless: the
// next interaction moves it back into the visible set.
func (c *Channels) SeenAccounts(ctx context.Context) ([]ChannelAccountSeen, error) {
	rows, err := c.pool.Query(ctx, `
select channel, account, conversation, last_seen
from (
    select channel, account, conversation, last_seen,
           row_number() over (partition by channel order by last_seen desc) as n
    from channel_accounts_seen
) recent
where n <= 50
order by channel, last_seen desc`)
	if err != nil {
		return nil, fmt.Errorf("admin: list seen channel accounts: %w", err)
	}
	defer rows.Close()

	var out []ChannelAccountSeen
	for rows.Next() {
		var seen ChannelAccountSeen
		if err := rows.Scan(&seen.Channel, &seen.Account, &seen.Conversation, &seen.LastSeen); err != nil {
			return nil, fmt.Errorf("admin: read seen channel account: %w", err)
		}
		out = append(out, seen)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("admin: read seen channel accounts: %w", err)
	}
	return out, nil
}
