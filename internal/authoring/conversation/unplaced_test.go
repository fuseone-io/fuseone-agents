package conversation_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/conversation"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

/*
A run of an agent with steps is offered only what some step reaches. Not told
so, the model read a granted tool as available, and when a run said it was not,
blamed the connector and the platform. The projection lists the tools no step
reaches, and the prompt says what that means and how to fix it.
*/
func TestProject_listsTheToolsNoStepReaches(t *testing.T) {
	t.Parallel()
	d := draft.New("sentinel", "Sentinel", domain.Scope{Company: "acme", Area: "sec"})
	d.Blocks = blocks.Parse("Objetivo\nVigiar.")
	d.Spec.Tools = []domain.ToolID{"grafana.query", "rootly.create_alert"}
	d.Spec.Steps = []spec.Step{{Name: "Consultar", Reaches: []domain.ToolID{"grafana.query"}}}

	got, err := conversation.Project(d, nil, conversation.Offer{})
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	var out struct {
		Unplaced []string `json:"unplaced"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !slices.Equal(out.Unplaced, []string{"rootly.create_alert"}) {
		t.Errorf("unplaced = %v, want [rootly.create_alert]", out.Unplaced)
	}
}

func TestPrompt_saysAToolNoStepReachesNeverRuns_andHowToPlaceIt(t *testing.T) {
	t.Parallel()
	for locale, want := range map[string][]string{
		"pt-BR": {`"unplaced"`, "nunca é oferecida", "último passo"},
		"en-US": {`"unplaced"`, "is never offered", "last step"},
	} {
		got, err := conversation.Prompt(locale, conversation.PromptInput{Projection: "{}", Message: "Oi"})
		if err != nil {
			t.Fatalf("%s: Prompt: %v", locale, err)
		}
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: the prompt does not say %q", locale, w)
			}
		}
	}
}
