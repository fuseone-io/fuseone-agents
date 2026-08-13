package spec_test

import (
	"testing"
	"testing/fstest"

	"github.com/fuseone/agents/internal/spec"
)

/*
A directory of definitions is a way to seed an installation, not the source.

Publishing is an interface action (PRD DE-07), so the ordinary installation
authors through the console and never has a directory at all. A platform that
refused to start without one would be refusing to start in its normal shape —
which is exactly what it did on the first clean install: the worker read its
default path, found nothing there, and exited fatal in a crash loop while the
API served happily beside it.
*/
func TestLoadDir_directoryDoesNotExist_loadsNothingAndSaysSo(t *testing.T) {
	t.Parallel()
	store := spec.NewStore()

	loaded, err := store.LoadDir(t.Context(), fstest.MapFS{}, "agents")
	if err != nil {
		t.Fatalf("an absent seed directory is not an error, got %v", err)
	}
	if loaded != 0 {
		t.Errorf("loaded = %d from a directory that does not exist", loaded)
	}
	if agents := store.Agents(); len(agents) != 0 {
		t.Errorf("agents = %v from a directory that does not exist", agents)
	}
}

// A directory that exists and cannot be read is a different fact: somebody
// mounted something and it is wrong. That still has to be loud.
func TestLoadDir_definitionIsMalformed_refuses(t *testing.T) {
	t.Parallel()
	store := spec.NewStore()

	fsys := fstest.MapFS{"agents/broken.agent.md": {Data: []byte("not a definition")}}
	if _, err := store.LoadDir(t.Context(), fsys, "agents"); err == nil {
		t.Fatal("a malformed definition loaded without complaint")
	}
}
