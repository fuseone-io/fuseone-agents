package connect_test

import (
	"slices"
	"testing"

	"github.com/fuseone/agents/internal/channel/connect"
)

/*
What this binary can connect.

Answered from the same table that builds a connection, because two lists saying
which vendors exist is one list offering a kind the process cannot make — and
that failure arrives as a connection that saves cleanly and never delivers.
*/
func TestKinds_areTheOnesWithADriver(t *testing.T) {
	t.Parallel()
	kinds := connect.Kinds()

	if !slices.Contains(kinds, "slack") {
		t.Errorf("kinds = %v, want the vendor this binary has a driver for", kinds)
	}
	if !slices.IsSorted(kinds) {
		t.Errorf("kinds = %v, want a stable order for the console to offer", kinds)
	}
}
