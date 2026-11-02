package spec_test

import (
	"reflect"
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

// A role's single stops_when must cover every way it has into Travado, so
// a run that blocks a card always ends with stopped_by in the trail, whatever
// the reason. It does by construction when it names the end state they all
// share: the card in Travado, and the blocking comment ending with its
// Return to: line. A reference whose instructions move cards to Travado or
// write that line must say both in the stops_when of its last step.
func TestFactoryReferences_travadoRoles_endWithAStopsWhenThatNamesTravadoAndTheReturnLine(t *testing.T) {
	t.Parallel()

	for role, data := range factoryReferences(t) {
		s, err := spec.Parse(role, data)
		if err != nil {
			t.Fatalf("parse %s: %v", role, err)
		}
		if !strings.Contains(s.Instructions, "to Travado") && !strings.Contains(s.Instructions, "Return to:") {
			continue
		}
		if len(s.Steps) == 0 {
			t.Errorf("%s moves cards to Travado but declares no steps to carry a stops_when", role)
			continue
		}
		last := s.Steps[len(s.Steps)-1].StopsWhen
		if !strings.Contains(last, "Travado") || !strings.Contains(last, "Return to:") {
			t.Errorf("%s moves cards to Travado, but its last step's stops_when %q does not name Travado and Return to:", role, last)
		}
	}
}

// stopsWhen is each role's exception, step by step, as the spec parses it.
// The position test above only says a stops_when sits where no write
// follows; it is safe there only because its text names the block as
// already written — the question commented and the card moved. A text that
// names only the situation ("a decision belongs to the owner") would end
// the run on the last step before those writes, so the whole text is
// compared, per role, and a new stops_when anywhere fails until it is added
// here.
var stopsWhen = map[string]map[string]string{
	"triage": {
		"Write the outcome back": "the card is in Travado, and the comment that blocked it names the action that unblocks it and ends with Return to: Triagem",
	},
	"architect": {
		"Write the plan back": "the card is in Travado, and the comment that blocked it names the action that unblocks it and ends with Return to: Triagem",
	},
	"developer": {
		"Write the change": "the card is left for a person: its issue labelled factory:needs-runtime with the reason commented, or the card in Travado, and the comment that blocked it names the action that unblocks it and ends with Return to: Doing",
	},
	"reviewer": {
		"Submit the verdict": "the card is in Travado, and the comment that blocked it names the action that unblocks it and ends with Return to: Doing",
	},
	"qa": {
		"Submit the verdict": "the card is in Travado, and the comment that blocked it names the action that unblocks it and ends with Return to: Doing",
	},
}

func TestFactoryReferences_stopsWhen_isExactlyTheTextThatNamesTheBlockAsWritten(t *testing.T) {
	t.Parallel()

	refs := factoryReferences(t)
	for role, data := range refs {
		s, err := spec.Parse(role, data)
		if err != nil {
			t.Fatalf("parse %s: %v", role, err)
		}
		got := map[string]string{}
		for _, step := range s.Steps {
			if step.StopsWhen != "" {
				got[step.Name] = step.StopsWhen
			}
		}
		want, ok := stopsWhen[role]
		if !ok {
			t.Errorf("the %s reference has no stops_when declared here", role)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("the %s reference's stops_when is\n%q\nwant\n%q", role, got, want)
		}
	}
	for role := range stopsWhen {
		if _, ok := refs[role]; !ok {
			t.Errorf("there is no %s reference", role)
		}
	}
}
