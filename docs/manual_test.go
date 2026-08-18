package docs_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/fuseone/agents/docs"
)

// Every locale, every time. A rule that runs against one translation is a rule
// the other translation does not have — and the untested one is always the one
// somebody edits in a hurry.
var locales = []string{"pt-BR", "en-US"}

// A link in the manual is the one thing no reviewer checks by hand, and the one
// that fails in front of a reader rather than in front of an author. The manual
// is served from a route that knows nothing about the repository, so a link to
// a document that is not in the manual is a 404 in the console however well it
// resolves on GitHub.
var link = regexp.MustCompile(`\]\(([^)]+)\)`)

func TestManual_everyInternalLinkNamesAPageTheManualHas(t *testing.T) {
	t.Parallel()

	for _, locale := range locales {
		t.Run(locale, func(t *testing.T) {
			t.Parallel()
			pages := read(t, locale)

			has := make(map[string]bool, len(pages))
			for _, p := range pages {
				has[p.Slug] = true
			}

			for _, p := range pages {
				for _, m := range link.FindAllStringSubmatch(p.Body, -1) {
					target := m[1]
					if isExternal(target) {
						continue
					}
					slug, _, _ := strings.Cut(strings.TrimSuffix(target, ".md"), "#")
					if slug == "" {
						continue // an anchor within the same page
					}
					if !has[slug] {
						t.Errorf("%s links to %q, which is not a page of the manual", p.Slug, target)
					}
				}
			}
		})
	}
}

func isExternal(target string) bool {
	for _, scheme := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(target, scheme) {
			return true
		}
	}
	return false
}

func TestManual_ordersAreUniqueSoTheIndexDoesNotReshuffle(t *testing.T) {
	t.Parallel()

	for _, locale := range locales {
		t.Run(locale, func(t *testing.T) {
			t.Parallel()

			at := make(map[int]string)
			for _, p := range read(t, locale) {
				if first, taken := at[p.Order]; taken {
					t.Errorf("%s and %s both claim order %d", first, p.Slug, p.Order)
				}
				at[p.Order] = p.Slug
			}
		})
	}
}

// The manual is user-facing content, and the repository's rule for user-facing
// content is that pt-BR and en-US stay in parity. A page written in one
// language only is not half a manual — it is a reader who switches language and
// watches a page disappear.
func TestManual_bothLocalesCarryTheSamePagesInTheSameOrder(t *testing.T) {
	t.Parallel()

	pt, en := read(t, "pt-BR"), read(t, "en-US")
	if len(pt) != len(en) {
		t.Fatalf("pt-BR has %d pages and en-US has %d", len(pt), len(en))
	}
	for i := range pt {
		// Same slug at the same position: the order is what the index shows,
		// so a page that sorts differently is a different manual.
		if pt[i].Slug != en[i].Slug {
			t.Errorf("position %d is %q in pt-BR and %q in en-US", i, pt[i].Slug, en[i].Slug)
		}
	}
}

func read(t *testing.T, locale string) []docs.Page {
	t.Helper()
	pages, err := docs.Manual(locale)
	if err != nil {
		t.Fatalf("%s: %v", locale, err)
	}
	if len(pages) == 0 {
		t.Fatalf("%s is empty, so the console would serve an index of nothing", locale)
	}
	return pages
}
