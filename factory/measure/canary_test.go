package main

import (
	"strings"
	"testing"
)

func canaryArgs(extra ...string) []string {
	return append([]string{"canary", "--repo", fakeOwner + "/" + fakeName, "--project", "2", "--issue", "5"}, extra...)
}

func movedBy(who Actor) *fakeThing {
	return &fakeThing{onBoard: true, nodes: []map[string]any{
		nStatus("2026-10-05T08:00:00Z", "", "Triagem", person),
		nStatus("2026-10-05T09:00:00Z", "Triagem", "Preparado", who),
	}}
}

func TestCanary_appMoveReadAsItsBot_passes(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	f.add(5, movedBy(triBot))
	pr := f.add(6, &fakeThing{isPR: true, onBoard: true})
	pr.nodes = []map[string]any{nStatus("2026-10-05T08:00:00Z", "", "Doing", person)}
	o := runCmd(t, f, validBots, canaryArgs("--pr", "6")...)
	if o.code != exitOK || strings.Count(o.stdout, "PASS  ") != 3 || !strings.Contains(o.stdout, "Result: PASS.") {
		t.Fatalf("exit %d:\n%s%s", o.code, o.stdout, o.stderr)
	}
	if !strings.Contains(o.stdout, "moved by t-app[bot], the triage bot") {
		t.Errorf("the pass does not say which bot moved the card:\n%s", o.stdout)
	}
}

func TestCanary_failures_saySoAndExitNonZero(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(f *fakeGitHub)
		args  []string
		fail  string // the premise that fails
		says  string
	}{
		{"moved only by people", func(f *fakeGitHub) { f.add(5, movedBy(person)) }, nil,
			"FAIL  2. " + premiseBot, "User alice (person) x2"},
		{"moved by a bot the map lacks", func(f *fakeGitHub) { f.add(5, movedBy(strayBot)) }, nil,
			"FAIL  2. " + premiseBot, "Bot stray-app[bot] (unmapped) x1"},
		{"the token cannot read timelines", func(f *fakeGitHub) { f.add(5, movedBy(triBot)); f.failTypes = "FORBIDDEN" }, nil,
			"FAIL  1. " + premiseRead, "FORBIDDEN"},
		{"the token cannot read the project", func(f *fakeGitHub) { f.add(5, movedBy(triBot)); f.nullActors = true }, nil,
			"FAIL  1. " + premiseRead, "the token cannot read the project"},
		{"no move on the issue", func(f *fakeGitHub) { f.add(5, &fakeThing{onBoard: true}) }, nil,
			"FAIL  1. " + premiseRead, "no status change on #5"},
		{"a pull request card without the event", func(f *fakeGitHub) {
			f.add(5, movedBy(triBot))
			f.add(6, &fakeThing{isPR: true})
		}, []string{"--pr", "6"}, "FAIL  3. " + premisePR, "no status change on #6"},
	} {
		f := newFakeGitHub(t)
		tc.setup(f)
		o := runCmd(t, f, validBots, canaryArgs(tc.args...)...)
		if o.code != exitFail || !strings.Contains(o.stdout, tc.fail) || !strings.Contains(o.stdout, tc.says) ||
			!strings.Contains(o.stdout, "Do not rely on factory-measure report") {
			t.Errorf("%s: exit %d, want %q saying %q:\n%s%s", tc.name, o.code, tc.fail, tc.says, o.stdout, o.stderr)
		}
	}
}

func TestCanary_placeholders_runButAreNamed(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	f.add(5, movedBy(Actor{"Bot", "fuseone-factory-triage[bot]"}))
	o := runCmd(t, f, "", canaryArgs()...)
	if o.code != exitOK || !strings.Contains(o.stdout, "placeholders for architect, developer") {
		t.Fatalf("exit %d:\n%s%s", o.code, o.stdout, o.stderr)
	}
}

func TestCanary_neverWrites(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	f.add(5, movedBy(triBot))
	runCmd(t, f, validBots, canaryArgs()...)
	for _, r := range f.requests {
		if r.Method != "POST" || r.Path != "/graphql" || !strings.HasPrefix(strings.TrimSpace(r.Body), "query") {
			t.Errorf("the canary sent %s %s %.30q", r.Method, r.Path, r.Body)
		}
	}
}
