package draft_test

import (
	"testing"

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
