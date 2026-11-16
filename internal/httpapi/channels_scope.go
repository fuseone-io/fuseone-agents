package httpapi

import (
	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
	"github.com/fuseone/agents/internal/settings"
)

// channelAt is where a connection is stored, the installation for a row
// read before connections had a scope.
func channelAt(c admin.Channel) (settings.ScopeKind, domain.Scope) {
	if c.At == "" {
		return settings.ScopeInstallation, domain.Scope{}
	}
	return c.At, c.Stored
}

/*
channelListed is rule 4 for a connection (NT-019, phase 4 writers): listed
when the caller configures it or inherits it, never a sibling area's.

An inherited connection is read-only, and is the platform's or the company's
configuration, not the caller's. It carries only the conversations of scopes
the caller configures or inherits from, and no bindings or seen accounts:
those name other people.
*/
func channelListed(
	p domain.Principal, c admin.Channel,
	bound []admin.ChannelIdentity, seen []admin.ChannelAccountSeen,
) (openapi.Channel, bool) {
	kind, stored := channelAt(c)
	listed, editable := rowListing(p, kind, stored)
	if !listed {
		return openapi.Channel{}, false
	}
	if !editable {
		c.Conversations = conversationsSeenBy(p, c.Conversations)
		bound, seen = nil, nil
	}
	out := channelFrom(c, bound, seen)
	withChannelScope(&out, kind, stored)
	out.Editable = ptr(editable)
	return out, true
}

// conversationsSeenBy keeps the conversations of a scope the caller
// configures, or one that hears them.
func conversationsSeenBy(p domain.Principal, all []admin.Conversation) []admin.Conversation {
	held := p.ScopesFor(permConfigure)
	var out []admin.Conversation
	for _, conv := range all {
		for _, scope := range held {
			if scope.Contains(conv.Scope) || conv.Scope.Contains(scope) {
				out = append(out, conv)
				break
			}
		}
	}
	return out
}

// withChannelScope says where a connection is stored (NT-019).
func withChannelScope(out *openapi.Channel, kind settings.ScopeKind, stored domain.Scope) {
	out.ScopeKind = ptr(openapi.ChannelScopeKind(kind))
	if stored.Company != "" {
		out.Company = ptr(string(stored.Company))
	}
	if stored.Area != "" {
		out.Area = ptr(string(stored.Area))
	}
}
