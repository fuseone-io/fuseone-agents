package docs

import "testing"

// The parser rather than the corpus, so proving these rules bite never means
// editing a page and putting it back. A test that has to damage real content
// to demonstrate itself is one restore away from being silently disarmed.
func TestParse_frontMatterMissingAField_isRefused(t *testing.T) {
	t.Parallel()

	page := func(lines string) string { return "---\n" + lines + "---\nBody.\n" }

	for name, front := range map[string]string{
		// `order` decides where a page lands in the index, and Go's zero value
		// is a real position. A page that forgets it does not fail — it
		// quietly becomes the first thing anybody reads.
		"no order":   "title: A\nsummary: B\n",
		"no title":   "summary: B\norder: 1\n",
		"no summary": "title: A\norder: 1\n",
		"no fence":   "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw := page(front)
			if name == "no fence" {
				raw = "title: A\nBody.\n"
			}
			if _, err := parse(raw); err == nil {
				t.Errorf("accepted a page with %s", name)
			}
		})
	}
}

func TestParse_orderZero_isAPositionSomebodyChose(t *testing.T) {
	t.Parallel()

	// Stated zero is not missing zero. Refusing it would make the first
	// position unusable to avoid a mistake the required check already catches.
	page, err := parse("---\ntitle: A\nsummary: B\norder: 0\n---\nBody.\n")
	if err != nil {
		t.Fatalf("refused an order somebody wrote down: %v", err)
	}
	if page.Order != 0 {
		t.Errorf("order is %d, not the 0 the page asked for", page.Order)
	}
}
