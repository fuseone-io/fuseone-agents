package conversation_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/conversation"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
)

type offeredProvider struct {
	Name       string   `json:"name"`
	Models     []string `json:"models"`
	MoreModels int      `json:"moreModels"`
}

func providersIn(t *testing.T, offer conversation.Offer) []offeredProvider {
	t.Helper()
	d := draft.New("triage", "Triagem", domain.Scope{Company: "acme", Area: "cx"})
	d.Blocks = blocks.Parse("Objetivo\nTriar.")
	got, err := conversation.Project(d, nil, offer)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	var out struct {
		Providers []offeredProvider `json:"providersOffered"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.Providers
}

/*
The model is shown the models each offered provider serves, as the installation
configured them, so a set_model proposal names one instead of asking the person or
guessing (#414). A gateway's model names are the installation's own: nothing else can
tell the model what they are.
*/
func TestProject_eachProviderOffered_carriesItsModels(t *testing.T) {
	t.Parallel()
	got := providersIn(t, conversation.Offer{Providers: []conversation.ProviderOffer{
		{Name: "gateway", Models: []string{"claude-sonnet", "gpt-mini"}},
		{Name: "openai"},
	}})
	if len(got) != 2 || got[0].Name != "gateway" || len(got[0].Models) != 2 || got[0].Models[1] != "gpt-mini" {
		t.Fatalf("providers = %+v, want the gateway with its two models", got)
	}
	if got[1].Name != "openai" || len(got[1].Models) != 0 || got[1].MoreModels != 0 {
		t.Errorf("providers[1] = %+v, want openai with no models listed", got[1])
	}
}

// Bounded per provider, and the bound is said: the projection names how many models
// it left out, so the model knows the list is not the whole of what the provider serves.
func TestProject_aProviderWithMoreModelsThanTheBound_saysHowManyWereLeftOut(t *testing.T) {
	t.Parallel()
	models := make([]string, conversation.MaxModelsPerProvider+3)
	for i := range models {
		models[i] = fmt.Sprintf("model-%03d", i)
	}
	got := providersIn(t, conversation.Offer{Providers: []conversation.ProviderOffer{{Name: "gateway", Models: models}}})
	if len(got[0].Models) != conversation.MaxModelsPerProvider || got[0].MoreModels != 3 {
		t.Errorf("gateway = %d models, %d more, want %d and 3", len(got[0].Models), got[0].MoreModels, conversation.MaxModelsPerProvider)
	}
}

// The prompt tells the model, in both locales, to choose a model among those listed for
// its provider, and to ask only when a provider lists none.
func TestPrompt_asksForAModelAmongThoseOffered(t *testing.T) {
	t.Parallel()
	for locale, want := range map[string]string{
		"pt-BR": `um dos seus "models"`,
		"en-US": `one of its "models"`,
	} {
		got, err := conversation.Prompt(locale, conversation.PromptInput{Projection: "{}", Message: "Oi"})
		if err != nil || !strings.Contains(got, want) || !strings.Contains(got, `"providersOffered"`) {
			t.Errorf("%s: prompt does not ask for an offered model (%v)", locale, err)
		}
	}
}
