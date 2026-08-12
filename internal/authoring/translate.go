package authoring

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
Reading the model's answer.

The model translates business language into specification fields. It never
grants anything: what comes back is read against the catalogue of tools an
operator already connected and classified, and a name that is not in it does
not exist.

That boundary is the point. Without it the interview becomes a way to widen an
agent's reach by describing a process persuasively, which would put the model
on the granting side of a product whose entire argument is that granting is a
human act with a name attached.
*/

// Translated is the half of a draft the model produced.
type Translated struct {
	Tools []domain.ToolID `json:"tools"`
	Steps []spec.Step     `json:"steps"`
}

// Read parses a reply and keeps only what the catalogue supports.
func Read(reply []byte, catalogue []domain.ToolID) (Translated, error) {
	body, ok := jsonIn(reply)
	if !ok {
		return Translated{}, fmt.Errorf("authoring: the reply carried no JSON object")
	}

	var got Translated
	if err := json.Unmarshal(body, &got); err != nil {
		return Translated{}, fmt.Errorf("authoring: unreadable reply: %w", err)
	}

	got.Tools = known(got.Tools, catalogue)
	for i := range got.Steps {
		// The step survives even when everything it named is dropped: a stage
		// that reaches nothing is the agent thinking, which is a real shape.
		// Discarding the step instead would silently lose something the author
		// described.
		got.Steps[i].Reaches = known(got.Steps[i].Reaches, catalogue)
	}
	return got, nil
}

// known keeps the tools an operator connected, in the order the model gave.
func known(named, catalogue []domain.ToolID) []domain.ToolID {
	var out []domain.ToolID
	for _, tool := range named {
		if slices.Contains(catalogue, tool) && !slices.Contains(out, tool) {
			out = append(out, tool)
		}
	}
	return out
}

/*
jsonIn finds the object inside a reply.

Models pad — a courteous sentence, a fenced block. Refusing the whole answer
over that would spend the call and throw it away. Anything that is not an
object at all is refused rather than guessed at: a draft assembled from a
guess would be approved as though the platform had understood something.
*/
func jsonIn(reply []byte) ([]byte, bool) {
	text := string(reply)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil, false
	}
	return []byte(text[start : end+1]), true
}
