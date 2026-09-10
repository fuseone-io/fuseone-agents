package channelmetrics_test

import (
	"os"
	"strings"
	"testing"

	"github.com/fuseone/agents/internal/channelmetrics"
)

/*
Every code an operator can be shown has words for it.

The cockpit renders a failure by looking the code up in the console's table, and
falls back to the code itself. So a code added here and forgotten there does not
break anything: it puts `channel_nobody_reachable` on the screen of somebody
trying to find out why an approval never arrived, which is the moment they can
least afford to be reading our identifiers.

The two lists live in different languages, so nothing but a test can hold them
together. It reads the file rather than the built bundle: what it is checking is
that somebody added a line, and that is what the file is.
*/
func TestCodes_haveALabelInTheConsole(t *testing.T) {
	t.Parallel()

	const table = "../../web/src/features/runtime/failure-labels.ts"
	labels, err := os.ReadFile(table)
	if err != nil {
		t.Fatalf("read the console's failure labels: %v", err)
	}

	for _, code := range channelmetrics.Codes() {
		if !strings.Contains(string(labels), code+":") {
			t.Errorf("%s has no label in %s, so it reaches an operator as itself",
				code, table)
		}
	}
}
