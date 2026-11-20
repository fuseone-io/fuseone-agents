package conversation_test

import (
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/conversation"
	"github.com/fuseone/agents/internal/authoring/drafts"
)

/*
The model is told what became of what it proposed before (#413). The window sent the
earlier messages and replies only, so a model that proposed a ceiling last turn assumed it
was in place and said so, although the person had not accepted it.
*/
func TestOutcomes_groupTheThreadByTurn_eachWithItsFate(t *testing.T) {
	t.Parallel()
	got := conversation.Outcomes([]drafts.Proposal{
		{Turn: "t1", Kind: "set_budget", Valid: true, Decision: "accepted"},
		{Turn: "t1", Kind: "set_triggers", Valid: true, Current: true},
		{Turn: "t1", Kind: "set_model", Valid: true},
		{Turn: "t2", Kind: "insert_block", Valid: false},
		{Turn: "t2", Kind: "add_tool", Valid: true, Decision: "rejected"},
		{Turn: "", Kind: "set_model", Valid: true, Current: true},
	})
	want := map[string][]conversation.Outcome{
		"t1": {{Kind: "set_budget", Fate: conversation.FateAccepted}, {Kind: "set_triggers", Fate: conversation.FateWaiting},
			{Kind: "set_model", Fate: conversation.FateStale}},
		"t2": {{Kind: "insert_block", Fate: conversation.FateInvalid}, {Kind: "add_tool", Fate: conversation.FateRejected}},
	}
	if len(got) != len(want) {
		t.Fatalf("outcomes = %+v, want two turns and nothing for a proposal of none", got)
	}
	for turn, outcomes := range want {
		for i, o := range outcomes {
			if len(got[turn]) != len(outcomes) || got[turn][i] != o {
				t.Errorf("%s = %+v, want %+v", turn, got[turn], outcomes)
				break
			}
		}
	}
}

func TestPrompt_saysWhatBecameOfEachEarlierProposal_inItsTurn(t *testing.T) {
	t.Parallel()
	for locale, want := range map[string][]string{
		"pt-BR": {"set_budget: aceita", "set_triggers: ainda esperando", "set_model: obsoleta"},
		"en-US": {"set_budget: accepted", "set_triggers: still waiting", "set_model: stale"},
	} {
		got, err := conversation.Prompt(locale, conversation.PromptInput{
			Projection: "{}", Message: "E o gatilho?",
			Window: []drafts.Turn{{ID: "t1", Message: "Ponha um teto e um gatilho.", Reply: "Proponho os dois."}},
			Outcomes: map[string][]conversation.Outcome{"t1": {
				{Kind: "set_budget", Fate: conversation.FateAccepted}, {Kind: "set_triggers", Fate: conversation.FateWaiting},
				{Kind: "set_model", Fate: conversation.FateStale},
			}},
		})
		if err != nil {
			t.Fatalf("%s: Prompt: %v", locale, err)
		}
		reply, end := strings.Index(got, "Proponho os dois."), strings.Index(got, "</conversation>")
		for _, w := range want {
			at := strings.Index(got, w)
			if at < reply || at > end {
				t.Errorf("%s: %q is not in its turn, inside <conversation>:\n%s", locale, w, got)
			}
		}
	}
}

// The reply speaks of a proposal as a proposal, and the draft shown is what is applied.
func TestPrompt_forbidsSayingAProposalWasApplied(t *testing.T) {
	t.Parallel()
	for locale, want := range map[string]string{
		"pt-BR": "Nunca diga que acrescentou, mudou, removeu ou aplicou",
		"en-US": "Never say you added, changed, removed or applied",
	} {
		got, err := conversation.Prompt(locale, conversation.PromptInput{Projection: "{}", Message: "Oi"})
		if err != nil || !strings.Contains(got, want) {
			t.Errorf("%s: the prompt does not forbid it (%v)", locale, err)
		}
	}
}

// The block kinds there are, from the codec itself, and the positions a block may go to:
// the model invented kinds ("default", "always") and positions past the end.
func TestPrompt_namesTheBlockKindsAndPositions(t *testing.T) {
	t.Parallel()
	for _, locale := range []string{"pt-BR", "en-US"} {
		got, err := conversation.Prompt(locale, conversation.PromptInput{Projection: "{}", Message: "Oi"})
		if err != nil {
			t.Fatalf("%s: Prompt: %v", locale, err)
		}
		for _, k := range blocks.Kinds() {
			if !strings.Contains(got, `"`+string(k)+`"`) {
				t.Errorf("%s: the prompt does not name the block kind %s", locale, k)
			}
		}
		if !strings.Contains(got, `"at"`) {
			t.Errorf("%s: the prompt does not say where a block may go", locale)
		}
	}
}
