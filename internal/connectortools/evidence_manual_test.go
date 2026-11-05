package connectortools

import (
	"os"
	"regexp"
	"testing"
)

var manualJSONRE = regexp.MustCompile("(?s)```json\n(.*?)```")

// Somebody will copy these. A declaration in the manual that the validator
// refuses, or that has a field the store silently drops, is a broken first day.
func TestManual_evidenceDeclarationsAreAccepted(t *testing.T) {
	t.Parallel()
	for _, locale := range []string{"en-US", "pt-BR"} {
		raw, err := os.ReadFile("../../docs/manual/" + locale + "/evidence-collection.md")
		if err != nil {
			t.Fatal(err)
		}
		blocks := manualJSONRE.FindAllSubmatch(raw, -1)
		if len(blocks) != 2 {
			t.Fatalf("%s holds %d declarations, want 2", locale, len(blocks))
		}
		for _, block := range blocks {
			var cfg EvidenceConfig
			if !decodeStrict(block[1], &cfg) {
				t.Fatalf("%s: a declaration has an unknown field: %s", locale, block[1])
			}
			instance := Instance{Connector: "evidence", Name: "example", Evidence: cfg}
			if err := ValidateInstanceConfig(instance); err != nil {
				t.Fatalf("%s: %v", locale, err)
			}
		}
	}
}
