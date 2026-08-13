package catalogue_test

import (
	"testing"

	"github.com/fuseone/agents/internal/catalogue"
)

/*
The catalogue ships inside the binary, so a broken template is a broken
release rather than a broken installation. These are the assertions that make
that true at build time.
*/

func TestAll_everyTemplateParses(t *testing.T) {
	t.Parallel()

	all, err := catalogue.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) < 4 {
		t.Fatalf("catalogue holds %d templates, want the four the PRD names", len(all))
	}

	for _, template := range all {
		if template.Summary == "" {
			t.Errorf("%s has no summary; the gallery has nothing to show", template.ID)
		}
		if len(template.Needs) == 0 {
			t.Errorf("%s names no needs, so an author cannot tell what to pick", template.ID)
		}
	}
}

func TestAll_namesNoTools(t *testing.T) {
	t.Parallel()

	// A template naming crm.reply is broken in every installation that calls
	// its CRM something else, and picking the pack is the author's act: they
	// choose from what the Curator connected (PRD SE-03). What a template
	// carries instead is the role, in words.
	all, err := catalogue.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	for _, template := range all {
		for _, need := range template.Needs {
			if len(need) > 0 && need[0] >= 'a' && need[0] <= 'z' {
				t.Errorf("%s: need %q reads like a tool id rather than a role", template.ID, need)
			}
		}
	}
}

func TestGet_unknownTemplate_isAnError(t *testing.T) {
	t.Parallel()

	if _, err := catalogue.Get("nao-existe"); err == nil {
		t.Error("Get accepted a template that does not exist")
	}
}
