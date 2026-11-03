package spec_test

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/spec"
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

// minimalRulesetHeredoc opens the command every setup text gives for the
// minimal ruleset (POST /repos/{owner}/{repo}/rulesets); the payload follows
// on the next lines, up to a line that is EOF alone.
const minimalRulesetHeredoc = `gh api --method POST repos/<owner>/<name>/rulesets --input - <<'EOF'`

// minimalRulesetPayload is what that command must send, compared as JSON
// values rather than as text: the pull_request rule is the one that refuses
// a direct push, and its five parameters are the ones GitHub requires.
const minimalRulesetPayload = `{
  "name": "main: pull request required",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },
  "rules": [
    { "type": "pull_request", "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false } },
    { "type": "non_fast_forward" },
    { "type": "deletion" }
  ],
  "bypass_actors": []
}`

// rulesetPayload returns the payload of the minimal ruleset command in text,
// or fails when the text has no such command or it is not closed.
func rulesetPayload(t *testing.T, path, text string) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	start := slices.Index(lines, minimalRulesetHeredoc)
	if start < 0 {
		t.Fatalf("%s has no line %q", path, minimalRulesetHeredoc)
	}
	end := slices.Index(lines[start+1:], "EOF")
	if end < 0 {
		t.Fatalf("%s never closes the minimal ruleset heredoc", path)
	}
	return strings.Join(lines[start+1:start+1+end], "\n")
}

// decodeJSON decodes into an untyped value on purpose: the comparison is of
// the whole document, so a field or rule the expected payload lacks fails it
// rather than being dropped by a struct that does not declare it.
func decodeJSON(data string) (any, error) {
	var v any
	err := json.Unmarshal([]byte(data), &v)
	return v, err
}

// No role moves a card out of Travado, and answering the question on the
// issue moves nothing: without a person whose job is the Travado lane, a
// blocked card sits there forever. In v1 that person is the board operator,
// stated in one exact sentence per language wherever the lanes and the setup
// are described. A card goes back where it was blocked, not to one lane for
// all: a pull request blocked in Doing sent to Triagem would be triaged again
// while it still carries its sub-state label. The blocking comment names the
// lane (factory_references_test.go, returnLines).
const (
	travadoOperatorEN = "The board operator reviews the Travado lane every working day and, once the blocker named in the comment that blocked a card is cleared, moves the card back to the lane that comment names, Triagem or Doing, leaving its labels as they are; that comment is the most recent one by the blocking role's GitHub App that ends with a Return to: line, on the issue or pull request the role worked on, where a pull request is either the card itself or the one in the card's Linked pull requests field."
	travadoOperatorPT = "Quem opera o board revisa a coluna Travado todo dia útil e, quando o bloqueio indicado no comentário que travou um card é resolvido, devolve o card para a coluna que esse comentário indica, Triagem ou Doing, sem mexer nos rótulos; esse comentário é o mais recente do GitHub App do papel que travou o card que termina com uma linha Return to:, na issue ou no pull request em que o papel trabalhou, sendo o pull request o próprio card ou o que aparece no campo Linked pull requests do card."
)

// A card whose own item was closed while it waited in Travado is finished,
// not unblocked: returned to its lane, a role could pick it up again (a
// duplicate a person closed would be triaged again). Only the item the card
// represents decides it: a card is usually the issue, with its pull request
// under Linked pull requests, and a pull request closed without a merge
// while that issue stays open finishes nothing. That case is stated too,
// so the operator does not bury an open issue in Done.
const (
	travadoClosedEN = "A card goes back to no lane once the item it represents is finished: the board operator moves it from Travado to Done when the card is an issue that is closed, or a pull request that is merged or closed."
	travadoClosedPT = "Um card não volta para nenhuma coluna quando o item que ele representa terminou: quem opera o board o move de Travado para Done quando o card é uma issue fechada, ou um pull request com merge feito ou fechado."
	linkedClosedEN  = "A pull request closed without a merge while the card's issue stays open does not finish the card: it goes back to the lane its Return to: line names, and a person starts the next attempt, since the Developer skips a card that has a linked pull request."
	linkedClosedPT  = "Um pull request fechado sem merge enquanto a issue do card continua aberta não encerra o card: ele volta para a coluna indicada pela linha Return to:, e uma pessoa começa a próxima tentativa, já que o Desenvolvedor pula um card que tem pull request vinculado."
)

