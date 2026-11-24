package spec_test

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/domain"
)

/*
The console marks a granted tool no step reaches before the draft is saved,
and so restates which tools are the platform's. The set is explicit on both
sides: a prefix would exempt a typo of a platform tool, or a vendor squatting
the sigil, and the console would stay quiet about a tool the server refuses.
This holds the console's list to domain.PlatformTools, both ways.
*/
func TestPlatformTools_theConsoleListsExactlyTheServersSet(t *testing.T) {
	t.Parallel()
	const file = "../../web/src/features/agents/platform-tools.ts"
	source, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read the console's platform tools: %v", err)
	}
	var listed []string
	for _, m := range regexp.MustCompile(`"(\$fuseone\.[^"]+)"`).FindAllStringSubmatch(string(source), -1) {
		listed = append(listed, m[1])
	}
	var want []string
	for _, tool := range domain.PlatformTools() {
		want = append(want, string(tool))
	}
	slices.Sort(listed)
	slices.Sort(want)
	if !slices.Equal(listed, want) {
		t.Errorf("%s lists %v, the server's platform tools are %v", file, listed, want)
	}
}
