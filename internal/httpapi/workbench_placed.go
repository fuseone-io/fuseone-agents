package httpapi

import (
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// unplacedTools is a definition with tools no step reaches: a run is offered
// only what its steps reach, so these would never run. The workbench refuses
// to publish or activate it; the JSON API, the file and Terraform do not.
type unplacedTools struct{ tools []domain.ToolID }

func (u unplacedTools) Error() string {
	names := make([]string, 0, len(u.tools))
	for _, t := range u.tools {
		names = append(names, string(t))
	}
	return "no step reaches " + strings.Join(names, ", ") +
		": a run is offered only what its steps reach, so place each in a step or take it out of tools"
}

// checkPlaced refuses a definition with a tool no step reaches.
func checkPlaced(s spec.Spec) error {
	if unplaced := s.Unplaced(); len(unplaced) > 0 {
		return unplacedTools{tools: unplaced}
	}
	return nil
}