func TestFactorySetup_travado_sendsAClosedCardToDoneInEveryText(t *testing.T) {
	t.Parallel()

	want := map[string][]string{
		"../../factory/LANES.md":                      {travadoClosedEN, linkedClosedEN},
		"../../factory/agents/README.md":              {travadoClosedEN, linkedClosedEN},
		"../../docs/manual/en-US/software-factory.md": {travadoClosedEN, linkedClosedEN},
		"../../docs/manual/pt-BR/software-factory.md": {travadoClosedPT, linkedClosedPT},
	}
	for path, sentences := range want {
		lines := strings.Split(readFile(t, path), "\n")
		for _, sentence := range sentences {
			if count := countLines(lines, sentence); count != 1 {
				t.Errorf("%s carries the closed-card line %q %d times, want 1", path, sentence, count)
			}
		}
	}
}

func TestFactorySetup_travado_hasTheBoardOperatorAsItsOwnerInEveryText(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"../../factory/LANES.md":                      travadoOperatorEN,
		"../../factory/agents/README.md":              travadoOperatorEN,
		"../../docs/manual/en-US/software-factory.md": travadoOperatorEN,
		"../../docs/manual/pt-BR/software-factory.md": travadoOperatorPT,
	}
	for path, sentence := range want {
		lines := strings.Split(readFile(t, path), "\n")
		if count := countLines(lines, sentence); count != 1 {
			t.Errorf("%s carries the Travado owner line %d times, want 1", path, count)
		}
	}
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
}

