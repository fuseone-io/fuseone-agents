// Package scripts holds tests for the shell scripts the Makefile calls.
//
// release-guard.sh decides whether `make release` may tag HEAD. Each case
// builds a throwaway repository with a local bare "origin", so nothing here
// fetches from, pushes to or tags the real remote.
package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is a working clone whose origin is a bare repository in the same
// temporary directory, with one commit on main pushed to it.
type fixture struct {
	t      *testing.T
	work   string
	origin string
	env    []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{
		t:      t,
		work:   filepath.Join(dir, "work"),
		origin: filepath.Join(dir, "origin.git"),
		env: append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.invalid",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.invalid",
		),
	}
	f.git(dir, "init", "--quiet", "--bare", "--initial-branch=main", f.origin)
	f.git(dir, "init", "--quiet", "--initial-branch=main", f.work)
	f.git(f.work, "remote", "add", "origin", f.origin)
	f.commit("first")
	f.git(f.work, "push", "--quiet", "origin", "main")
	f.git(f.work, "fetch", "--quiet", "origin")
	return f
}

func (f *fixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (f *fixture) write(name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.work, name), []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) commit(msg string) {
	f.t.Helper()
	f.write(msg+".txt", msg)
	f.git(f.work, "add", msg+".txt")
	f.git(f.work, "commit", "--quiet", "-m", msg)
}

// guard runs the real script in the fixture's working clone.
func (f *fixture) guard() (string, error) {
	f.t.Helper()
	script, err := filepath.Abs("release-guard.sh")
	if err != nil {
		f.t.Fatal(err)
	}
	cmd := exec.Command("bash", script)
	cmd.Dir = f.work
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestReleaseGuard_refusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		arrange func(f *fixture)
		reason  string
	}{
		{
			name:    "untracked file",
			arrange: func(f *fixture) { f.write("stray.txt", "x") },
			reason:  "dirty",
		},
		{
			name: "staged change",
			arrange: func(f *fixture) {
				f.write("first.txt", "changed")
				f.git(f.work, "add", "first.txt")
			},
			reason: "dirty",
		},
		{
			name:    "unstaged change",
			arrange: func(f *fixture) { f.write("first.txt", "changed") },
			reason:  "dirty",
		},
		{
			// A release branch with the changelog commit on it: squash-merged,
			// this commit never reaches main.
			name:    "HEAD ahead of origin/main",
			arrange: func(f *fixture) { f.commit("docs-changelog-cut") },
			reason:  "origin/main",
		},
		{
			// The local origin/main still equals HEAD; only a fetch shows
			// that main has moved on.
			name: "origin/main moved since the last fetch",
			arrange: func(f *fixture) {
				f.commit("unrelated")
				f.git(f.work, "push", "--quiet", "origin", "main")
				f.git(f.work, "reset", "--quiet", "--hard", "HEAD~1")
				f.git(f.work, "update-ref", "refs/remotes/origin/main", "HEAD")
			},
			reason: "origin/main",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			tc.arrange(f)
			out, err := f.guard()
			if err == nil {
				t.Fatalf("guard accepted; want refusal naming %q\n%s", tc.reason, out)
			}
			if !strings.Contains(out, tc.reason) {
				t.Fatalf("guard refused without naming %q:\n%s", tc.reason, out)
			}
		})
	}
}

func TestReleaseGuard_cleanHeadAtOriginMain_accepted(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if out, err := f.guard(); err != nil {
		t.Fatalf("guard refused a clean HEAD at origin/main: %v\n%s", err, out)
	}
}
