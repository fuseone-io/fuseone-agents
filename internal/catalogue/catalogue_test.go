package catalogue_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/catalogue"
)

/*
The catalogue ships inside the binary, so a broken template is a broken
release rather than a broken installation. These are the assertions that make
that true at build time.
*/

// Every language this installation ships. A template broken in one of them is
// a broken release, not a broken installation, and these run at build time.
var locales = []string{"pt-BR", "en-US"}

func TestAll_everyTemplateParses(t *testing.T) {
	t.Parallel()

	for _, locale := range locales {
		all, err := catalogue.All(locale)
		if err != nil {
			t.Fatalf("All(%s): %v", locale, err)
		}
		if len(all) < 4 {
			t.Fatalf("%s holds %d templates, want the four the PRD names", locale, len(all))
		}

		for _, template := range all {
			if template.Summary == "" {
				t.Errorf("%s/%s has no summary; the gallery has nothing to show",
					locale, template.ID)
			}
			if len(template.Needs) == 0 {
				t.Errorf("%s/%s names no needs, so an author cannot tell what to pick",
					locale, template.ID)
			}
		}
	}
}

func TestAll_everyLanguageOffersTheSameTemplates(t *testing.T) {
	t.Parallel()

	// A card in one language and not the other is an author who cannot start
	// from something a colleague can, for no reason they could discover.
	ids := map[string][]string{}
	for _, locale := range locales {
		all, err := catalogue.All(locale)
		if err != nil {
			t.Fatalf("All(%s): %v", locale, err)
		}
		for _, template := range all {
			ids[locale] = append(ids[locale], template.ID)
		}
		// The set, not the order: the gallery sorts by name, and names differ
		// per language, so a Portuguese gallery legitimately reads in a
		// different order from an English one.
		slices.Sort(ids[locale])
	}

	if !slices.Equal(ids["pt-BR"], ids["en-US"]) {
		t.Errorf("pt-BR has %v and en-US has %v", ids["pt-BR"], ids["en-US"])
	}
}

func TestAll_aLanguageNobodyShips_fallsBackRatherThanAnsweringEmpty(t *testing.T) {
	t.Parallel()

	// A console in a third language showing four cards it can read is better
	// off than one showing none.
	all, err := catalogue.All("de-DE")
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) < 4 {
		t.Errorf("got %d templates for an unshipped language, want the default set", len(all))
	}
}

func TestAll_namesNoTools(t *testing.T) {
	t.Parallel()

	// A template naming crm.reply is broken in every installation that calls
	// its CRM something else, and picking the pack is the author's act: they
	// choose from what the Curator connected (PRD SE-03). What a template
	// carries instead is the role, in words.
	all, err := catalogue.All(catalogue.Default)
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

	if _, err := catalogue.Get(catalogue.Default, "nao-existe"); err == nil {
		t.Error("Get accepted a template that does not exist")
	}
}
