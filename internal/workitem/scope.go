package workitem

import "github.com/fuseone/agents/internal/domain"

// Scope answers where a tool this layer serves belongs (NT-019). Claims and links are
// the platform's own, so they are the installation's and every scope reaches
// them: declared here, explicitly, because only this layer knows it serves
// them. Anything else is the layer below's to answer.
func (l *Layer) Scope(id domain.ToolID) (domain.Scope, bool) {
	if id.Coordination() {
		return domain.Scope{Company: domain.Installation}, true
	}
	if l.catalog == nil {
		return domain.Scope{}, false
	}
	return l.catalog.Scope(id)
}
