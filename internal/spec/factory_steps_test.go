package spec_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/spec"
)

// A step's stops_when ends the whole run: the model is told to call the
// finish tool with stopped_by set, and nothing runs after it
// (internal/model/step_note.go). Every factory role answers its exception
// by writing — a question on the issue, a label, a card moved to Travado —
// so a stops_when on a step that a writing step follows lets the run end
// before the block is recorded, and the card stays where the next run picks
// it again. A stops_when may sit only where no later step reaches a write.

// factoryReadsRow is the row of the manual's classification table that a
// person setting the factory up rules "read, untrusted".
var factoryReadsRow = regexp.MustCompile("(?m)^\\| read, \\*\\*untrusted\\*\\* \\|(.*)\\|$")

var backticked = regexp.MustCompile("`([a-z_]+)`")

// factoryReadTools is every tool the manual classifies as a read. Anything
// else a reference reaches counts as a write: an unlisted or new tool fails
// closed, so no list kept here can fall behind a reference that gains a
// write.
func factoryReadTools(t *testing.T) map[string]bool {
	t.Helper()
	row := factoryReadsRow.FindStringSubmatch(readFile(t, "../../docs/manual/en-US/software-factory.md"))
	if row == nil {
		t.Fatal("the manual has no read, untrusted row in its classification table")
	}
	reads := map[string]bool{}
	for _, m := range backticked.FindAllStringSubmatch(row[1], -1) {
		reads[m[1]] = true
	}
	// The anchor: a broken scan would make every tool a write, or none.
	if !reads["issue_read"] || reads["issue_write"] {
		t.Fatalf("reads parsed from the manual = %v; the scan is broken", reads)
	}
	return reads
}

func TestFactoryReferences_stopsWhen_isNeverFollowedByAStepThatWrites(t *testing.T) {
	t.Parallel()

	reads := factoryReadTools(t)
	for role, data := range factoryReferences(t) {
		s, err := spec.Parse(role, data)
		if err != nil {
			t.Fatalf("parse %s: %v", role, err)
		}
		for i, step := range s.Steps {
			if step.StopsWhen == "" {
				continue
			}
			for _, later := range s.Steps[i+1:] {
				for _, tool := range later.Reaches {
					_, name, _ := strings.Cut(string(tool), ".")
					if !reads[name] {
						t.Errorf("%s: step %q stops the run when %q, but the later step %q reaches the write %s: the run would end before writing its outcome",
							role, step.Name, step.StopsWhen, later.Name, tool)
					}
				}
			}
		}
	}
}
