package spec

import (
	"context"
	"fmt"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/gate"
)

// Pack is the capability pack of one published version (PRD AU-07).
//
// Of that version, not of the one published now: replaying a run against a
// specification written after it ran would report every decision as diverged
// for a reason that has nothing to do with the run. It is the same rule the
// run followed when it pinned a version in the first place (DE-09).
func (r *Registry) Pack(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (gate.Pack, error) {
	spec, err := r.Get(ctx, agent, version)
	if err != nil {
		return gate.Pack{}, fmt.Errorf("spec: pack of %s@%s: %w", agent, version, err)
	}
	return gate.NewPack(spec.Tools...), nil
}
