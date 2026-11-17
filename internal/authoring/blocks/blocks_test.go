package blocks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/authoring/blocks"
)

/*
The instructions as blocks, and back, without losing a byte (NT-018,
contract 2).

The draft keeps each block's raw fragment, so an untouched block is written
back exactly as it was read: opening an agent in the workbench and publishing
it unchanged keeps its version. Only a block somebody changed is written in
the canonical form.
*/

// examples are the instructions of every agent file in the repository, as
// spec.Parse normalises them, which is what a draft starts from.
func examples(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("../../../*/agents/*.agent.md")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	more, err := filepath.Glob("../../../internal/catalogue/templates/*/*.agent.md")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	root, err := filepath.Glob("../../../agents/*.agent.md")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	files = append(append(files, more...), root...)
	if len(files) < 19 {
		t.Fatalf("found %d agent files, want every example agent (19)", len(files))
	}
	out := map[string]string{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		out[file] = bodyOf(t, file, data)
	}
	return out
}

// bodyOf is the file's body as spec.Parse normalises it: everything after the
// frontmatter, outer space trimmed. Read here rather than through spec.Parse
// because the catalogue's templates are not complete definitions (no tools,
// no provider), and the codec only ever sees the body.
func bodyOf(t *testing.T, file string, data []byte) string {
	t.Helper()
	text := strings.TrimLeft(strings.TrimPrefix(string(data), "\ufeff"), " \t\r\n")
	rest, ok := strings.CutPrefix(text, "---")
	end := strings.Index(rest, "\n---")
	if !ok || end < 0 {
		t.Fatalf("%s has no frontmatter", file)
	}
	return strings.TrimSpace(rest[end+len("\n---"):])
}

// fixtures are the shapes an author actually writes.
var fixtures = map[string]string{
	"loose prose":             "Atenda o chamado.\n\nResponda com educação.",
	"prose then blocks":       "Contexto solto.\n\nObjetivo\nTriar.\n\nNunca\nPrometer prazo.",
	"repeated kinds":          "Nunca\nPrometer.\n\nComo agir\nLeia.\n\nNunca\nExpor dados.",
	"both languages":          "Purpose\nTriage tickets.\n\nComo agir\nLeia o histórico.\n\nWhen to stop\nAt once.",
	"capitalisation":          "OBJETIVO\nTriar.\n\nnever\nGuess.",
	"spaced labels":           "Contexto.\n  Objetivo  \nTriar.\n\n\n\nNunca   \nPrometer.",
	"crlf":                    "Objetivo\r\nTriar.\r\n\r\nNunca\r\nPrometer.",
	"an empty labelled block": "Objetivo\n\nNunca\nPrometer.",
	"no blank line":           "Objetivo\nTriar.\nNunca\nPrometer.",
	"a label-looking phrase":  "Nunca prometa prazo.\nObjetivo: triar.",
	"one line":                "Triar.",
}

func TestParseRender_everyExampleAndFixture_roundTripsByteForByte(t *testing.T) {
	t.Parallel()
	cases := examples(t)
	for name, text := range fixtures {
		cases["fixture: "+name] = text
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := blocks.Render(blocks.Parse(text)); got != text {
				t.Fatalf("round trip changed the bytes\nwant %q\n got %q", text, got)
			}
		})
	}
}

func TestParse_readsTheKindsTheConsoleReads(t *testing.T) {
	t.Parallel()
	got := blocks.Parse(fixtures["both languages"])
	want := []struct {
		kind blocks.Kind
		text string
	}{
		{blocks.Objective, "Triage tickets."},
		{blocks.HowToAct, "Leia o histórico."},
		{blocks.WhenToStop, "At once."},
	}
	if len(got) != len(want) {
		t.Fatalf("blocks = %+v, want %d", got, len(want))
	}
	for i, w := range want {
		if got[i].Kind != w.kind || got[i].Text != w.text {
			t.Errorf("block %d = %s %q, want %s %q", i, got[i].Kind, got[i].Text, w.kind, w.text)
		}
	}
}

// Ids are stable: the same instructions give the same ids, one per block.
func TestParse_givesEachBlockAStableDistinctId(t *testing.T) {
	t.Parallel()
	a, b := blocks.Parse(fixtures["repeated kinds"]), blocks.Parse(fixtures["repeated kinds"])
	seen := map[string]bool{}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].ID == "" || seen[a[i].ID] {
			t.Fatalf("ids %v and %v, want the same distinct ids", a, b)
		}
		seen[a[i].ID] = true
	}
}

// A changed block is written in the canonical form, with the label in the
// locale of whoever changed it; its neighbours keep their bytes.
func TestReplace_writesTheChangedBlockCanonicallyAndLeavesTheOthers(t *testing.T) {
	t.Parallel()
	doc := blocks.Parse("Contexto.\n  Objetivo  \nTriar.\n\n\n\nNunca   \nPrometer.")

	changed, err := blocks.Replace(doc, doc[2].ID, "Prometer prazo ou valor.", "en-US")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	want := "Contexto.\n  Objetivo  \nTriar.\n\n\n\nNever\nPrometer prazo ou valor."
	if got := blocks.Render(changed); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if blocks.Render(doc) != "Contexto.\n  Objetivo  \nTriar.\n\n\n\nNunca   \nPrometer." {
		t.Error("Replace changed the document it was given")
	}
}

