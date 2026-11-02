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

// newReleaseFixture adds to the fixture what `make release` reads: the
// repository's real Makefile and guard, and a changelog that already has the
// section for the version under release. All three are committed and pushed,
// so a clean tree at origin/main is the starting point.
func newReleaseFixture(t *testing.T, version string) *fixture {
	t.Helper()
	f := newFixture(t)
	f.copyIn("../Makefile", "Makefile")
	f.copyIn("release-guard.sh", "scripts/release-guard.sh")
	f.write("CHANGELOG.md", "# Changelog\n\n## ["+version+"]\n")
	f.git(f.work, "add", "Makefile", "scripts/release-guard.sh", "CHANGELOG.md")
	f.git(f.work, "commit", "--quiet", "-m", "release inputs")
	f.git(f.work, "push", "--quiet", "origin", "main")
	f.git(f.work, "fetch", "--quiet", "origin")
	return f
}

func (f *fixture) copyIn(src, dst string) {
	f.t.Helper()
	body, err := os.ReadFile(src)
	if err != nil {
		f.t.Fatal(err)
	}
	path := filepath.Join(f.work, dst)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o700); err != nil {
		f.t.Fatal(err)
	}
}

// makeRelease runs the real `release` target. GO=true keeps the Makefile's
// package listing from invoking Go in a repository that has no module.
// check replaces $(MAKE) for the recursive `make check`: "true" passes it
// instantly, "false" fails it, which proves a refusal came before the check.
func (f *fixture) makeRelease(version, check string) (string, error) {
	f.t.Helper()
	cmd := exec.Command("make", "--no-print-directory", "release",
		"V="+version, "GO=true", "MAKE="+check)
	cmd.Dir = f.work
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (f *fixture) tags(dir string) string {
	f.t.Helper()
	return strings.TrimSpace(f.git(dir, "tag", "--list"))
}

func TestMakeRelease_refusedBeforeAnyTag(t *testing.T) {
	t.Parallel()
	const version = "9.9.9"
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
			name:    "HEAD ahead of origin/main",
			arrange: func(f *fixture) { f.commit("docs-changelog-cut") },
			reason:  "origin/main",
		},
	}
	// With a passing check, nothing but the guard stands between the target
	// and the tag. With a failing one, a refusal that names the reason proves
	// the guard ran before the check.
	checks := []string{"true", "false"}
	for _, tc := range cases {
		for _, check := range checks {
			t.Run(tc.name+"/check="+check, func(t *testing.T) {
				t.Parallel()
				f := newReleaseFixture(t, version)
				tc.arrange(f)
				out, err := f.makeRelease(version, check)
				if err == nil {
					t.Fatalf("make release accepted; want refusal naming %q\n%s", tc.reason, out)
				}
				if !strings.Contains(out, tc.reason) {
					t.Fatalf("make release refused without naming %q:\n%s", tc.reason, out)
				}
				if local, remote := f.tags(f.work), f.tags(f.origin); local != "" || remote != "" {
					t.Fatalf("refused release left tags: local %q, origin %q", local, remote)
				}
			})
		}
	}
}

// The anchor for the refusals above: with the guard satisfied, the same
// invocation reaches the tag and pushes it, so their refusal is the guard's.
func TestMakeRelease_cleanHeadAtOriginMain_tagsAndPushes(t *testing.T) {
	t.Parallel()
	f := newReleaseFixture(t, "9.9.9")
	if out, err := f.makeRelease("9.9.9", "true"); err != nil {
		t.Fatalf("make release refused a clean HEAD at origin/main: %v\n%s", err, out)
	}
	if got := f.tags(f.origin); got != "v9.9.9" {
		t.Fatalf("origin tags = %q, want v9.9.9", got)
	}
}
