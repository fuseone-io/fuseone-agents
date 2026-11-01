package admin_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
)

// offered is what the console reads for one server: the IDs of its tools.
func offered(t *testing.T, curator *admin.Curator, server string) []domain.ToolID {
	t.Helper()
	entries, err := curator.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	var ids []domain.ToolID
	for _, e := range entries {
		if e.Server == server {
			ids = append(ids, e.ID)
		}
	}
	slices.Sort(ids)
	return ids
}

func publish(t *testing.T, curator *admin.Curator, pub domain.ToolPublication) {
	t.Helper()
	if err := curator.Publish(context.Background(), pub); err != nil {
		t.Fatalf("Publish: %v", err)
	}
}

// standalone publishes fixtures as servers the configuration does not hold,
// each its own authority — the shape a command-line server publishes in.
func standalone(entries []domain.ToolEntry) domain.ToolPublication {
	var servers []domain.PublishedServer
	for _, e := range entries {
		if !slices.ContainsFunc(servers, func(s domain.PublishedServer) bool { return s.Name == e.Server }) {
			servers = append(servers, domain.PublishedServer{Name: e.Server, Unmanaged: true})
		}
	}
	return domain.ToolPublication{Servers: servers, Entries: entries, Holder: "worker-a"}
}

// configure saves a server row as the console would, and returns the
// generation a worker connecting now would read.
func configure(t *testing.T, pool *pgxpool.Pool, name string, enabled bool) time.Time {
	t.Helper()
	var generation time.Time
	if err := pool.QueryRow(context.Background(), `
		insert into settings (scope_kind, company_id, area_id, kind, name, value, enabled, updated_by, updated_at)
		values ('installation', '', '', 'mcp_server', $1, '{}', $2, 'usr_ana', clock_timestamp())
		on conflict (scope_kind, company_id, area_id, kind, name) do update set
			enabled = excluded.enabled, updated_at = clock_timestamp()
		returning updated_at`, name, enabled).Scan(&generation); err != nil {
		t.Fatalf("configure %s: %v", name, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`delete from settings where kind = 'mcp_server' and name = $1`, name)
	})
	return generation
}

func unconfigure(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`delete from settings where kind = 'mcp_server' and name = $1`, name); err != nil {
		t.Fatalf("unconfigure %s: %v", name, err)
	}
}