func TestFactorySetup_minimalRulesetCommand_sendsTheExpectedPayloadInEveryText(t *testing.T) {
	t.Parallel()

	want, err := decodeJSON(minimalRulesetPayload)
	if err != nil {
		t.Fatalf("the expected payload does not decode: %v", err)
	}
	for _, path := range factorySetupTexts {
		got, err := decodeJSON(rulesetPayload(t, path, readFile(t, path)))
		if err != nil {
			t.Errorf("%s: the minimal ruleset payload is not JSON: %v", path, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the minimal ruleset payload is\n%v\nwant\n%v", path, got, want)
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

// factoryServers is every MCP server the role references reach, read from
// their tools (github-<role>.<tool>), sorted. Platform tools ($fuseone.*)
// need no registration.
func factoryServers(t *testing.T) []string {
	t.Helper()
	var servers []string
	for role, data := range factoryReferences(t) {
		s, err := spec.Parse(role, data)
		if err != nil {
			t.Fatalf("parse %s: %v", role, err)
		}
		for _, tool := range s.Tools {
			server, _, ok := strings.Cut(string(tool), ".")
			if !ok || strings.HasPrefix(server, "$") || slices.Contains(servers, server) {
				continue
			}
			servers = append(servers, server)
		}
	}
	slices.Sort(servers)
	return servers
}

// A role whose server the setup never names is a role nobody registers as
// documented: its publish is refused for every GitHub tool it declares.
func TestFactorySetup_everyServerTheRolesReach_isNamedByTheSetup(t *testing.T) {
	t.Parallel()

	servers := factoryServers(t)
	// The anchor: an empty scan would pass on any setup text. Each role
	// reaches two servers, the hosted GitHub one and the factory's own.
	if !slices.Contains(servers, "github-triage") || !slices.Contains(servers, "github-developer") ||
		!slices.Contains(servers, "factory-triage") || !slices.Contains(servers, "factory-qa") {
		t.Fatalf("servers read from the references = %v; the scan is broken", servers)
	}
	for _, path := range factorySetupTexts {
		text := readFile(t, path)
		for _, server := range servers {
			if !strings.Contains(text, "`"+server+"`") {
				t.Errorf("%s never names the server `%s`", path, server)
			}
		}
	}
}

// The Merger is the one role that merges, so before it starts main must also
// require the checks merge_if_ready waits for, from GitHub Actions and from
// nothing else: a check run's name proves nothing, since anyone who can write
// checks can publish one, so each is pinned to the app that emits it (15368,
// github-actions). The list is FACTORY_CHECKS, the jobs of check.yml
// (factory/mcp/checks_doc_test.go), and nobody bypasses it. factory/qa and
// the Reviewer's approval stay outside: requiring them with no bypass would
// block every pull request that does not go through the factory, and a
// ruleset cannot name a GitHub App as a required reviewer. merge_if_ready
// requires both on the head.
const (
	checksRulesetEN = "Before the Merger is started, main must also carry the checks ruleset: the six checks of the check workflow required from GitHub Actions on an up-to-date head, and no bypass for anyone."
	checksRulesetPT = "Antes de iniciar o Merger, o main precisa ter também o ruleset de checks: os seis checks do workflow check exigidos do GitHub Actions num head atualizado, e nenhum bypass para ninguém."
)

const checksRulesetHeredoc = `gh api --method POST repos/<owner>/<name>/rulesets --input - <<'CHECKS'`

const checksRulesetPayload = `{
  "name": "main: checks required",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },
  "rules": [
    { "type": "required_status_checks", "parameters": {
        "strict_required_status_checks_policy": true,
        "do_not_enforce_on_create": false,
        "required_status_checks": [
          { "context": "go", "integration_id": 15368 },
          { "context": "postgres", "integration_id": 15368 },
          { "context": "console", "integration_id": 15368 },
          { "context": "chart", "integration_id": 15368 },
          { "context": "factory-mcp-image", "integration_id": 15368 },
          { "context": "artifact", "integration_id": 15368 }
        ] } }
  ],
  "bypass_actors": []
}`

// factoryChecks is FACTORY_CHECKS as factory/mcp/README.md states it, the
// list its own test ties to check.yml.
var factoryChecks = regexp.MustCompile("(?m)^\\| `FACTORY_CHECKS` \\|[^\\n]*?`([a-z-]+(?:,[a-z-]+)+)`")

func TestFactorySetup_checksRuleset_isTheMergersPrerequisiteInEveryText(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"../../factory/agents/merger.agent.md":        checksRulesetEN,
		"../../factory/agents/README.md":              checksRulesetEN,
		"../../docs/manual/en-US/software-factory.md": checksRulesetEN,
		"../../docs/manual/pt-BR/software-factory.md": checksRulesetPT,
	}
	for path, sentence := range want {
		if count := countLines(strings.Split(readFile(t, path), "\n"), sentence); count != 1 {
			t.Errorf("%s carries the checks ruleset line %d times, want 1", path, count)
		}
	}
}

func TestFactorySetup_checksRulesetCommand_requiresExactlyFactoryChecksFromActions(t *testing.T) {
	t.Parallel()

	want, err := decodeJSON(checksRulesetPayload)
	if err != nil {
		t.Fatalf("the expected payload does not decode: %v", err)
	}
	// The payload's contexts are FACTORY_CHECKS, in its order: the Merger's
	// list and the ruleset's must be one list.
	m := factoryChecks.FindStringSubmatch(readFile(t, "../../factory/mcp/README.md"))
	if m == nil {
		t.Fatal("factory/mcp/README.md states no FACTORY_CHECKS")
	}
	var contexts []string
	for _, c := range want.(map[string]any)["rules"].([]any)[0].(map[string]any)["parameters"].(map[string]any)["required_status_checks"].([]any) {
		contexts = append(contexts, c.(map[string]any)["context"].(string))
	}
	if strings.Join(contexts, ",") != m[1] {
		t.Fatalf("the checks ruleset requires %v, but FACTORY_CHECKS is %s", contexts, m[1])
	}
	for _, path := range factorySetupTexts {
		text := readFile(t, path)
		lines := strings.Split(text, "\n")
		start := slices.Index(lines, checksRulesetHeredoc)
		if start < 0 {
			t.Errorf("%s has no line %q", path, checksRulesetHeredoc)
			continue
		}
		end := slices.Index(lines[start+1:], "CHECKS")
		if end < 0 {
			t.Errorf("%s never closes the checks ruleset heredoc", path)
			continue
		}
		got, err := decodeJSON(strings.Join(lines[start+1:start+1+end], "\n"))
		if err != nil {
			t.Errorf("%s: the checks ruleset payload is not JSON: %v", path, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the checks ruleset payload is\n%v\nwant\n%v", path, got, want)
		}
	}
}
