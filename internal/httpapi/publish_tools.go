package httpapi

import (
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// unofferedTools is every tool in a draft's pack this installation does not
// offer, for the area it publishes into: unknown to the catalogue, or
// discovered and kept off the surface.
//
// Either way the tool is never described to a model nor invoked, so an agent
// published holding one stops at its first call — and a typo copied into both
// `tools` and a step's reaches passes every check that only compares the two.
// A tool on the surface the Curator has not ruled on is not here: that is a
// server just connected, and publishing before its classification is
// legitimate. Platform tools are served by the platform, not listed by any
// server. With no catalogue wired there is nothing to compare against.
func (s *Server) unofferedTools(draft spec.Spec, catalogue ruled) []domain.ToolID {
	if s.tools == nil {
		return nil
	}
	var unoffered []domain.ToolID
	for _, tool := range draft.Tools {
		if entry, known := catalogue[tool]; (!known || !entry.OnSurface) && !tool.PlatformOwned() {
			unoffered = append(unoffered, tool)
		}
	}
	return unoffered
}

func unofferedToolsDetail(tools []domain.ToolID) string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = string(tool)
	}
	return fmt.Sprintf(
		"this installation does not offer %s: correct the name, or connect the server that "+
			"provides it and bring the tool onto the surface before publishing",
		strings.Join(names, ", "))
}
