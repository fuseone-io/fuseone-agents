package spec_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/spec"
)

/*
A draft is a definition still being written (NT-018 1b).

What it lacks is not a defect yet: a draft born with an identifier, a name and
a scope has no tools, no provider, no budget and no instructions, and every
operation on it would be refused if it had to be publishable already. So the
draft's parse tells the two apart. What is structurally wrong is refused;
what is merely missing is listed, which is what the checklist reads.
*/

const draftOnlyIdentity = "---\nid: triage\nname: Triagem\ncompany: acme\narea: cx\n---\n"

func TestParseDraft_aDefinitionStillBeingWritten_listsWhatIsMissing(t *testing.T) {
	t.Parallel()
	s, missing, err := spec.ParseDraft("draft", []byte(draftOnlyIdentity))
	if err != nil {
		t.Fatalf("ParseDraft = %v, want a draft that only lacks things", err)
	}
	if s.ID != "triage" || s.Area != "cx" {
		t.Errorf("parsed %+v, want the identity kept", s)
	}
	want := []spec.Missing{spec.MissingInstructions, spec.MissingTools, spec.MissingBudget, spec.MissingProvider}
	if !slices.Equal(missing, want) {
		t.Errorf("missing = %v, want %v", missing, want)
	}
}

// What a publication would refuse for being wrong, not for being unfinished,
// is refused in a draft too.
func TestParseDraft_somethingStructurallyWrong_isRefused(t *testing.T) {
	t.Parallel()
	for name, front := range map[string]string{
		"a cache window the provider does not sell": "cache_prefix_ttl: 2h\n",
		"a step reaching outside the pack":          "tools: [crm.lookup]\nsteps:\n  - name: Look\n    reaches: [crm.delete]\n",
		"a trigger of no known type":                "triggers:\n  - { type: telepathy }\n",
		"an invalid company":                        "company: '*'\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data := "---\nid: triage\nname: Triagem\narea: cx\n" + front + "---\n"
			if _, _, err := spec.ParseDraft("draft", []byte(data)); !errors.Is(err, spec.ErrInvalid) {
				t.Fatalf("ParseDraft = %v, want ErrInvalid", err)
			}
		})
	}
}

// Without an identifier or an area there is no draft at all: they are what
// the human step at its birth sets, and nothing is listed instead of them.
func TestParseDraft_noIdentifierOrArea_isRefused(t *testing.T) {
	t.Parallel()
	for name, data := range map[string]string{
		"no id":   "---\nname: Triagem\narea: cx\n---\n",
		"no area": "---\nid: triage\nname: Triagem\n---\n",
	} {
		if _, _, err := spec.ParseDraft("draft", []byte(data)); !errors.Is(err, spec.ErrInvalid) {
			t.Errorf("%s: ParseDraft = %v, want ErrInvalid", name, err)
		}
	}
}

// A complete draft lists nothing and parses to what Parse gives.
func TestParseDraft_aCompleteDefinition_missesNothingAndMatchesParse(t *testing.T) {
	t.Parallel()
	s, missing, err := spec.ParseDraft("triage.agent.md", []byte(definition))
	if err != nil || len(missing) != 0 {
		t.Fatalf("ParseDraft = %v, missing %v; want a complete definition", err, missing)
	}
	parsed, err := spec.Parse("triage.agent.md", []byte(definition))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Version != parsed.Version || s.Instructions != parsed.Instructions {
		t.Errorf("ParseDraft %s and Parse %s disagree", s.Version, parsed.Version)
	}
}
