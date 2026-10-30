package httpapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// unknownTools is every tool in a draft's pack the catalogue has never heard
// of, for the area it publishes into.
//
// The Gate never allows such a tool, so an agent published holding one stops
// at its first call — and a typo copied into both `tools` and a step's reaches
// passes every check that only compares the two. A tool the catalogue knows
// but the Curator has not ruled on is not here: that is a server just
// connected, and publishing before its classification is legitimate. Platform
// tools are served by the platform, not listed by any server. With no
// catalogue wired there is nothing to compare against, and nothing is named.
func (s *Server) unknownTools(ctx context.Context, draft spec.Spec) ([]domain.ToolID, error) {
	if s.tools == nil {
		return nil, nil
	}
	catalogue, err := s.rulingsFor(ctx, specScope(draft))
	if err != nil {
		return nil, err
	}
	var unknown []domain.ToolID
	for _, tool := range draft.Tools {
		if _, known := catalogue[tool]; !known && !tool.PlatformOwned() {
			unknown = append(unknown, tool)
		}
	}
	return unknown, nil
}

func unknownToolsDetail(tools []domain.ToolID) string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = string(tool)
	}
	return fmt.Sprintf(
		"no connected server provides %s: correct the name, or connect the server that offers it before publishing",
		strings.Join(names, ", "))
}
