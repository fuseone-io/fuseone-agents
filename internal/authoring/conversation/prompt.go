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

// promptData is what one turn's prompt is rendered from.
type promptData struct {
	Projection   string
	Examples     []string
	MaxProposals int
	Window       []drafts.Turn
	Message      string
	Pasted       bool
}

// Prompt renders a turn's prompt in the author's locale, pt-BR by default.
func Prompt(locale, projection string, window []drafts.Turn, message string, pasted bool) (string, error) {
	name := "prompts/turn.pt-BR.tmpl"
	if strings.HasPrefix(locale, "en") {
		name = "prompts/turn.en-US.tmpl"
	}
	tmpl, err := template.ParseFS(prompts, name)
	if err != nil {
		return "", fmt.Errorf("conversation: read the prompt: %w", err)
	}
	var out bytes.Buffer
	err = tmpl.Execute(&out, promptData{
		Projection: projection, Examples: Examples(), MaxProposals: MaxProposalsPerTurn,
		Window: window, Message: message, Pasted: pasted,
	})
	if err != nil {
		return "", fmt.Errorf("conversation: render the prompt: %w", err)
	}
	return out.String(), nil
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
