package conversation

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/authoring/drafts"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

//go:embed prompts/*.tmpl
var prompts embed.FS

// PromptInput is what one turn's prompt is built from: the draft's projection, the window
// of earlier turns with what became of each one's proposals, and the message.
type PromptInput struct {
	Projection string
	Window     []drafts.Turn
	// Outcomes is each earlier turn's proposals, by the turn's id (#413).
	Outcomes map[string][]Outcome
	Message  string
	Pasted   bool
}

// promptData is what one turn's prompt is rendered from.
type promptData struct {
	Projection   string
	Examples     []string
	BlockKinds   string
	MaxProposals int
	Window       []exchange
	Message      string
	Pasted       bool
}

// exchange is one earlier turn as the prompt shows it, its outcomes in the locale's words.
type exchange struct {
	Message, Reply string
	Outcomes       []string
}

// Prompt renders a turn's prompt in the author's locale, pt-BR by default.
func Prompt(locale string, in PromptInput) (string, error) {
	name, words := "prompts/turn.pt-BR.tmpl", fateWords["pt-BR"]
	if strings.HasPrefix(locale, "en") {
		name, words = "prompts/turn.en-US.tmpl", fateWords["en-US"]
	}
	tmpl, err := template.ParseFS(prompts, name)
	if err != nil {
		return "", fmt.Errorf("conversation: read the prompt: %w", err)
	}
	window := make([]exchange, 0, len(in.Window))
	for _, t := range in.Window {
		e := exchange{Message: t.Message, Reply: t.Reply}
		for _, o := range in.Outcomes[t.ID] {
			e.Outcomes = append(e.Outcomes, o.Kind+": "+words[o.Fate])
		}
		window = append(window, e)
	}
	var out bytes.Buffer
	err = tmpl.Execute(&out, promptData{
		Projection: in.Projection, Examples: Examples(), BlockKinds: blockKinds(), MaxProposals: MaxProposalsPerTurn,
		Window: window, Message: in.Message, Pasted: in.Pasted,
	})
	if err != nil {
		return "", fmt.Errorf("conversation: render the prompt: %w", err)
	}
	return out.String(), nil
}

// blockKinds is every block kind, quoted, from the codec: the prompt names no other.
func blockKinds() string {
	kinds := blocks.Kinds()
	quoted := make([]string, 0, len(kinds))
	for _, k := range kinds {
		quoted = append(quoted, `"`+string(k)+`"`)
	}
	return strings.Join(quoted, ", ")
}

/*
Examples is one body per operation the model may propose, rendered by
draft.Body itself: the shapes the prompt shows are the shapes ParseBody reads,
whatever the operations' fields become. The human-only operations are not
offered.
*/
func Examples() []string {
	samples := []draft.Operation{
		draft.SetModel{Provider: "openai", Model: "gpt-x", Effort: "medium"},
		draft.SetFallback{Pair: domain.ModelPair{Provider: "openai", Model: "gpt-y"}},
		draft.SetCachePrefixTTL{TTL: "1h"},
		draft.SetReplyLocale{Locale: "pt-BR"},
		draft.AddTool{Tool: "crm.lookup"}, draft.RemoveTool{Tool: "crm.lookup"},
		draft.SetBudget{Budget: domain.Budget{Micros: 500000, Steps: 60}},
		draft.SetTriggers{Triggers: []spec.Trigger{{Type: "cron", Schedule: "0 9 * * 1-5"}}},
		draft.SetMemoryLearning{Policy: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningOff}},
		draft.SetApprovalDelivery{Policy: domain.ApprovalPolicy{Direct: true}},
		draft.SetEmits{Emits: spec.Emits{{Event: "ticket.triaged"}}},
		draft.AddStep{Step: spec.Step{Name: "Look up", Reaches: []domain.ToolID{"crm.lookup"}}},
		draft.EditStep{Name: "Look up", Step: spec.Step{Name: "Look up the customer"}},
		draft.RemoveStep{Name: "Look up"},
		draft.InsertBlock{At: 0, Type: blocks.Never, Text: "Promise a deadline.", Locale: "en-US"},
		draft.ReplaceBlock{ID: "b1", Text: "Triage the ticket.", Locale: "en-US"},
		draft.RemoveBlock{ID: "b2"}, draft.MoveBlock{ID: "b1", To: 0},
	}
	out := make([]string, 0, len(samples))
	for _, op := range samples {
		out = append(out, string(draft.Body(op)))
	}
	return out
}
