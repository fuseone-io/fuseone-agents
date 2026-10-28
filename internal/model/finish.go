package model

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

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
		"headline": map[string]any{
			"type":        "string",
			"description": "Optional one-line header for the run's answer. When set, a channel shows this line and carries the full summary in its thread.",
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
	Headline  string            `json:"headline"`
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
	base.Headline = headlineLine(decoded.Headline)
	return base
}

// maxHeadlineBytes bounds the one line a channel shows. Well under any
// vendor's block limit on purpose: an unbounded headline refused by the
// channel would leave the announcement owed and retried for ever, turning a
// malformed model answer into a permanent operational loop.
const maxHeadlineBytes = 240

// headlineLine is the declared headline reduced to one bounded line: the
// first non-empty line under every break a JSON string carries — \n, \r,
// CRLF, U+2028, U+2029, U+0085 — trimmed, and capped on a rune boundary. A
// multi-line headline posted at a channel's top level is the wallpaper the
// field exists to end.
func headlineLine(declared string) string {
	line := strings.FieldsFunc(declared, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' || r == '\u0085'
	})
	if len(line) == 0 {
		return ""
	}
	out := strings.TrimSpace(line[0])
	if len(out) <= maxHeadlineBytes {
		return out
	}
	cut := maxHeadlineBytes
	for cut > 0 && !utf8.RuneStart(out[cut]) {
		cut--
	}
	return strings.TrimSpace(out[:cut])
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
