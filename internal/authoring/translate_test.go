package authoring_test

import (
	"testing"

	"github.com/fuseone/agents/internal/authoring"
	"github.com/fuseone/agents/internal/domain"
)

// The model translates; it never grants. What comes back is read against the
// catalogue, and anything that is not in it does not exist.

var catalogue = []domain.ToolID{"crm.lookup", "kb.search", "crm.reply"}

func TestRead_toolsTheCatalogueDoesNotHave_areDropped(t *testing.T) {
	t.Parallel()

	got, err := authoring.Read([]byte(`{
	  "tools": ["crm.lookup", "crm.delete_everything"],
	  "steps": [{"name": "Identificar", "reaches": ["crm.lookup"]}]
	}`), catalogue)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// A model that names a tool nobody connected has invented a capability.
	// Trusting it would make the interview a way to widen an agent's reach by
	// describing a process persuasively.
	if len(got.Tools) != 1 || got.Tools[0] != "crm.lookup" {
		t.Errorf("got %v", got.Tools)
	}
}

func TestRead_aStepReachingAnUnknownTool_losesTheTool_notTheStep(t *testing.T) {
	t.Parallel()

	got, err := authoring.Read([]byte(`{
	  "tools": ["crm.lookup"],
	  "steps": [{"name": "Resumir", "reaches": ["magic.summarise"]}]
	}`), catalogue)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// The step survives: a stage that reaches nothing is the agent thinking,
	// which is a real shape. Dropping the step instead would silently discard
	// something the author described.
	if len(got.Steps) != 1 || got.Steps[0].Name != "Resumir" || len(got.Steps[0].Reaches) != 0 {
		t.Errorf("got %+v", got.Steps)
	}
}

func TestRead_repliesThatAreNotJSON_areRefusedRatherThanGuessedAt(t *testing.T) {
	t.Parallel()

	if _, err := authoring.Read([]byte("Claro! Aqui está o agente:"), catalogue); err == nil {
		t.Error("want a refusal")
	}
}

func TestRead_prosePaddedAroundTheJSON_isStillRead(t *testing.T) {
	t.Parallel()

	// Models pad. Refusing the whole answer over a courteous sentence would
	// spend the call and throw it away.
	got, err := authoring.Read([]byte("Segue:\n```json\n{\"tools\":[\"kb.search\"]}\n```\n"), catalogue)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "kb.search" {
		t.Errorf("got %v", got.Tools)
	}
}
