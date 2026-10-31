package admin_test

import (
	"context"
	"slices"
	"testing"

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

// A server whose toolsets were narrowed is rediscovered with fewer tools.
// The worker holding that discovery is the authority on what the server
// offers, so what it no longer lists stops being offered — otherwise the
// console keeps offering, and the publish check keeps accepting, a tool the
// runtime cannot call.
func TestPublish_aServerItCovers_dropsTheToolsItNoLongerOffers(t *testing.T) {
	curator := freshCurator(t)
	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{
		{ID: "github.issue_read", Server: "github"},
		{ID: "github.projects_write", Server: "github"},
	}})

	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{
		{ID: "github.issue_read", Server: "github"},
	}})

	if got := offered(t, curator, "github"); !slices.Equal(got, []domain.ToolID{"github.issue_read"}) {
		t.Fatalf("github offers %v, want only the tool its last discovery listed", got)
	}
}

// Narrowed to nothing is still a full answer about the server.
func TestPublish_aServerItCoversOfferingNothing_dropsEveryToolOfIt(t *testing.T) {
	curator := freshCurator(t)
	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{
		{ID: "github.issue_read", Server: "github"},
	}})

	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}})

	if got := offered(t, curator, "github"); len(got) != 0 {
		t.Fatalf("github offers %v, want nothing after a discovery that listed nothing", got)
	}
}

// A publication says nothing about a server it does not name: another worker
// may hold it, or its discovery failed on this one. Neither is evidence the
// server stopped offering anything.
func TestPublish_aServerItDoesNotCover_keepsItsTools(t *testing.T) {
	curator := freshCurator(t)
	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{
		{ID: "github.issue_read", Server: "github"},
	}})

	publish(t, curator, domain.ToolPublication{Servers: []string{"crm"}, Entries: []domain.ToolEntry{
		{ID: "crm.lookup", Server: "crm"},
	}})

	if got := offered(t, curator, "github"); !slices.Equal(got, []domain.ToolID{"github.issue_read"}) {
		t.Fatalf("github offers %v after a publication about crm alone, want its tool kept", got)
	}
}

// Two workers rediscovering one server at once each publish a whole answer.
// The catalogue ends with one of them, never a mix: a mix is a list no
// server ever offered.
func TestPublish_twoWholeAnswersAtOnce_oneOfThemHolds(t *testing.T) {
	curator := freshCurator(t)
	first := domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{
		{ID: "github.issue_read", Server: "github"},
	}}
	second := domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{
		{ID: "github.projects_list", Server: "github"},
	}}
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
	curator := freshCurator(t)
	write := domain.ToolEntry{ID: "github.projects_write", Server: "github", Digest: "sha-projects-write"}
	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{write}})
	if err := curator.Classify(context.Background(), domain.Scope{}, domain.ToolClassification{
		Tool: write.ID, Effect: domain.EffectWrite, By: "usr_ana", Digest: write.Digest,
	}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}})

	publish(t, curator, domain.ToolPublication{Servers: []string{"github"}, Entries: []domain.ToolEntry{write}})

	entries, err := curator.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if got := find(t, entries, write.ID); got.Effect != domain.EffectWrite || got.Stale {
		t.Fatalf("returning tool reads %+v, want the write ruling it had", got)
	}
}
