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
		"artifacts": map[string]any{
			"type":                 "object",
			"description":          "Optional named context artifacts to publish for event listeners. Values are stored by reference and are not sent to listeners unless they request them through FuseOne context.",
			"additionalProperties": map[string]any{"type": "string"},
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
	Summary   string            `json:"summary"`
	Artifacts map[string]string `json:"artifacts"`
	StoppedBy string            `json:"stopped_by"`
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
	base.Artifacts = cleanArtifacts(decoded.Artifacts)
	base.StoppedBy = strings.TrimSpace(decoded.StoppedBy)
	return base
}

/*
cleanArtifacts is what the model asked to publish, minus what it may not.

The reserved name is the part that is not tidying. `final_answer` is what a
citation calls the run's closing answer, and the resolver answers it from
OutcomeRef before it looks at anything the run published — so an artifact under
that name is bytes no citation can reach. Unreachable would be harmless on its
own; the harm is that a screen listing what this run offers would show two
entries with one name, and a memory taught from the second would cite the
first. The name is taken here rather than compared later, because comparing
later means every reader has to know.

Dropped rather than refused. Asking for a name it cannot have is not a reason
to fail a run that has already produced its answer.
*/
func cleanArtifacts(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for name, body := range in {
		name = strings.TrimSpace(name)
		body = strings.TrimSpace(body)
		if name == "" || body == "" {
			continue
		}
		if strings.EqualFold(name, domain.ArtifactFinalAnswer) {
			continue
		}
		out[name] = body
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
