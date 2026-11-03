package spec_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The hosted GitHub server's projects set offers projects_write beside the
// reads the roles need, and no role declares it any more (#213). Leaving it
// out of the packs is an instruction to whoever copies a reference; what
// keeps it out mechanically is each registration's tool list, the one the
// setup texts give per server: the platform's surface, and the X-MCP-Tools
// header that narrows the hosted server itself. Both are only as good as the
// list, so it is compared with the references here.

// surfaceRow is one row of that table: a github-<role> server, then its tools
// as one comma-separated list, the form X-MCP-Tools takes.
var surfaceRow = regexp.MustCompile("(?m)^\\| `(github-[a-z]+)` \\| `([a-z_,]+)` \\|$")

// githubTools is every tool a reference declares on its github-<role>
// server, sorted.
func githubTools(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for role, s := range parsedReferences(t) {
		server := "github-" + role
		for _, tool := range s.Tools {
			if name, ok := strings.CutPrefix(string(tool), server+"."); ok {
				out[server] = append(out[server], name)
			}
		}
		slices.Sort(out[server])
	}
	return out
}

func TestFactorySetup_eachRolesToolList_isExactlyTheGitHubToolsItsReferenceDeclares(t *testing.T) {
	t.Parallel()

	want := githubTools(t)
	// The anchor: a scan that found no tools would compare nothing.
	if !slices.Contains(want["github-triage"], "issue_read") || len(want) != 5 {
		t.Fatalf("GitHub tools read from the references = %v; the scan is broken", want)
	}
	for _, path := range factorySetupTexts {
		got := map[string][]string{}
		for _, row := range surfaceRow.FindAllStringSubmatch(readFile(t, path), -1) {
			if _, twice := got[row[1]]; twice {
				t.Errorf("%s lists the tools of `%s` twice", path, row[1])
			}
			tools := strings.Split(row[2], ",")
			slices.Sort(tools)
			got[row[1]] = tools
		}
		for server, tools := range want {
			if !slices.Equal(got[server], tools) {
				t.Errorf("%s gives `%s` the tools %v, want what its reference declares, %v", path, server, got[server], tools)
			}
		}
		for server, tools := range got {
			if slices.Contains(tools, "projects_write") {
				t.Errorf("%s brings projects_write into `%s`", path, server)
			}
		}
	}
}

// writeRow is a row of a manual's classification table that rules tools a
// write.
var writeRow = regexp.MustCompile("(?m)^\\| write[^|]*\\|(.*)\\|$")

// projects_write stays unclassified: an unclassified tool never executes
// (domain.EffectUnknown), so a registration that still offers it lends a
// steered run nothing. The factory's own tools are writes.
func TestFactorySetup_classification_leavesProjectsWriteUnclassifiedAndRulesTheBoardToolsWrites(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"../../docs/manual/en-US/software-factory.md",
		"../../docs/manual/pt-BR/software-factory.md",
	} {
		rows := writeRow.FindAllStringSubmatch(readFile(t, path), -1)
		if len(rows) == 0 {
			t.Fatalf("%s has no write row in its classification table", path)
		}
		writes := map[string]bool{}
		for _, row := range rows {
			for _, m := range backticked.FindAllStringSubmatch(row[1], -1) {
				writes[m[1]] = true
			}
		}
		if writes["projects_write"] {
			t.Errorf("%s classifies projects_write as a write; it stays unclassified", path)
		}
		for _, tool := range []string{"move_card", "set_card_fields", "record_qa_verdict", "issue_write"} {
			if !writes[tool] {
				t.Errorf("%s does not classify %s as a write", path, tool)
			}
		}
	}
}
