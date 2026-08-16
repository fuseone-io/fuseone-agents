package tools_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/engine"
	"github.com/fuseone/agents/internal/tools"
)

/*
A ruling names the definition it judged, not only the name it was filed under.

`crm.lookup` is a string. What the Curator read when they said "this only
reads" was a description and a schema, and a server is free to change both
tomorrow while keeping the name. Keyed by name alone, yesterday's ruling
carries forward onto a tool nobody has looked at — which is the one path by
which an effect reaches production without anybody judging it.

Same shape as an approval carrying the step it approved: the decision has to
say what it was about, or it is a decision about whatever is there now.
*/

func serving(t *testing.T, description string, schema map[string]any) *tools.Catalog {
	t.Helper()
	catalog := tools.NewCatalog(engine.NewMemoryContent())
	server := &fakeServer{list: []*mcp.Tool{{
		Name: "lookup", Description: description,
		InputSchema: map[string]any{"type": "object", "properties": schema},
	}}}
	if err := catalog.AddServer(context.Background(), "crm", server); err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	return catalog
}

func digestOf(t *testing.T, catalog *tools.Catalog) string {
	t.Helper()
	for _, e := range catalog.List() {
		if e.ID == "crm.lookup" {
			return e.Digest
		}
	}
	t.Fatal("crm.lookup is not in the catalogue")
	return ""
}

// A ruling filed against the definition that is there is applied.
func TestSync_aRulingOnTheDefinitionOnOffer_isApplied(t *testing.T) {
	catalog := serving(t, "Look a customer up", map[string]any{"id": "string"})
	ruled := digestOf(t, catalog)

	applied, err := catalog.Sync(context.Background(),
		rulings{{Tool: "crm.lookup", Effect: domain.EffectRead, Digest: ruled}}, domain.Scope{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied %d, want the ruling for the tool it was made about", applied)
	}
	if effect, ok := catalog.Effect("crm.lookup"); !ok || effect != domain.EffectRead {
		t.Errorf("effect = %v (%v), want the recorded ruling", effect, ok)
	}
}

/*
The same name over a different definition does not inherit the ruling.

The description is what a Curator reads to decide, and a tool that now says it
"looks a customer up and closes their account" is not the tool that was
allowed. It stays unclassified, which the Gate already refuses.
*/
func TestSync_theSameToolRedefined_doesNotInheritTheOldRuling(t *testing.T) {
	judged := digestOf(t, serving(t, "Look a customer up", map[string]any{"id": "string"}))
	now := serving(t, "Look a customer up and close their account",
		map[string]any{"id": "string"})

	if _, err := now.Sync(context.Background(),
		rulings{{Tool: "crm.lookup", Effect: domain.EffectRead, Digest: judged}},
		domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if effect, _ := now.Effect("crm.lookup"); effect != domain.EffectUnknown {
		t.Errorf("effect = %v, want it refused until somebody looks again", effect)
	}
}

// A changed schema is a changed tool too. The description can stay honest
// while the arguments grow a `force` nobody ruled on.
func TestSync_theSameDescriptionOverANewSchema_doesNotInheritTheOldRuling(t *testing.T) {
	judged := digestOf(t, serving(t, "Look a customer up", map[string]any{"id": "string"}))
	now := serving(t, "Look a customer up",
		map[string]any{"id": "string", "andDelete": "boolean"})

	if _, err := now.Sync(context.Background(),
		rulings{{Tool: "crm.lookup", Effect: domain.EffectRead, Digest: judged}},
		domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if effect, _ := now.Effect("crm.lookup"); effect != domain.EffectUnknown {
		t.Errorf("effect = %v, want it refused until somebody looks again", effect)
	}
}

/*
Refused for a reason, and the reason is on the screen.

"Nobody has ever ruled on this" and "the tool changed under a ruling" are both
refusals and they are different work: the first is a decision to make, the
second is a decision to check. Shown as the same thing, the second looks like
somebody forgot.
*/
func TestSync_aRulingOvertakenByANewDefinition_saysItNeedsLookingAtAgain(t *testing.T) {
	judged := digestOf(t, serving(t, "Look a customer up", map[string]any{"id": "string"}))
	now := serving(t, "Look up and close", map[string]any{"id": "string"})

	if _, err := now.Sync(context.Background(),
		rulings{{Tool: "crm.lookup", Effect: domain.EffectRead, Digest: judged}},
		domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	for _, e := range now.List() {
		if e.ID == "crm.lookup" && !e.Stale {
			t.Error("not marked as overtaken; it reads as a tool nobody got to yet")
		}
	}
}

/*
A ruling recorded before digests existed still applies.

Every installation upgrading has rulings with no digest against tools that
have not changed. Refusing those would stop every agent on the platform to add
a check — the honest reading of an empty digest is "made before we recorded
what was judged", not "made about something else".
*/
func TestSync_aRulingFromBeforeDigestsExisted_stillApplies(t *testing.T) {
	catalog := serving(t, "Look a customer up", map[string]any{"id": "string"})

	if _, err := catalog.Sync(context.Background(),
		rulings{{Tool: "crm.lookup", Effect: domain.EffectRead}}, domain.Scope{}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if effect, _ := catalog.Effect("crm.lookup"); effect != domain.EffectRead {
		t.Errorf("effect = %v; an upgrade would have stopped every agent", effect)
	}
}

// rulings is a Classifier holding what the administration area recorded.
type rulings []domain.ToolClassification

func (r rulings) List(context.Context, domain.Scope) ([]domain.ToolClassification, error) {
	return r, nil
}
