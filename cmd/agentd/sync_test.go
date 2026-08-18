package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

/*
An agents directory mounted at an absolute path.

`io/fs` paths are unrooted by contract, so a leading slash is invalid rather
than absolute: `os.DirFS(".")` with `/agents` fails with "invalid argument" and
reads nothing. That is the only kind of path a Kubernetes volumeMount produces,
which means the specs ConfigMap this chart offers could never have worked —
the worker refused to start with it, in a lab, before anybody noticed the value
existed.
*/
func TestSpecsRoot_anAbsoluteDirectory_isReadableRatherThanInvalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.agent.md"), []byte("---\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	found, err := fs.Glob(specsRoot(dir), filepath.Join(specsPath(dir), "*.agent.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(found) != 1 {
		t.Errorf("found %v, want the definition in %s", found, dir)
	}
}

// And a relative one still resolves against the working directory, so running
// the worker inside a checkout goes on finding `agents/`.
func TestSpecsRoot_aRelativeDirectory_staysRelativeToTheWorkingDirectory(t *testing.T) {
	if got := specsPath("agents"); got != "agents" {
		t.Errorf("path = %q, want the directory as given", got)
	}
}
