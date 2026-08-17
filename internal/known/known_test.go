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
		if entry.Status == "" {
			t.Errorf("%s does not say whether it is published, reference or archived", entry.Server)
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

/*
Two questions a recipe has to answer separately.

Where the suggestions came from — somebody ran it, or somebody read the docs —
and whose documentation the link points at. They are independent: a community
server we actually ran has trustworthy suggestions and no publisher behind it,
and a publisher's own server read off their page is the reverse.

Collapsed into one word — "official" — both get worse. The careful entry looks
like the careless one, and the reader takes the label as a promise of support
that nobody made.
*/
func TestLoad_anEntryPointingAtThePublishersOwnDocs_saysSoWithoutClaimingSupport(t *testing.T) {
	t.Parallel()
	servers := load(t)

	for _, entry := range servers.All() {
		if entry.DocsFrom == known.DocsFromPublisher && entry.Docs == "" {
			t.Errorf("%s claims the publisher's documentation and links to none", entry.Server)
		}
		if entry.DocsFrom == "" {
			t.Errorf("%s does not say whose documentation it points at", entry.Server)
		}
	}
}

// A recipe fills the form and never submits it. What it proposes has to be
// something the form can hold, or the console offers a shape the platform
// cannot make.
func TestLoad_aSuggestedTransport_isOneThisBinaryCanBuild(t *testing.T) {
	t.Parallel()

	for _, entry := range load(t).All() {
		switch entry.Transport {
		case "stdio":
			if entry.Command == "" {
				t.Errorf("%s suggests stdio and no command to run", entry.Server)
			}
		case "http":
			if entry.URL == "" {
				t.Errorf("%s suggests http and no address to call", entry.Server)
			}
		case "":
			// No opinion is allowed, and honest: several servers ship both,
			// and picking one for somebody is a recommendation this package
			// has no basis for.
		default:
			t.Errorf("%s suggests %q, which is not a transport", entry.Server, entry.Transport)
		}
	}
}

/*
An entry may suggest nothing at all.

Identity, a link and a sentence about the credential are worth shipping on
their own — they fill the form and warn about what the token can reach. Effects
invented for tools nobody verified would be worse than silence, and silence is
what a stale entry is supposed to degrade into.
*/
func TestLoad_anEntryWithNoSuggestions_isValid(t *testing.T) {
	t.Parallel()

	for _, entry := range load(t).All() {
		if entry.Title == "" || entry.Publisher == "" {
			t.Errorf("%s ships without saying what it is or who publishes it", entry.Server)
		}
	}
}

/*
Reference implementations are not vendor integrations.

The upstream repository now points readers to the MCP Registry for the broad
list, keeps only a small reference set, and marks PostgreSQL as archived. If
that entry reads like a current published recipe again, the console will put
too much confidence on the weakest database option.
*/
func TestLoad_archivedReferenceIsNotPresentedAsPublished(t *testing.T) {
	t.Parallel()

	entry, ok := load(t).For("postgres")
	if !ok {
		t.Fatal("postgres recipe missing")
	}
	if entry.Status != known.StatusArchived {
		t.Fatalf("postgres status = %q, want archived", entry.Status)
	}
	if len(entry.Config) != 1 || entry.Config[0] != known.ConfigCredential {
		t.Fatalf("postgres config = %+v, want the database credential named", entry.Config)
	}
}
