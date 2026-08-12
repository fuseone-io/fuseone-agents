package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/model"
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

// ErrOverCeiling means the day's authoring budget is already gone.
var ErrOverCeiling = errors.New("authoring: the daily ceiling is reached")

// ErrDisabled means the installation has no authoring assistant switched on.
var ErrDisabled = errors.New("authoring: the assistant is switched off")

// Job is one translation: the answers to turn into fields, and the bounds it
// happens inside.
type Job struct {
	Completer model.Completer
	Choice    Choice
	// SpentToday is what authoring has already cost since the window opened.
	SpentToday int64
	Answers    Answers
	Catalogue  []domain.ToolID
}

// Result is what came back, and what it cost.
type Result struct {
	Translated Translated
	Cost       domain.Cost
}

/*
Translate turns the interview's prose answers into specification fields.

The ceiling is checked before the request leaves rather than after the answer
arrives. A ceiling enforced afterwards is a report: the money is already gone,
and the only thing it can do is describe the overspend.

The cost comes back on every path, including the ones that fail. It left the
installation whether or not the answer was usable, and dropping the figure when
the reply is unreadable is how a ceiling drifts away from what was really
spent — quietly, and in the direction that costs money.
*/
func Translate(ctx context.Context, job Job) (Result, error) {
	if !job.Choice.Enabled {
		return Result{}, ErrDisabled
	}
	if job.Choice.DailyMicros > 0 && job.SpentToday >= job.Choice.DailyMicros {
		return Result{}, ErrOverCeiling
	}

	out, err := job.Completer.Complete(ctx, Prompt(job.Answers, job.Catalogue))
	if err != nil {
		return Result{Cost: out.Cost}, err
	}

	translated, err := Read([]byte(out.Text), job.Catalogue)
	if err != nil {
		return Result{Cost: out.Cost}, err
	}
	return Result{Translated: translated, Cost: out.Cost}, nil
}

/*
Prompt is what the assistant is asked.

It names the tools that exist and says the reply is read against them. That is
not a hint the model may take or leave — Read enforces it — but saying so
turns a silently discarded invention into an answer that fits the first time,
for the price of a few words.
*/
func Prompt(a Answers, catalogue []domain.ToolID) string {
	var b strings.Builder
	b.WriteString("Você organiza a descrição de um processo em campos.\n\n")
	b.WriteString("Ferramentas disponíveis, e só estas existem:\n")
	for _, tool := range catalogue {
		b.WriteString("- " + string(tool) + "\n")
	}
	b.WriteString("\nO que a pessoa respondeu:\n")
	b.WriteString("Precisa saber: " + a.MustKnow + "\n")
	b.WriteString("Passos: " + a.Steps + "\n")
	b.WriteString("O que dá errado: " + a.GoesWrong + "\n")
	b.WriteString("Não decidiria sozinha: " + a.NotDecide + "\n")
	b.WriteString(`
Responda só com JSON:
{"tools":["..."],"steps":[{"name":"...","reaches":["..."],"stops_when":"..."}]}

Um passo pode não alcançar ferramenta nenhuma: é a pessoa pensando.
Não invente ferramenta fora da lista; nomes fora dela são descartados.
`)
	return b.String()
}
