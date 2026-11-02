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
