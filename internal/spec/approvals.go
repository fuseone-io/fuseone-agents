package spec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/fuseone/agents/internal/domain"
)

/*
ApprovalPolicy answers how an agent's owner asked for approval to arrive.

Read from the version rather than from the agent, because that is where it was
published: a run is governed by the version it pinned, and how its owner wanted
to be told is not an exception to that.

One column rather than the whole version. Read through Get it decoded the
instructions, the tools, the triggers, the steps and the emissions to answer one
boolean — on a sweep that reads a page of runs every thirty seconds, including
for every agent that asked for nothing.

A version that cannot be read is an error and not a shrug. The caller keeps the
run and reads again next sweep: answering "the console alone" would be inventing
the owner's decision out of a database being briefly away, and the run would be
retired having told nobody.
*/
func (r *Registry) ApprovalPolicy(
	ctx context.Context, agent domain.AgentID, version domain.VersionID,
) (domain.ApprovalPolicy, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		select approvals from agent_specs
		where agent_id = $1 and version_id = $2`,
		string(agent), string(version)).Scan(&raw)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ApprovalPolicy{}, fmt.Errorf("%w: %s@%s", ErrNotPublished, agent, version)
	}
	if err != nil {
		return domain.ApprovalPolicy{}, fmt.Errorf(
			"spec: read the approvals of %s@%s: %w", agent, version, err)
	}

	var policy domain.ApprovalPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return domain.ApprovalPolicy{}, fmt.Errorf("spec: decode approvals: %w", err)
	}
	return policy.Normalize(), nil
}
