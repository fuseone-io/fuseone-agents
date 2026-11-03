package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden")

const clock = "2026-10-22T12:00:00Z"

type outcome struct {
	code           int
	stdout, stderr string
}

// runCmd runs the command against the fake, with the bot map doc (none: the
// shipped one) and a fixed clock.
func runCmd(t *testing.T, f *fakeGitHub, botsDoc string, args ...string) outcome {
	t.Helper()
	env := f.env()
	if botsDoc != "" {
		path := filepath.Join(t.TempDir(), "bots.yaml")
		if err := os.WriteFile(path, []byte(botsDoc), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--bots", path)
	}
	var out, errOut bytes.Buffer
	d := deps{getenv: func(k string) string { return env[k] }, stdout: &out, stderr: &errOut,
		now: func() time.Time { return at(t, clock) }, http: f.srv.Client()}
	code := run(context.Background(), args, d)
	return outcome{code, out.String(), errOut.String()}
}

func reportArgs(extra ...string) []string {
	return append([]string{"report", "--since", "2026-10-07", "--weeks", "3",
		"--repo", fakeOwner + "/" + fakeName, "--project", "2"}, extra...)
}

func TestReport_placeholdersInTheBotMap_refusedBeforeAnyRequest(t *testing.T) {
	t.Parallel()
	for name, doc := range map[string]string{
		"a placeholder":                  strings.Replace(validBots, "d-app[bot]", "PLACEHOLDER-developer-app-slug[bot]", 1),
		"the shipped map, which has two": "",
	} {
		f := newFakeGitHub(t)
		o := runCmd(t, f, doc, reportArgs()...)
		if o.code != exitUsage || !strings.Contains(o.stderr, "developer") || o.stdout != "" {
			t.Errorf("%s: exit %d, stderr %q, stdout %q; want a refusal naming the developer", name, o.code, o.stderr, o.stdout)
		}
		if len(f.requests) != 0 {
			t.Errorf("%s: %d requests reached GitHub before the refusal", name, len(f.requests))
		}
	}
}

func TestReport_noToken_refused(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	var out, errOut bytes.Buffer
	d := deps{getenv: func(string) string { return "" }, stdout: &out, stderr: &errOut,
		now: func() time.Time { return at(t, clock) }}
	if code := run(context.Background(), append(reportArgs(), "--bots", writeBots(t)), d); code != exitUsage ||
		!strings.Contains(errOut.String(), "GITHUB_TOKEN") {
		t.Fatalf("exit %d, stderr %q; want a refusal naming GITHUB_TOKEN", code, errOut.String())
	}
	if len(f.requests) != 0 {
		t.Fatal("a request went out without a token")
	}
}

func writeBots(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bots.yaml")
	if err := os.WriteFile(path, []byte(validBots), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// goldenBoard is a small pilot: an issue the Architect blocked, a person
// returned, and the Architect then prepared; its Developer pull request
// through review, a QA failure, a person's commit and a pass; a card Triage
// passed and a person sent back; and an App the map does not know.
func goldenBoard(f *fakeGitHub) {
	f.add(1, &fakeThing{onBoard: true, linked: []int{2}, nodes: []map[string]any{
		nStatus("2026-10-05T08:00:00Z", "", "Triagem", person),
		nLabel("2026-10-05T10:00:00Z", true, labelNeedsDesign, triBot),
		nComment("2026-10-06T09:00:00Z", "Which tenant owns this?\n\nReturn to: Triagem\n", archBot),
		nStatus("2026-10-06T09:00:01Z", "Triagem", "Travado", archBot),
		nStatus("2026-10-07T09:00:00Z", "Travado", "Triagem", person),
		nLabel("2026-10-07T12:00:00Z", false, labelNeedsDesign, archBot),
		nStatus("2026-10-07T13:00:00Z", "Triagem", "Preparado", archBot),
		nStatus("2026-10-08T09:00:00Z", "Preparado", "Doing", person),
	}})
	f.add(2, &fakeThing{isPR: true, updatedAt: time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC),
		author: ghActor(devBot).(map[string]any), linked: []int{1}, nodes: []map[string]any{
			nCommit("2026-10-08T11:00:00Z", shaA, devBot),
			nLabel("2026-10-08T12:00:00Z", true, labelNeedsReview, devBot),
			nReview("2026-10-09T10:00:00Z", "CHANGES_REQUESTED", shaA, revBot),
			nLabel("2026-10-09T10:00:00Z", false, labelNeedsReview, revBot),
			nLabel("2026-10-09T10:00:00Z", true, labelChangesRequested, revBot),
			nCommit("2026-10-09T15:00:00Z", shaB, devBot),
			nLabel("2026-10-09T15:00:00Z", false, labelChangesRequested, devBot),
			nLabel("2026-10-09T15:00:00Z", true, labelNeedsReview, devBot),
			nReview("2026-10-12T10:00:00Z", "APPROVED", shaB, revBot),
			nLabel("2026-10-12T10:00:00Z", true, labelNeedsQA, revBot),
			nLabel("2026-10-13T09:00:00Z", true, labelChangesRequested, qaBot),
			nCommit("2026-10-14T10:00:00Z", shaC, person),
			nLabel("2026-10-15T10:00:00Z", true, labelReadyToMerge, qaBot),
			nReview("2026-10-20T10:00:00Z", "CHANGES_REQUESTED", shaC, person),
		}})
	f.statuses[shaB] = []map[string]any{rStatus("2026-10-13T09:00:00Z", "failure", qaContext, qaBot)}
	f.statuses[shaC] = []map[string]any{rStatus("2026-10-15T10:00:00Z", "success", qaContext, qaBot)}
	f.add(3, &fakeThing{onBoard: true, nodes: []map[string]any{
		nStatus("2026-10-12T08:00:00Z", "", "Triagem", person),
		nComment("2026-10-13T08:00:00Z", "Build passed", strayBot),
		nStatus("2026-10-12T14:00:00Z", "Triagem", "Preparado", triBot),
		nStatus("2026-10-19T09:00:00Z", "Preparado", "Triagem", person),
	}})
}

func TestReport_markdown_matchesTheGolden(t *testing.T) {
	t.Parallel()
	f := newFakeGitHub(t)
	goldenBoard(f)
	o := runCmd(t, f, validBots, reportArgs()...)
	if o.code != exitOK {
		t.Fatalf("exit %d: %s", o.code, o.stderr)
	}
	golden := filepath.Join("testdata", "report.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(o.stdout), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if o.stdout != string(want) {
		t.Fatalf("report differs from %s:\n%s", golden, o.stdout)
	}
	again := runCmd(t, f, validBots, reportArgs()...)
	if again.stdout != o.stdout {
		t.Fatal("two runs over the same board printed different reports")
	}
}
