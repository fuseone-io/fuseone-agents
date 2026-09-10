package spec

import (
	"bytes"
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

	return decodeApprovals(raw)
}

// ErrUnreadableApprovals means a stored policy is a shape this version cannot
// honour. Refused rather than read as much of it as happens to fit: a field
// this binary does not know is a decision somebody took that it cannot carry
// out, and obeying the half it recognises is obeying something nobody wrote.
var ErrUnreadableApprovals = errors.New("spec: that approval policy is not one this version knows")

/*
decodeApprovals reads a stored policy the way publishing wrote it.

Written by one path and read by another, the two drifted: authoring refuses a
policy that names people while asking for no message, and the read accepted it —
so a row that could only have arrived by restore or by a newer version was
obeyed in a shape the platform says is meaningless.

Unknown fields are refused for the same reason. Absent and null are refused too:
the column is not null and defaults to an object, so neither is a policy anybody
wrote — it is a row that lost its meaning somewhere, and reading it as "asked
for nothing" would be inventing an answer.
*/
func decodeApprovals(raw []byte) (domain.ApprovalPolicy, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return domain.ApprovalPolicy{}, fmt.Errorf("%w: it is empty", ErrUnreadableApprovals)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var policy domain.ApprovalPolicy
	if err := decoder.Decode(&policy); err != nil {
		return domain.ApprovalPolicy{}, fmt.Errorf("%w: %v", ErrUnreadableApprovals, err)
	}

	policy = policy.Normalize()
	if err := policy.Validate(); err != nil {
		return domain.ApprovalPolicy{}, fmt.Errorf("%w: %v", ErrUnreadableApprovals, err)
	}
	return policy, nil
}
