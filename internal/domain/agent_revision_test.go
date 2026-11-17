package domain

import "testing"

// An agent's tag names what publishing and retiring change: the current
// version and whether the agent is out of circulation (NT-017, phase 1).
func TestAgentRevision_movesWithTheVersionAndTheRetirement(t *testing.T) {
	t.Parallel()
	base := AgentRevision("v1a2b3c4d5e6f", false)
	if AgentRevision("v1a2b3c4d5e6f", false) != base {
		t.Fatal("the same version and retirement named two tags; an identical publish would look like drift")
	}
	if AgentRevision("v0000000000ff", false) == base {
		t.Error("another current version kept the tag")
	}
	if AgentRevision("v1a2b3c4d5e6f", true) == base {
		t.Error("retiring kept the tag")
	}
}

func TestParseIfNoneMatch_onlyTheWildcardAsksForAnAbsentAgent(t *testing.T) {
	t.Parallel()
	star, spaced, empty, tag := "*", " * ", "", `"v1-abc"`
	cases := []struct {
		name   string
		header *string
		absent bool
		bad    bool
	}{
		{name: "no header", header: nil},
		{name: "the wildcard", header: &star, absent: true},
		{name: "the wildcard with spaces", header: &spaced, absent: true},
		// A write is never conditional on a tag being absent here: refused
		// rather than ignored, as a malformed If-Match is.
		{name: "an entity-tag", header: &tag, bad: true},
		{name: "empty", header: &empty, bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			absent, err := ParseIfNoneMatch(tc.header)
			if (err != nil) != tc.bad || absent != tc.absent {
				t.Fatalf("ParseIfNoneMatch = %v, %v; want absent=%v bad=%v", absent, err, tc.absent, tc.bad)
			}
		})
	}
}
