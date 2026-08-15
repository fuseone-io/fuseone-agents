package known_test

import (
	"testing"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/known"
)

/*
What the platform already knows about servers other people publish.

A suggestion is matched against the tool the server actually answered with, so
an entry that has aged degrades into silence rather than into a wrong answer.
That is the property that makes shipping these defensible at all: the worst a
stale entry can do is leave the Curator where they would have been without it.
*/

func TestSuggest_aToolTheEntryKnows_carriesAnEffectAndAReason(t *testing.T) {
	t.Parallel()

	got, ok := load(t).Suggest("github", "merge_pull_request")
	if !ok {
		t.Fatal("nothing suggested for a tool the entry names")
	}
	if got.Effect != domain.EffectDestructive.String() {
		t.Errorf("effect = %q, want destructive", got.Effect)
	}
	if got.Why == "" {
		t.Error("a suggested classification with no reasoning is a number to click past")
	}
}

func TestSuggest_aToolTheEntryNeverHeardOf_saysNothing(t *testing.T) {
	t.Parallel()

	if _, ok := load(t).Suggest("github", "summon_the_moon"); ok {
		t.Error("suggested something for a tool the server never offered")
	}
}

func TestSuggest_aServerNobodyCatalogued_saysNothing(t *testing.T) {
	t.Parallel()

	if _, ok := load(t).Suggest("acme-internal", "read_file"); ok {
		t.Error("suggested something for a server the platform has never seen")
	}
}

/*
Every shipped entry has to be usable and honest.

An effect the domain does not know is a typo that becomes a suggestion nobody
can accept, discovered at the moment somebody is trying to accept it. And an
entry that does not say how far to trust it is one the console cannot present
differently from one somebody verified — which is what makes verifying worth
doing.
*/
func TestEntries_everySuggestionIsRealAndExplained(t *testing.T) {
	t.Parallel()

	for _, entry := range load(t).All() {
		if entry.Provenance == "" {
			t.Errorf("%s does not say how far it should be trusted", entry.Server)
		}
		if entry.Docs == "" {
			t.Errorf("%s points nowhere an operator can read", entry.Server)
		}
		for _, s := range entry.Suggestions {
			if _, err := domain.ParseEffect(s.Effect); err != nil {
				t.Errorf("%s/%s suggests %q: %v", entry.Server, s.Tool, s.Effect, err)
			}
			if s.Why == "" {
				t.Errorf("%s/%s suggests an effect and no reason", entry.Server, s.Tool)
			}
		}
	}
}

func load(t *testing.T) *known.Servers {
	t.Helper()
	servers, err := known.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return servers
}
