package authoring_test

import (
	"testing"

	"github.com/fuseone/agents/internal/authoring"
)

/*
How hard the assistant thinks about organising a paragraph.

Nobody chose "high" for this. It fell out of a default written for agent runs,
where a model reasoning at length about a decision it is about to take is the
point — and it reached the assistant, where the job is to reorganise a
description somebody already wrote into fields. The author waits, the
installation pays, and the answer is a JSON object either way.
*/

func TestEffortFor_nobodyChose_asksForLittleThinking(t *testing.T) {
	t.Parallel()

	if got := authoring.EffortFor(""); got != "low" {
		t.Errorf("effort = %q, want the modest default", got)
	}
}

func TestEffortFor_configured_isObeyed(t *testing.T) {
	t.Parallel()

	// An installation that finds the readings shallow raises it, and this
	// must not quietly override them.
	if got := authoring.EffortFor("high"); got != "high" {
		t.Errorf("effort = %q, want what was configured", got)
	}
}
