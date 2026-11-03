package main

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// checkWorkflow is the part of check.yml that decides which check runs a
// pull request gets: its triggers and each job's reported name.
type checkWorkflow struct {
	// A bare `pull_request:` decodes to a nil filter under a present key.
	On   map[string]*triggerFilter `yaml:"on"`
	Jobs map[string]struct {
		Name string `yaml:"name"`
		If   string `yaml:"if"`
	} `yaml:"jobs"`
}

type triggerFilter struct {
	Paths       []string `yaml:"paths"`
	PathsIgnore []string `yaml:"paths-ignore"`
	Branches    []string `yaml:"branches"`
}

// everyPullRequestChecks is the check-run name of every job in check.yml,
// which runs on every pull request: GitHub reports a job by its `name`, or
// by its id when it has none.
func everyPullRequestChecks(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../.github/workflows/check.yml")
	if err != nil {
		t.Fatalf("read check.yml: %v", err)
	}
	var wf checkWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse check.yml: %v", err)
	}
	pr, ok := wf.On["pull_request"]
	if !ok || (pr != nil && len(pr.Paths)+len(pr.PathsIgnore)+len(pr.Branches) > 0) {
		t.Fatalf("check.yml must run on every pull request, unfiltered; its triggers are %v", wf.On)
	}
	var names []string
	for id, job := range wf.Jobs {
		if job.If != "" {
			t.Fatalf("job %s runs only if %s: it is not a check every pull request gets", id, job.If)
		}
		name := job.Name
		if name == "" {
			name = id
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// The Merger refuses a head unless every check in FACTORY_CHECKS passed on
// it, so a job added to check.yml and left out of the documented list is a
// job the Merger would merge past red. The README's list is the one an
// operator copies into FACTORY_CHECKS and into the ruleset's required checks,
// so it must name every job a pull request runs, and nothing else.
func TestReadme_factoryChecksExample_namesEveryJobOfEveryPullRequest(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	row := regexp.MustCompile("(?m)^\\| `FACTORY_CHECKS` \\|.*required on the head, `([^`]+)`").FindSubmatch(raw)
	if row == nil {
		t.Fatal("README has no FACTORY_CHECKS row with an example list")
	}
	var documented []string
	for _, name := range strings.Split(string(row[1]), ",") {
		documented = append(documented, strings.TrimSpace(name))
	}
	slices.Sort(documented)

	if want := everyPullRequestChecks(t); !slices.Equal(documented, want) {
		t.Fatalf("README's FACTORY_CHECKS example = %v, check.yml's jobs = %v", documented, want)
	}
}
