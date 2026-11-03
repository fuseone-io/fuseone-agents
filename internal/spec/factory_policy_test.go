package spec_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

/*
merge_if_ready is destructive, and the Gate blocks a destructive effect
outright unless an authored policy says otherwise; a person's approval
releases only what needed approval, never a block. So a Merger started
without the policy proposes every merge into a block nobody can release.

Every setup text carries the one policy that turns that block into an
approval, field for field: this tool, this effect, this agent, escalate,
enforced. A wider rule — scope `*`, or every agent — would make any
destructive tool approvable, so the payload is compared whole.
cmd/agentd's factory_policy_journey_test.go stores it through the API and
holds the Gate to it.
*/

// mergePolicyRequest opens the policy's block in every setup text.
const mergePolicyRequest = "PUT /api/v1/policies/FACTORY-MERGE"

const mergePolicyPayload = `{
  "name": "A person approves every factory merge",
  "reason": "The Merger only proposes a merge: a person approves it, and merge_if_ready re-checks the review, QA and the checks on the head before it merges.",
  "resource": "factory-merger.merge_if_ready",
  "effects": ["destructive"],
  "reach": "agents",
  "agents": ["factory-merger"],
  "effect": "escalate",
  "mode": "enforce",
  "enabled": true
}`

// mergePolicyIn is the JSON body of the policy's block: from the first line
// that is exactly "{" after the request line, to the line before the fence.
func mergePolicyIn(t *testing.T, path, text string) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	start := slices.Index(lines, mergePolicyRequest)
	if start < 0 {
		t.Fatalf("%s has no line %q", path, mergePolicyRequest)
	}
	open := slices.Index(lines[start:], "{")
	if open < 0 {
		t.Fatalf("%s: the merge policy has no body", path)
	}
	body := lines[start+open:]
	end := slices.Index(body, "```")
	if end < 0 {
		t.Fatalf("%s never closes the merge policy's block", path)
	}
	return strings.Join(body[:end], "\n")
}

func TestFactorySetup_mergePolicy_isExactlyTheMergersEscalationInEveryText(t *testing.T) {
	t.Parallel()

	want, err := decodeJSON(mergePolicyPayload)
	if err != nil {
		t.Fatalf("the expected payload does not decode: %v", err)
	}
	for _, path := range factorySetupTexts {
		text := readFile(t, path)
		if count := countLines(strings.Split(text, "\n"), mergePolicyRequest); count != 1 {
			t.Errorf("%s carries the merge policy %d times, want 1", path, count)
			continue
		}
		got, err := decodeJSON(mergePolicyIn(t, path, text))
		if err != nil {
			t.Errorf("%s: the merge policy is not JSON: %v", path, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the merge policy is\n%v\nwant\n%v", path, got, want)
		}
	}
}

// The policy is a prerequisite, said where starting the Merger is: the
// sentence that lists what main and the sidecar must carry names it too.
func TestFactorySetup_mergerStart_waitsForThePolicyInEveryText(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"../../factory/agents/merger.agent.md":        mergerStartEN,
		"../../factory/agents/README.md":              mergerStartEN,
		"../../docs/manual/en-US/software-factory.md": mergerStartEN,
		"../../docs/manual/pt-BR/software-factory.md": mergerStartPT,
	}
	for path, sentence := range want {
		if count := countLines(strings.Split(readFile(t, path), "\n"), sentence); count != 1 {
			t.Errorf("%s carries the Merger's start condition %d times, want 1", path, count)
		}
	}
}

const (
	mergerStartEN = "Start the Merger only once the FACTORY-MERGE policy is stored and enforced; without it, the Gate blocks every merge and no approval releases it."
	mergerStartPT = "Inicie o Merger só depois que a política FACTORY-MERGE estiver salva e em enforce; sem ela, o Gate bloqueia todo merge e nenhuma aprovação o libera."
)
