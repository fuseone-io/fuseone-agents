package spec

import (
	"context"

	"github.com/fuseone/agents/internal/domain"
)

/*
ApprovalPolicy answers how an agent's owner asked for approval to arrive.

Read from the version rather than from the agent, because that is where it was
published: a run is governed by the version it pinned, and how its owner wanted
to be told is not an exception to that. An agent whose version cannot be read is
answered with the console alone — the same as one that asked for nothing, which
is the safe direction: a message nobody was meant to get is worse than a
notification that did not go out on top of the console that always does.
*/
func (r *Registry) ApprovalPolicy(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (domain.ApprovalPolicy, error) {
	s, err := r.Get(ctx, agent, version)
	if err != nil {
		return domain.ApprovalPolicy{}, err
	}
	return s.Approvals, nil
}
