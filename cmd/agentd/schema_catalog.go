package main

import "github.com/fuseone/agents/internal/domain"

// schemaCatalog is the planner's view of tool schemas: it describes tools and
// rules on none of them.
type schemaCatalog struct{ schemas toolSchemas }

func (s schemaCatalog) Effect(domain.ToolID) (domain.Effect, bool) {
	return domain.EffectUnknown, false
}

func (s schemaCatalog) Dedupe(domain.ToolID) (domain.ToolDedupe, bool) {
	return domain.ToolDedupe{}, false
}

// Scope answers nothing, like Effect: this catalogue describes tools and rules
// on none of them.
func (s schemaCatalog) Scope(domain.ToolID) (domain.Scope, bool) { return domain.Scope{}, false }

func (s schemaCatalog) Schema(id domain.ToolID) (string, string, map[string]any, bool) {
	if s.schemas == nil {
		return "", "", nil, false
	}
	return s.schemas.Schema(id)
}