func catalogueOf(t *testing.T) (*admin.Curator, *pgxpool.Pool) {
	t.Helper()
	curator := freshCurator(t)
	pool := openPool(t)
	if _, err := pool.Exec(context.Background(), `delete from settings where kind = 'mcp_server'`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return curator, pool
}

func discovered(name string, generation time.Time, ids ...domain.ToolID) domain.ToolPublication {
	pub := domain.ToolPublication{Servers: []domain.PublishedServer{{Name: name, Generation: generation}}}
	for _, id := range ids {
		pub.Entries = append(pub.Entries, domain.ToolEntry{ID: id, Server: name})
	}
	return pub
}

// A server whose toolsets were narrowed is rediscovered with fewer tools.
// The worker holding that discovery is the authority on what the server
// offers, so what it no longer lists stops being offered.
func TestPublish_aServerItCovers_dropsTheToolsItNoLongerOffers(t *testing.T) {
	curator, pool := catalogueOf(t)
	g := configure(t, pool, "github", true)
	publish(t, curator, discovered("github", g, "github.issue_read", "github.projects_write"))

	publish(t, curator, discovered("github", g, "github.issue_read"))

	if got := offered(t, curator, "github"); !slices.Equal(got, []domain.ToolID{"github.issue_read"}) {
		t.Fatalf("github offers %v, want only the tool its last discovery listed", got)
	}
}

// Narrowed to nothing is still a full answer about the server.
func TestPublish_aServerItCoversOfferingNothing_dropsEveryToolOfIt(t *testing.T) {
	curator, pool := catalogueOf(t)
	g := configure(t, pool, "github", true)
	publish(t, curator, discovered("github", g, "github.issue_read"))

	publish(t, curator, discovered("github", g))

	if got := offered(t, curator, "github"); len(got) != 0 {
		t.Fatalf("github offers %v, want nothing after a discovery that listed nothing", got)
	}
}

// A publication says nothing about a server it does not name: another worker
// may hold it, or its discovery failed on this one.
func TestPublish_aServerItDoesNotCover_keepsItsTools(t *testing.T) {
	curator, pool := catalogueOf(t)
	github := configure(t, pool, "github", true)
	crm := configure(t, pool, "crm", true)
	publish(t, curator, discovered("github", github, "github.issue_read"))

	publish(t, curator, discovered("crm", crm, "crm.lookup"))

	if got := offered(t, curator, "github"); !slices.Equal(got, []domain.ToolID{"github.issue_read"}) {
		t.Fatalf("github offers %v after a publication about crm alone, want its tool kept", got)
	}
}

/*
A discovery made under a configuration that has since changed is not about
the server as it is now. A worker that connected before the toolsets were
narrowed and publishes after the narrowed discovery was recorded would put
the dropped tools back; its word about the server is ignored instead, and
what the current discovery recorded stays.
*/
func TestPublish_aDiscoveryUnderAnOlderConfiguration_isIgnored(t *testing.T) {
	curator, pool := catalogueOf(t)
	broad := configure(t, pool, "github", true)
	narrow := configure(t, pool, "github", true)
	publish(t, curator, discovered("github", narrow, "github.issue_read"))

	publish(t, curator, discovered("github", broad, "github.issue_read", "github.projects_write"))

	if got := offered(t, curator, "github"); !slices.Equal(got, []domain.ToolID{"github.issue_read"}) {
		t.Fatalf("github offers %v, want the current configuration's discovery to stand", got)
	}
}

// A late publication about a server that has been deleted records nothing,
// and its tools go.
func TestPublish_aDiscoveryOfADeletedServer_recordsNothing(t *testing.T) {
	curator, pool := catalogueOf(t)
	g := configure(t, pool, "github", true)
	publish(t, curator, discovered("github", g, "github.issue_read"))
	unconfigure(t, pool, "github")

	publish(t, curator, discovered("github", g, "github.issue_read"))

	if got := offered(t, curator, "github"); len(got) != 0 {
		t.Fatalf("a deleted server offers %v after a late publication about it", got)
	}
}

// Every publication, about any server, clears the tools of servers the
// configuration no longer wants — deleted or switched off — so a withdrawal
// that never arrived costs nothing past the next publish.
func TestPublish_anyPublication_clearsTheToolsOfServersNoLongerWanted(t *testing.T) {
	curator, pool := catalogueOf(t)
	github := configure(t, pool, "github", true)
	jira := configure(t, pool, "jira", true)
	crm := configure(t, pool, "crm", true)
	publish(t, curator, discovered("github", github, "github.issue_read"))
	publish(t, curator, discovered("jira", jira, "jira.search"))
	unconfigure(t, pool, "github")
	configure(t, pool, "jira", false)

	publish(t, curator, discovered("crm", crm, "crm.lookup"))

	for _, gone := range []string{"github", "jira"} {
		if got := offered(t, curator, gone); len(got) != 0 {
			t.Errorf("%s offers %v, want its tools cleared by a publication about crm", gone, got)
		}
	}
}

// A server started from the command line is not in the configuration, and
// another worker's publication must not sweep it away.
func TestPublish_anUnmanagedServer_isLeftToItsOwnWorker(t *testing.T) {
	curator, pool := catalogueOf(t)
	crm := configure(t, pool, "crm", true)
	publish(t, curator, standalone([]domain.ToolEntry{{ID: "local.run", Server: "local"}}))

	publish(t, curator, discovered("crm", crm, "crm.lookup"))

	if got := offered(t, curator, "local"); !slices.Equal(got, []domain.ToolID{"local.run"}) {
		t.Fatalf("local offers %v after another worker published, want its tool kept", got)
	}
}

// A configured server now carries the name a command-line server once had:
// the configuration decides, and the unmanaged publication says nothing.
func TestPublish_anUnmanagedPublicationForAConfiguredName_isIgnored(t *testing.T) {
	curator, pool := catalogueOf(t)
	g := configure(t, pool, "local", true)
	publish(t, curator, discovered("local", g, "local.lookup"))

	publish(t, curator, standalone([]domain.ToolEntry{{ID: "local.run", Server: "local"}}))

	if got := offered(t, curator, "local"); !slices.Equal(got, []domain.ToolID{"local.lookup"}) {
		t.Fatalf("local offers %v, want the configured server's discovery to stand", got)
	}
}

// Every tool is written in one statement, and one statement cannot touch a
// row twice: a tool listed twice is recorded once, as last listed, rather
// than failing the whole publication.
func TestPublish_aToolListedTwice_isRecordedOnceAsLastListed(t *testing.T) {
	curator, pool := catalogueOf(t)
	g := configure(t, pool, "github", true)
	pub := discovered("github", g)
	pub.Entries = []domain.ToolEntry{
		{ID: "github.issue_read", Server: "github", Description: "first"},
		{ID: "github.issue_read", Server: "github", Description: "second"},
	}

	publish(t, curator, pub)

	entries, err := curator.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if got := find(t, entries, "github.issue_read"); got.Description != "second" {
		t.Fatalf("issue_read reads %q, want the last listing", got.Description)
	}
}

// Two workers rediscovering one server at once each publish a whole answer.
// The catalogue ends with one of them, never a mix: a mix is a list no
// server ever offered.
func TestPublish_twoWholeAnswersAtOnce_oneOfThemHolds(t *testing.T) {
	curator, pool := catalogueOf(t)
	g := configure(t, pool, "github", true)
	first := discovered("github", g, "github.issue_read")
	second := discovered("github", g, "github.projects_list")
	for round := range 30 {
		done := make(chan error, 2)
		for _, pub := range []domain.ToolPublication{first, second} {
			go func() { done <- curator.Publish(context.Background(), pub) }()
		}
		for range 2 {
			if err := <-done; err != nil {
				t.Fatalf("round %d: Publish: %v", round, err)
			}
		}
		if got := offered(t, curator, "github"); len(got) != 1 {
			t.Fatalf("round %d: github offers %v, want exactly one worker's answer", round, got)
		}
	}
}

// Dropping a tool from the catalogue is not forgetting the ruling on it. A
// tool that comes back with the same definition is the tool somebody ruled
// on, and making the Curator rule again would be asking them to repeat a
// decision the trail already holds.
func TestPublish_aToolThatComesBack_getsItsRulingBack(t *testing.T) {
	curator, pool := catalogueOf(t)
	g := configure(t, pool, "github", true)
	write := domain.ToolEntry{ID: "github.projects_write", Server: "github", Digest: "sha-projects-write"}
	with := discovered("github", g)
	with.Entries = []domain.ToolEntry{write}
	publish(t, curator, with)
	if err := curator.Classify(context.Background(), domain.Scope{}, domain.ToolClassification{
		Tool: write.ID, Effect: domain.EffectWrite, By: "usr_ana", Digest: write.Digest,
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	publish(t, curator, discovered("github", g))

	publish(t, curator, with)

	entries, err := curator.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if got := find(t, entries, write.ID); got.Effect != domain.EffectWrite || got.Stale {
		t.Fatalf("returning tool reads %+v, want the write ruling it had", got)
	}
}
