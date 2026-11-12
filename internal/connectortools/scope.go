package connectortools

import "github.com/fuseone/agents/internal/domain"

/*
Scope answers where a tool this layer serves belongs (NT-019).

Two kinds of tool reach a run through here. The ticket answer is the platform's
own, served by this layer because it rides the governed ticket, so it is the
installation's and every scope reaches it; declared here, explicitly, because
nothing else knows this layer serves it. A native connector tool is its
instance's, and an instance stored in an area reaches only that area. Anything
else is the layer below's to answer.
*/
func (l *Layer) Scope(id domain.ToolID) (domain.Scope, bool) {
	if id == TicketAnswerTool {
		return domain.Scope{Company: domain.Installation}, l.answers != nil
	}
	if native, _, ok := l.native(id); ok {
		return native.Scope, native.Enabled
	}
	if l.catalog == nil {
		return domain.Scope{}, false
	}
	return l.catalog.Scope(id)
}
