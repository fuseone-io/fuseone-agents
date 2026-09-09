package admin

import (
	"context"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/settings"
)

/*
Where people can be reached on one connection.

The direction this platform normally travels is the other one. An account
arrives, and PrincipalFor has to say who it speaks for; a wrong answer there
hands one person's authority to whoever typed. This goes outward, and a wrong
answer sends a private message to the wrong person — different damage, and a
different set of things to be careful about.

It is deliberately not Identities. That is an administrative listing and shows
a binding that is broken or switched off, because an operator sent to fix one
needs to see it exists. Reach is the opposite question: a binding nobody
enabled must receive nothing, and one nobody can read names nobody. The two
answers are allowed to differ, and here they do.

Asked once per connection rather than once per person. The bindings are
configuration — they change by an administrative act, not by a run — and
re-reading them between recipients would let the set of people a message goes
to change halfway through sending it.
*/
func (c *Channels) AccountsOn(
	ctx context.Context, channelName string, who []domain.UserID,
) (map[domain.UserID]string, error) {
	stored, err := c.settings.List(ctx, KindChannelIdentity)
	if err != nil {
		return nil, fmt.Errorf("admin: list channel identities: %w", err)
	}

	wanted := make(map[domain.UserID]bool, len(who))
	for _, one := range who {
		wanted[one] = true
	}

	where := make(map[domain.UserID]string, len(who))
	for _, s := range stored {
		account, principal, ok := reachable(s, channelName)
		if !ok || !wanted[principal] {
			continue
		}
		/*
			Nobody has bound one person to two accounts on one connection — the
			key is the channel and the account, so nothing stops it. The choice
			is settled here rather than left to whatever comes back first,
			because it is also the idempotency key of the message: an answer
			that changed between sweeps would send the same person the same
			approval again on every one.

			No test separates this from "first wins", and it cannot today:
			settings.List orders by name, so the first row already is the
			smallest. That ordering is not part of what List promises a caller,
			and this is what keeps the answer stable if it ever changes.
		*/
		if held, seen := where[principal]; !seen || account < held {
			where[principal] = account
		}
	}
	return where, nil
}

/*
reachable reads one stored row as an address on this connection, or refuses.

Named by the key, like everything else here: the same two fields inside the
value are a copy that can only ever disagree with the thing doing the work, and
a row keyed `acme-slack/U-real` whose contents claim another account would
address a message to one this connection does not have.

One position, and it is the one authority reads. PrincipalFor resolves an
arriving account by an exact key at the installation scope, and the settings key
includes the scope — so a row for the same channel and account can also sit at a
company or an area, and listing a kind returns every one of them. Accepted, such
a row addresses a message by a mapping the inbound side will never honour: an
area row saying U123 is Ana, beside the installation row saying U123 is Bruno,
sends Ana's approval to Bruno's Slack.

Switched off and unreadable both answer no, and for the same reason from
opposite ends — one is a binding somebody withdrew, the other is a binding
nobody can read. Sending to either is guessing.

Only the first of those has a test that can fail. A row nobody can read parses
to no principal, so the caller's own list already excludes it; the check stays
because the alternative is a caller asking about an empty id and being handed
the broken row as that person's address.
*/
func reachable(s settings.Setting, channelName string) (account string, who domain.UserID, ok bool) {
	if s.ScopeKind != settings.ScopeInstallation || s.Scope != (domain.Scope{}) {
		return "", "", false
	}
	if !s.Enabled {
		return "", "", false
	}
	id, readable := identityFrom(s)
	if !readable || id.Principal == "" {
		return "", "", false
	}
	if keyChannel(s.Name) != channelName {
		return "", "", false
	}
	/*
		A key with no account names a person and not a place. Answered as
		reachable, it hands the next stage the empty conversation the delivery
		table refuses outright — a message attempted against nothing, and a
		recipient the sweep believes it has told.

		Blank the same way the write refuses it, whitespace included: BindIdentity
		rejects an account that is only spaces, so a stored one arrived by restore
		and means no more than an empty string does.

		What comes back is the key as stored, never a tidied copy. PrincipalFor
		matches an arriving account against that exact name, so an address
		trimmed here would be one the inbound side does not resolve — the two
		have to agree about who a row is for, even when the row is wrong.
	*/
	if account = keyAccount(s.Name); strings.TrimSpace(account) == "" {
		return "", "", false
	}
	return account, id.Principal, true
}
