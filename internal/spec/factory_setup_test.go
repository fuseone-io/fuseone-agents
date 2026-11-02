package spec_test

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The factory routes cards by labels, and GitHub applies only labels the
// repository already defines: a role that sets one that was never created
// fails at that write, and the card it meant to move stays where it was, to
// be picked again on the next run. So every factory:* label a role or a lane
// names has to be created by the setup a person follows, and these tests read
// the labels from the references and LANES.md rather than from a list kept
// here: a new label without its setup line fails, wherever it is introduced.

var factoryLabel = regexp.MustCompile(`factory:[a-z]+(?:-[a-z]+)*`)

// factorySetupTexts are the places a person setting the factory up reads the
// steps from.
var factorySetupTexts = []string{
	"../../factory/agents/README.md",
	"../../docs/manual/en-US/software-factory.md",
	"../../docs/manual/pt-BR/software-factory.md",
}

// factoryLabels is every factory:* label the role references and LANES.md
// name, sorted.
func factoryLabels(t *testing.T) []string {
	t.Helper()
	texts := []string{readFile(t, "../../factory/LANES.md")}
	for _, data := range factoryReferences(t) {
		texts = append(texts, string(data))
	}
	var labels []string
	for _, text := range texts {
		for _, label := range factoryLabel.FindAllString(text, -1) {
			if !slices.Contains(labels, label) {
				labels = append(labels, label)
			}
		}
	}
	slices.Sort(labels)
	return labels
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// The Developer's App holds Contents: write, which reaches main. What holds
// main in v1 is a minimal ruleset — a pull request required, force pushes and
// deletions blocked, nobody bypassing it — and it must exist before the
// Developer starts. It is not #207's ruleset: that one also requires checks
// that do not exist in v1 (factory/qa), so an operator told to install it
// cannot. The prerequisite is stated in one exact sentence per language, and
// the setup texts carry the same command to create it.
const (
	minimalRulesetEN = "Before the Developer is started, main must carry the minimal ruleset: a pull request required before merging, force pushes and deletions blocked, and no bypass for any factory GitHub App."
	minimalRulesetPT = "Antes de iniciar o Desenvolvedor, o main precisa ter o ruleset mínimo: pull request obrigatório antes do merge, force push e exclusão bloqueados, e nenhum bypass para os GitHub Apps da fábrica."
)

// minimalRulesetCommand is the command, line for line, every setup text
// gives for the minimal ruleset (POST /repos/{owner}/{repo}/rulesets).
var minimalRulesetCommand = []string{
	`gh api --method POST repos/<owner>/<name>/rulesets --input - <<'EOF'`,
	`  "name": "main: pull request required",`,
	`  "target": "branch",`,
	`  "enforcement": "active",`,
	`  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },`,
	`    { "type": "non_fast_forward" },`,
	`    { "type": "deletion" }`,
	`  "bypass_actors": []`,
}

// namesTheFullRuleset catches the wording the prerequisite must not use: #207
// is the deferred, full ruleset, never the one to install in v1.
var namesTheFullRuleset = regexp.MustCompile(`(?i)#207\s+ruleset|ruleset\s+(of|da|de|do)\s+(issue\s+)?#207`)

func TestFactorySetup_minimalRuleset_isAPrerequisiteInEveryText(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"../../factory/agents/developer.agent.md":     minimalRulesetEN,
		"../../factory/agents/README.md":              minimalRulesetEN,
		"../../docs/manual/en-US/software-factory.md": minimalRulesetEN,
		"../../docs/manual/pt-BR/software-factory.md": minimalRulesetPT,
	}
	for path, sentence := range want {
		lines := strings.Split(readFile(t, path), "\n")
		if count := countLines(lines, sentence); count != 1 {
			t.Errorf("%s carries the prerequisite line %d times, want 1", path, count)
		}
	}
	for _, path := range factorySetupTexts {
		lines := strings.Split(readFile(t, path), "\n")
		for _, line := range minimalRulesetCommand {
			if !slices.Contains(lines, line) {
				t.Errorf("%s lacks the minimal ruleset command line %q", path, line)
			}
		}
	}
}

func TestFactorySetup_prerequisite_neverCallsItThe207Ruleset(t *testing.T) {
	t.Parallel()

	paths := append([]string{
		"../../factory/agents/developer.agent.md",
		"../../docs/NT-013-software-factory-pilot.md",
	}, factorySetupTexts...)
	for _, path := range paths {
		if found := namesTheFullRuleset.FindString(readFile(t, path)); found != "" {
			t.Errorf("%s says %q; the v1 prerequisite is the minimal ruleset, and #207 is only the deferred part", path, found)
		}
	}
}

func TestFactorySetup_everyLabelTheRolesUse_isCreatedByTheSetup(t *testing.T) {
	t.Parallel()

	labels := factoryLabels(t)
	// The anchor: if the scan found nothing, the loop below would pass on
	// any setup text at all.
	if !slices.Contains(labels, "factory:needs-review") || !slices.Contains(labels, "factory:needs-runtime") {
		t.Fatalf("labels read from the references and LANES.md = %v; the scan is broken", labels)
	}
	for _, path := range factorySetupTexts {
		lines := strings.Split(readFile(t, path), "\n")
		for _, label := range labels {
			// A command a person can paste, on a line of its own.
			prefix := "gh label create " + label + " "
			if !slices.ContainsFunc(lines, func(line string) bool { return strings.HasPrefix(line, prefix) }) {
				t.Errorf("%s has no line starting %q", path, prefix)
			}
		}
	}
}
