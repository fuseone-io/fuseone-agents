/*
Package arch holds no code. It holds the accuser for the layering rule.

CLAUDE.md states two architectural invariants in prose: internal/domain does no
I/O and knows no infrastructure, and dependencies point inward — httpapi →
engine → ledger → domain, never the reverse. Prose is not an accuser. A rule
nothing checks is a rule that holds until the first afternoon somebody is in a
hurry, and the diagram that draws it is wrong from then on with nothing saying
so.

This walks the import graph with go/parser rather than shelling out to the
toolchain: stdlib only, no build, and fast enough to run on every change.

The two rules read different files, and the difference is the point. Purity
covers tests as well: a domain test that reaches for a driver has put the
driver in the package's world. Direction covers production only, because a
ledger test importing engine is the consumer-declared interface being proved —
the ledger asserting at compile time that it satisfies engine.ContentStore.
Forbidding that would forbid the check that makes the layering real.
*/
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/fuseone/agents/"

// layers are ordered from the inside out. A package may import its own layer
// and anything to its left; an import to the right is the reverse arrow the
// rule forbids.
var layers = []string{"domain", "ledger", "engine", "httpapi"}

/*
domainAllowed is the one non-stdlib import internal/domain may hold.

Unicode normalisation is a table the stdlib does not ship, and memory identity
is wrong without it — two spellings of one fact become two facts. The exception
is named here rather than left to judgement so that adding a second one is a
decision somebody makes in this file, in review, instead of an import that
slips in unnoticed.
*/
var domainAllowed = map[string]bool{
	"golang.org/x/text/unicode/norm": true,
}

func TestDomain_importsNothingButTheStandardLibraryAndItsNamedException(t *testing.T) {
	t.Parallel()

	for pkg, imports := range importGraph(t, true) {
		if !inLayer(pkg, "domain") {
			continue
		}
		for _, imported := range imports {
			switch {
			case external(imported) && !domainAllowed[imported]:
				t.Errorf("%s imports %s; domain is pure, and the exception list is in this file",
					pkg, imported)
			// Its own package, read back by an external test package, is not
			// infrastructure. Any other package of ours is: domain is what the
			// rest is built on, so it can be built on nothing of ours.
			case ours(imported) && !inLayer(strings.TrimPrefix(imported, module), "domain"):
				t.Errorf("%s imports %s; domain knows no other package of ours", pkg, imported)
			}
		}
	}
}

func TestLayers_dependenciesPointInwardOnly(t *testing.T) {
	t.Parallel()

	for pkg, imports := range importGraph(t, false) {
		from, ok := layerOf(pkg)
		if !ok {
			continue
		}
		for _, imported := range imports {
			to, ok := layerOf(strings.TrimPrefix(imported, module))
			if !ok || to <= from {
				continue
			}
			t.Errorf("%s imports %s: %s may not depend on %s",
				pkg, imported, layers[from], layers[to])
		}
	}
}

// importGraph maps every package path under internal/ and cmd/ to what it
// imports, test files included.
func importGraph(t *testing.T, includeTests bool) map[string][]string {
	t.Helper()
	graph := map[string][]string{}
	fset := token.NewFileSet()

	for _, root := range []string{"../../internal", "../../cmd"} {
		err := fs.WalkDir(os.DirFS(root), ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if !includeTests && strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, filepath.Join(root, path), nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			pkg := filepath.ToSlash(filepath.Join(filepath.Base(root), filepath.Dir(path)))
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				graph[pkg] = append(graph[pkg], path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(graph) == 0 {
		t.Fatal("the import graph is empty; this test would pass on anything")
	}
	return graph
}

// external reports whether an import is third-party: outside the standard
// library and outside this module. The stdlib has no dot in its first path
// element; every module path does.
func external(path string) bool {
	if ours(path) {
		return false
	}
	first, _, _ := strings.Cut(path, "/")
	return strings.Contains(first, ".")
}

func ours(path string) bool { return strings.HasPrefix(path, module) }

func layerOf(pkg string) (int, bool) {
	for i, layer := range layers {
		if inLayer(pkg, layer) {
			return i, true
		}
	}
	return 0, false
}

// inLayer matches the layer's own package and anything nested under it, so a
// subpackage cannot hold an edge its parent is forbidden.
func inLayer(pkg, layer string) bool {
	prefix := "internal/" + layer
	return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
}
