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
		if containsCredential(entry.Config) && len(entry.AuthModes) == 0 {
			t.Errorf("%s asks for a credential without naming the authentication shape", entry.Server)
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

func containsCredential(config []known.ConfigRequirement) bool {
	for _, one := range config {
		if one == known.ConfigCredential {
			return true
		}
	}
	return false
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

func TestLoad_rootlyIsRemoteFirstAndKeepsIncidentTextTainted(t *testing.T) {
	t.Parallel()

	entry, ok := load(t).For("rootly")
	if !ok {
		t.Fatal("rootly recipe missing")
	}
	if entry.Transport != "http" || entry.URL != "https://mcp.rootly.com/mcp" {
		t.Fatalf("rootly connection = %s %s, want hosted HTTP MCP", entry.Transport, entry.URL)
	}
	if entry.Status != known.StatusPublished || entry.DocsFrom != known.DocsFromPublisher {
		t.Fatalf("rootly status/docs = %s/%s, want published publisher recipe", entry.Status, entry.DocsFrom)
	}
	if !hasAuthMode(entry.AuthModes, known.AuthOAuth2) || !hasAuthMode(entry.AuthModes, known.AuthBearer) {
		t.Fatalf("rootly auth modes = %+v, want OAuth and bearer documented", entry.AuthModes)
	}

	read, ok := load(t).Suggest("rootly", "get_alert_by_short_id")
	if !ok || read.Effect != domain.EffectRead.String() || read.Untrusted == nil || !*read.Untrusted {
		t.Fatalf("get_alert_by_short_id suggestion = %+v, want tainted read", read)
	}
	write, ok := load(t).Suggest("rootly", "createIncident")
	if !ok || write.Effect != domain.EffectWrite.String() {
		t.Fatalf("createIncident suggestion = %+v, want write", write)
	}
}

func TestLoad_operationsPackKeepsRemoteAndInstanceDefinedShapesSeparate(t *testing.T) {
	t.Parallel()

	servers := load(t)
	for _, want := range []struct {
		server string
		url    string
		auth   []known.AuthType
	}{
		{server: "pagerduty", url: "https://mcp.pagerduty.com/mcp", auth: []known.AuthType{known.AuthOAuth2, known.AuthHeaders}},
		{server: "incident-io", url: "https://mcp.incident.io/mcp", auth: []known.AuthType{known.AuthOAuth2, known.AuthBearer}},
		{server: "newrelic", url: "https://mcp.newrelic.com/mcp/", auth: []known.AuthType{known.AuthOAuth2, known.AuthHeaders}},
		{server: "honeycomb", url: "https://mcp.honeycomb.io/mcp", auth: []known.AuthType{known.AuthOAuth2, known.AuthBearer}},
	} {
		entry, ok := servers.For(want.server)
		if !ok {
			t.Fatalf("%s recipe missing", want.server)
		}
		if entry.Transport != "http" || entry.URL != want.url {
			t.Fatalf("%s connection = %s %s, want hosted HTTP MCP at %s",
				want.server, entry.Transport, entry.URL, want.url)
		}
		for _, auth := range want.auth {
			if !hasAuthMode(entry.AuthModes, auth) {
				t.Fatalf("%s auth modes = %+v, missing %s", want.server, entry.AuthModes, auth)
			}
		}
	}

	for _, server := range []string{"servicenow", "elastic-agent-builder"} {
		entry, ok := servers.For(server)
		if !ok {
			t.Fatalf("%s recipe missing", server)
		}
		if entry.Transport != "" || entry.URL != "" {
			t.Fatalf("%s connection = %s %s, want no universal URL invented",
				server, entry.Transport, entry.URL)
		}
		if len(entry.Suggestions) != 0 {
			t.Fatalf("%s suggests %d tools for an instance-defined tool catalog",
				server, len(entry.Suggestions))
		}
	}
}

func TestSuggest_operationsPackKeepsOperationalTextTainted(t *testing.T) {
	t.Parallel()

	servers := load(t)
	for _, want := range []struct {
		server string
		tool   string
		effect string
	}{
		{server: "pagerduty", tool: "list_alerts_from_incident", effect: domain.EffectRead.String()},
		{server: "incident-io", tool: "incident_show", effect: domain.EffectRead.String()},
		{server: "newrelic", tool: "list_recent_logs", effect: domain.EffectRead.String()},
		{server: "honeycomb", tool: "get_trace", effect: domain.EffectRead.String()},
	} {
		got, ok := servers.Suggest(want.server, want.tool)
		if !ok {
			t.Fatalf("%s/%s suggestion missing", want.server, want.tool)
		}
		if got.Effect != want.effect || got.Untrusted == nil || !*got.Untrusted {
			t.Fatalf("%s/%s = %+v, want tainted %s", want.server, want.tool, got, want.effect)
		}
	}

	for _, want := range []struct {
		server string
		tool   string
		effect string
	}{
		{server: "pagerduty", tool: "update_event_orchestration_router", effect: domain.EffectDestructive.String()},
		{server: "incident-io", tool: "incident_create", effect: domain.EffectWrite.String()},
		{server: "honeycomb", tool: "create_trigger", effect: domain.EffectWrite.String()},
	} {
		got, ok := servers.Suggest(want.server, want.tool)
		if !ok {
			t.Fatalf("%s/%s suggestion missing", want.server, want.tool)
		}
		if got.Effect != want.effect {
			t.Fatalf("%s/%s effect = %s, want %s", want.server, want.tool, got.Effect, want.effect)
		}
	}
}

func TestLoad_cloudflareRecipesDoNotInventProductSpecificToolNames(t *testing.T) {
	t.Parallel()

	servers := load(t)
	api, ok := servers.For("cloudflare-api")
	if !ok {
		t.Fatal("cloudflare-api recipe missing")
	}
	if api.Transport != "http" || api.URL != "https://mcp.cloudflare.com/mcp" {
		t.Fatalf("cloudflare-api connection = %s %s, want hosted HTTP MCP",
			api.Transport, api.URL)
	}
	if !hasAuthMode(api.AuthModes, known.AuthOAuth2) || !hasAuthMode(api.AuthModes, known.AuthBearer) {
		t.Fatalf("cloudflare-api auth modes = %+v, want OAuth and bearer token", api.AuthModes)
	}
	execute, ok := servers.Suggest("cloudflare-api", "execute")
	if !ok || execute.Effect != domain.EffectDestructive.String() {
		t.Fatalf("cloudflare-api execute = %+v, want destructive", execute)
	}

	for server, url := range map[string]string{
		"cloudflare-docs":           "https://docs.mcp.cloudflare.com/mcp",
		"cloudflare-observability":  "https://observability.mcp.cloudflare.com/mcp",
		"cloudflare-auditlogs":      "https://auditlogs.mcp.cloudflare.com/mcp",
		"cloudflare-workers-builds": "https://builds.mcp.cloudflare.com/mcp",
	} {
		entry, ok := servers.For(server)
		if !ok {
			t.Fatalf("%s recipe missing", server)
		}
		if entry.Transport != "http" || entry.URL != url {
			t.Fatalf("%s connection = %s %s, want %s", server, entry.Transport, entry.URL, url)
		}
		if !hasAuthMode(entry.AuthModes, known.AuthOAuth2) {
			t.Fatalf("%s auth modes = %+v, want OAuth", server, entry.AuthModes)
		}
		if len(entry.Suggestions) != 0 {
			t.Fatalf("%s suggests %d tools even though the source only names the server purpose",
				server, len(entry.Suggestions))
		}
	}
}

func TestSuggest_vercelKeepsMoneyAndLogsInTheRightBoxes(t *testing.T) {
	t.Parallel()

	servers := load(t)
	entry, ok := servers.For("vercel")
	if !ok {
		t.Fatal("vercel recipe missing")
	}
	if entry.Transport != "http" || entry.URL != "https://mcp.vercel.com" {
		t.Fatalf("vercel connection = %s %s, want hosted HTTP MCP", entry.Transport, entry.URL)
	}
	if !hasAuthMode(entry.AuthModes, known.AuthOAuth2) {
		t.Fatalf("vercel auth modes = %+v, want OAuth", entry.AuthModes)
	}

	buy, ok := servers.Suggest("vercel", "buy_domain")
	if !ok || buy.Effect != domain.EffectFinancial.String() {
		t.Fatalf("vercel buy_domain = %+v, want financial", buy)
	}
	logs, ok := servers.Suggest("vercel", "get_runtime_logs")
	if !ok || logs.Effect != domain.EffectRead.String() || logs.Untrusted == nil || !*logs.Untrusted {
		t.Fatalf("vercel get_runtime_logs = %+v, want tainted read", logs)
	}
	deploy, ok := servers.Suggest("vercel", "deploy_to_vercel")
	if !ok || deploy.Effect != domain.EffectWrite.String() {
		t.Fatalf("vercel deploy_to_vercel = %+v, want write", deploy)
	}
}

func hasAuthMode(modes []known.AuthMode, typ known.AuthType) bool {
	for _, one := range modes {
		if one.Type == typ {
			return true
		}
	}
	return false
}
