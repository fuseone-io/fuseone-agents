package draft_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
	"github.com/fuseone/agents/internal/authoring/draft"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/spec"
)

// A draft is stored as the publication format's frontmatter plus its blocks,
// and read back exactly, block ids and raw bytes included.
func TestEncodeDecode_aDraftReadsBackExactly(t *testing.T) {
	t.Parallel()
	d := complete(t)
	edited, err := apply(t, d, draft.ReplaceBlock{ID: d.Blocks[1].ID, Text: "Prometer prazo.", Locale: "en-US"}, draft.FromProposal)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	stored, err := draft.Encode(edited.Draft)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	back, err := draft.Decode(stored)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	a, _ := spec.Render(edited.Draft.Definition())
	b, _ := spec.Render(back.Definition())
	if string(a) != string(b) {
		t.Fatalf("the definition changed on the way\n%s\n%s", a, b)
	}
	for i := range edited.Draft.Blocks {
		if back.Blocks[i] != edited.Draft.Blocks[i] {
			t.Errorf("block %d = %+v, want %+v", i, back.Blocks[i], edited.Draft.Blocks[i])
		}
	}
}

// A draft with only its identity stores and reads back too: the store holds
// drafts long before they are publishable.
func TestEncodeDecode_aDraftWithOnlyItsIdentity_readsBack(t *testing.T) {
	t.Parallel()
	stored, err := draft.Encode(draft.New("triage", "Triagem", domain.Scope{Company: "acme", Area: "cx"}))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	back, err := draft.Decode(stored)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if back.Spec.ID != "triage" || back.Spec.Company != "acme" || back.Spec.Area != "cx" || len(back.Blocks) != 0 {
		t.Fatalf("read back %+v", back)
	}
}

// The trail keeps a digest of each operation: the same operation, the same
// digest; any change to it, another.
func TestDigest_namesTheOperationAndItsPayload(t *testing.T) {
	t.Parallel()
	a := draft.Digest(draft.AddTool{Tool: "crm.lookup"})
	if a != draft.Digest(draft.AddTool{Tool: "crm.lookup"}) {
		t.Fatal("the same operation gave two digests")
	}
	for name, other := range map[string]draft.Operation{
		"another payload": draft.AddTool{Tool: "crm.reply"},
		"another kind":    draft.RemoveTool{Tool: "crm.lookup"},
	} {
		if draft.Digest(other) == a {
			t.Errorf("%s gave the same digest", name)
		}
	}
	if len(a) != 64 {
		t.Errorf("digest %q is not a hex SHA-256", a)
	}
}

// A proposal's operation is read back from the body the content store
// keeps: the same operation, digest for digest, for every kind there is.
func TestParseBody_readsBackEveryOperationExactly(t *testing.T) {
	t.Parallel()
	examples := []draft.Operation{
		draft.SetIdentity{Name: "T", Scope: domain.Scope{Company: "acme", Area: "ops"}},
		draft.SetStandingApprovals{Policy: domain.StandingForbidden},
		draft.SetModel{Provider: "openai", Model: "m", Effort: "high"},
		draft.SetFallback{Pair: domain.ModelPair{Provider: "p", Model: "m"}},
		draft.SetCachePrefixTTL{TTL: "1h"}, draft.SetReplyLocale{Locale: "en-US"},
		draft.AddTool{Tool: "crm.lookup"}, draft.RemoveTool{Tool: "crm.reply"},
		draft.SetBudget{Budget: domain.Budget{Micros: 1, Steps: 2}},
		draft.SetTriggers{Triggers: []spec.Trigger{{Type: "webhook", Path: "gh", From: "github", Events: []string{"issues"}}}},
		draft.SetMemoryLearning{Policy: domain.MemoryLearningPolicy{Mode: domain.MemoryLearningOff}},
		draft.SetApprovalDelivery{Policy: domain.ApprovalPolicy{Direct: true, Notify: []domain.UserID{"usr_bia"}}},
		draft.SetEmits{Emits: spec.Emits{{Event: "ticket.triaged", Artifacts: []string{"summary"}}}},
		draft.AddStep{Step: spec.Step{Name: "Ler", Reaches: []domain.ToolID{"crm.lookup"}}},
		draft.EditStep{Name: "Ler", Step: spec.Step{Name: "Ler bem"}}, draft.RemoveStep{Name: "Ler"},
		draft.InsertBlock{At: 1, Type: blocks.Never, Text: "Prometer.", Locale: "pt-BR"},
		draft.ReplaceBlock{ID: "b1", Text: "Triar.", Locale: "en-US"},
		draft.EditBlock{ID: "b1", Old: "Triar.", New: "Triar bem."},
		draft.RemoveBlock{ID: "b2"}, draft.MoveBlock{ID: "b1", To: 0},
	}
	if len(examples) != len(draft.Operations()) {
		t.Fatalf("%d examples for %d operations; every kind must be read back", len(examples), len(draft.Operations()))
	}
	for _, op := range examples {
		back, err := draft.ParseBody(draft.Body(op))
		if err != nil {
			t.Fatalf("%s: %v", op.Kind(), err)
		}
		// The same type, by value: a pointer would serialise the same and
		// still be another operation to every type switch downstream.
		if reflect.TypeOf(back) != reflect.TypeOf(op) || draft.Digest(back) != draft.Digest(op) {
			t.Errorf("%s read back as %#v", op.Kind(), back)
		}
	}
}

func TestParseBody_somethingNotAnOperation_isRefused(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"an unknown kind": `{"kind":"grant_admin","operation":{}}`,
		"not json":        `kind=add_tool`,
		"no kind":         `{"operation":{"Tool":"x"}}`,
	} {
		if _, err := draft.ParseBody([]byte(body)); !errors.Is(err, draft.ErrInvalidOperation) {
			t.Errorf("%s: ParseBody = %v, want ErrInvalidOperation", name, err)
		}
	}
}
