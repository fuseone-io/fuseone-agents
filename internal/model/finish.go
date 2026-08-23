package model

import (
	"encoding/json"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
)

const finishToolID domain.ToolID = "$fuseone.finish"

const finishToolDescription = "Finish this run with the final answer. Use it only when no more tool call is needed."

func finishToolSchema() map[string]any {
	return map[string]any{
		"summary": map[string]any{
			"type":        "string",
			"description": "The final answer to record for the run.",
		},
		"stopped_by": map[string]any{
			"type":        "string",
			"description": "When the step's declared stopping condition happened, copy that condition exactly here.",
		},
	}
}

func isFinishTool(id domain.ToolID) bool {
	return id == finishToolID
}

type finishArgs struct {
	Summary   string `json:"summary"`
	StoppedBy string `json:"stopped_by"`
}

func finishProposal(
	args []byte, base engine.Proposal,
) engine.Proposal {
	var decoded finishArgs
	_ = json.Unmarshal(args, &decoded)

	// Built from the proposal the call already produced rather than by listing
	// its fields. Listing them dropped provider and model the moment those
	// existed, silently, and would drop the next one the same way — a finished
	// run would carry cost with nothing saying which model spent it.
	base.Done = true
	base.Outcome = strings.TrimSpace(decoded.Summary)
	base.StoppedBy = strings.TrimSpace(decoded.StoppedBy)
	return base
}
