package spec_test

import (
	"strings"
	"testing"
)

/*
No role merges directly. The Merger's one merge is merge_if_ready, which a
person approves on every call (FACTORY-MERGE), and a person may still merge
by hand. The rule every role reads, RULEBOOK § 5, and the row of the
references' table that explains why no reference declares
merge_pull_request, say exactly that: a text still saying a person merges in
v1 contradicts the Merger, and one saying a role never merges at all would
contradict it the other way.
*/

const mergeRule = "A role never merges a pull request directly and never asks for or holds `merge_pull_request`. The Merger's one merge is `merge_if_ready`, which a person approves on every call and which re-checks the Reviewer's approval, `factory/qa` and the checks on the head; a person may still merge by hand (LANES.md § `factory:ready-to-merge`)."

const mergeRuleRow = "| no `merge_pull_request` | no role merges directly: the Merger's one merge is `merge_if_ready`, which a person approves on every call |"

func TestFactoryTexts_mergeRule_isTheMergersRuleNotV1s(t *testing.T) {
	t.Parallel()

	for path, line := range map[string]string{
		"../../factory/RULEBOOK.md":      mergeRule,
		"../../factory/agents/README.md": mergeRuleRow,
	} {
		text := readFile(t, path)
		if count := countLines(strings.Split(text, "\n"), line); count != 1 {
			t.Errorf("%s carries the merge rule %d times, want 1", path, count)
		}
		for _, stale := range []string{"a person merges in v1", "In v1\na person merges", "never merges, and never asks"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still says %q", path, stale)
			}
		}
	}
}