func TestInsertMoveRemove_keepUntouchedBytesAndSeparateBlocks(t *testing.T) {
	t.Parallel()
	doc := blocks.Parse("Objetivo\nTriar.\n\nNunca\nPrometer.")

	inserted, err := blocks.Insert(doc, 2, blocks.HowToReply, "Em português.", "pt-BR")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got, want := blocks.Render(inserted), "Objetivo\nTriar.\n\nNunca\nPrometer.\n\nComo responder\nEm português."; got != want {
		t.Fatalf("insert at the end: got %q, want %q", got, want)
	}
	if inserted[2].ID == doc[0].ID || inserted[2].ID == doc[1].ID {
		t.Errorf("the new block reused id %s", inserted[2].ID)
	}

	moved, err := blocks.Move(inserted, inserted[0].ID, 2)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if got, want := blocks.Render(moved), "Nunca\nPrometer.\n\nComo responder\nEm português.\n\nObjetivo\nTriar."; got != want {
		t.Fatalf("move: got %q, want %q", got, want)
	}

	removed, err := blocks.Remove(moved, moved[1].ID)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got, want := blocks.Render(removed), "Nunca\nPrometer.\n\nObjetivo\nTriar."; got != want {
		t.Fatalf("remove: got %q, want %q", got, want)
	}
}

// Rendering goes through spec.Parse unchanged: what the codec writes is what
// the published version holds.
func TestRender_afterAnEdit_isWhatPublicationKeeps(t *testing.T) {
	t.Parallel()
	doc := blocks.Parse("Objetivo\nTriar.\n\nNunca\nPrometer.")
	edited, err := blocks.Replace(doc, doc[0].ID, "Triar e responder.", "pt-BR")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	text := blocks.Render(edited)
	if strings.TrimSpace(text) != text {
		t.Fatalf("render %q carries outer space the parser would trim", text)
	}
}

func TestOperations_unknownIdOrPosition_areRefused(t *testing.T) {
	t.Parallel()
	doc := blocks.Parse("Objetivo\nTriar.")
	if _, err := blocks.Replace(doc, "b99", "x", "pt-BR"); err == nil {
		t.Error("Replace of an unknown id succeeded")
	}
	if _, err := blocks.Remove(doc, "b99"); err == nil {
		t.Error("Remove of an unknown id succeeded")
	}
	if _, err := blocks.Move(doc, doc[0].ID, 5); err == nil {
		t.Error("Move past the end succeeded")
	}
	if _, err := blocks.Insert(doc, -1, blocks.Never, "x", "pt-BR"); err == nil {
		t.Error("Insert before the start succeeded")
	}
	if _, err := blocks.Insert(doc, 0, blocks.Kind("policy"), "x", "pt-BR"); err == nil {
		t.Error("Insert of an unknown kind succeeded")
	}
}

// A label is recognised whatever its case, the space around it and the line
// ending: a round trip alone would pass with these read as prose.
func TestParse_recognisesALabelWhateverItsCaseSpacingOrLineEnding(t *testing.T) {
	t.Parallel()
	for name, want := range map[string][]blocks.Kind{
		"capitalisation": {blocks.Objective, blocks.Never},
		"spaced labels":  {blocks.Prose, blocks.Objective, blocks.Never},
		"crlf":           {blocks.Objective, blocks.Never},
	} {
		got := blocks.Parse(fixtures[name])
		kinds := make([]blocks.Kind, len(got))
		for i, b := range got {
			kinds[i] = b.Kind
		}
		if strings.Join(asStrings(kinds), ",") != strings.Join(asStrings(want), ",") {
			t.Errorf("%s: kinds %v, want %v", name, kinds, want)
		}
	}
}

func asStrings(kinds []blocks.Kind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// Replacing a block with the text it already has keeps its bytes, label and
// spacing included: a save with nothing changed changes no version.
func TestReplace_theSameText_keepsTheBlocksBytes(t *testing.T) {
	t.Parallel()
	text := fixtures["spaced labels"]
	doc := blocks.Parse(text)

	same, err := blocks.Replace(doc, doc[1].ID, "  Triar.  ", "en-US")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if got := blocks.Render(same); got != text {
		t.Fatalf("got %q, want the untouched %q", got, text)
	}
}

// A block added with nothing in it yet contributes nothing: a label with no
// text under it is a heading the model reads and cannot act on.
func TestInsert_anEmptyBlock_writesNoLoneHeading(t *testing.T) {
	t.Parallel()
	doc := blocks.Parse("Objetivo\nTriar.")

	added, err := blocks.Insert(doc, 1, blocks.Never, "   ", "pt-BR")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := blocks.Render(added); got != "Objetivo\nTriar." {
		t.Fatalf("got %q, want the empty block left out", got)
	}
}

/*
Parse is linear in its input. A draft holds up to 2 MiB, and an instruction of
many short lines must not cost the replica memory quadratic in its length: an
early Parse copied the whole fragment again for every line, and 32 KiB of
short lines allocated 283 MiB.

Not parallel, so the measurement counts this parse alone.
*/
func TestParse_manyShortLines_allocatesLinearlyInTheInput(t *testing.T) {
	text := "Objetivo\n" + strings.Repeat("x\n", 128<<10) + "Nunca\nPrometer."
	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			blocks.Parse(text)
		}
	})
	if got, limit := result.AllocedBytesPerOp(), int64(4*len(text)); got > limit {
		t.Fatalf("Parse of %d bytes allocated %d bytes, want at most %d", len(text), got, limit)
	}
}
