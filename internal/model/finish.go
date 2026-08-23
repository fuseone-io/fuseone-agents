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

func finishProposal(args []byte, cost domain.Cost, prompt domain.PromptInputBreakdown) engine.Proposal {
	var decoded finishArgs
	_ = json.Unmarshal(args, &decoded)
	return engine.Proposal{
		Done:      true,
		Outcome:   strings.TrimSpace(decoded.Summary),
		StoppedBy: strings.TrimSpace(decoded.StoppedBy),
		Cost:      cost,
		Prompt:    prompt,
	}
}
