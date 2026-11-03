package spec_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/spec"
)

// The board is moved by the factory's own MCP server, not by the hosted GitHub
// server's projects_write (#213). That tool updates any field of any item and
// deletes items, behind one name the Gate classifies as a whole; move_card
// changes only a card's Status, along the calling role's row of the
// transition table, and set_card_fields only Work type and Size, only in
// Triagem (factory/mcp/README.md § Roles and their surfaces). These tests
// keep every reference on the narrow tools.

// boardLine is the sentence every role's instructions carry, on a line of its
// own: the model is told the same thing the pack says.
const boardLine = "Move a card only with move_card; never call projects_write."

// fieldsLine is the sentence of the two roles that classify a card in
// Triagem. No other role may set fields: set_card_fields refuses a card that
// has left Triagem, and only these two act there.
const fieldsLine = "Set Work type and Size only with set_card_fields."

// factorySurface is each role's tools on its factory-<role> registration:
// the guard's table, which is what the server serves to that role's App.
var factorySurface = map[string][]string{
	"triage":    {"move_card", "set_card_fields"},
	"architect": {"move_card", "set_card_fields"},
	"developer": {"move_card"},
	"reviewer":  {"move_card"},
	"qa":        {"move_card", "record_qa_verdict"},
	"merger":    {"list_ready_prs", "merge_if_ready", "move_card", "send_back"},
}

func parsedReferences(t *testing.T) map[string]spec.Spec {
	t.Helper()
	out := map[string]spec.Spec{}
	for role, data := range factoryReferences(t) {
		s, err := spec.Parse(role, data)
		if err != nil {
			t.Fatalf("parse %s: %v", role, err)
		}
		out[role] = s
	}
	return out
}

// No role holds projects_write, in its pack or in any step: a tool declared
// nowhere cannot be proposed, whatever an issue's text asks for.
func TestFactoryReferences_noRoleDeclaresProjectsWrite(t *testing.T) {
	t.Parallel()

	scanned := 0
	for role, s := range parsedReferences(t) {
		reached := slices.Clone(s.Tools)
		for _, step := range s.Steps {
			reached = append(reached, step.Reaches...)
		}
		for _, tool := range reached {
			scanned++
			if strings.HasSuffix(string(tool), ".projects_write") {
				t.Errorf("%s declares %s; cards move with move_card", role, tool)
			}
		}
	}
	// The anchor: a scan that read no tools would pass on any reference.
	if scanned < 50 {
		t.Fatalf("scanned %d tools across the references; the scan is broken", scanned)
	}
}

// Each role declares exactly its row of the factory server's surface, on its
// own registration: a role holding another's tool would be refused by the
// server, and a role missing move_card could not block a card at all.
func TestFactoryReferences_eachRole_declaresExactlyItsFactorySurface(t *testing.T) {
	t.Parallel()

	refs := parsedReferences(t)
	for role, want := range factorySurface {
		s, ok := refs[role]
		if !ok {
			t.Errorf("there is no %s reference", role)
			continue
		}
		var got []string
		for _, tool := range s.Tools {
			server, name, _ := strings.Cut(string(tool), ".")
			if !strings.HasPrefix(server, "factory-") {
				continue
			}
			if server != "factory-"+role {
				t.Errorf("%s declares %s, on another role's factory server", role, tool)
				continue
			}
			got = append(got, name)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s declares %v on factory-%s, want %v", role, got, role, want)
		}
	}
}

func TestFactoryReferences_eachRole_carriesTheBoardLineOnce(t *testing.T) {
	t.Parallel()

	refs := factoryReferences(t)
	for role := range factorySurface {
		lines := strings.Split(string(refs[role]), "\n")
		if count := countLines(lines, boardLine); count != 1 {
			t.Errorf("the %s reference carries the board line %d times, want 1", role, count)
		}
		want := 0
		if slices.Contains(factorySurface[role], "set_card_fields") {
			want = 1
		}
		if count := countLines(lines, fieldsLine); count != want {
			t.Errorf("the %s reference carries the fields line %d times, want %d", role, count, want)
		}
	}
}
