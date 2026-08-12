package spec_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/ledger"
	"github.com/fuseone/agents/internal/spec"
)

const definition = `---
id: triage
name: Ticket triage
area: cx
provider: openai
model: test-model
tools:
  - crm.lookup
budget:
  micros: 500000
  steps: 60
triggers:
  - { type: cron, schedule: "*/15 * * * *" }
---

Read the ticket and classify it.
`

func openRegistry(t *testing.T) *spec.Registry {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("REQUIRE_DATABASE") != "" {
			t.Fatal("REQUIRE_DATABASE is set but TEST_DATABASE_URL is empty")
		}
		t.Skip("TEST_DATABASE_URL is unset; skipping the registry suite")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := ledger.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// TRUNCATE, not DELETE: the table refuses row deletion, which is the
	// point — a version a run is pinned to must not be removable. Truncation
	// does not fire row triggers, and is how the ledger's own suite resets.
	if _, err := pool.Exec(context.Background(), `truncate agent_specs`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return spec.NewRegistry(pool)
}

func published(t *testing.T, source string) spec.Spec {
	t.Helper()
	s, err := spec.Parse("triage.agent.md", []byte(source))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

func TestPublish_readsBackTheVersionThatRan(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	published := published(t, definition)

	if err := r.Publish(ctx, published, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// A run is pinned to a version, and an auditor reading it later needs the
	// exact text it ran under — not whatever the file says today.
	got, err := r.Get(ctx, "triage", published.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Instructions != published.Instructions {
		t.Errorf("instructions = %q, want the published text back", got.Instructions)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "crm.lookup" {
		t.Errorf("tools = %v, want the capability pack as published", got.Tools)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].Schedule != "*/15 * * * *" {
		t.Errorf("triggers = %v, want the schedule as published", got.Triggers)
	}
}

func TestPublish_twice_keepsTheOriginalAuthorship(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	published := published(t, definition)

	if err := r.Publish(ctx, published, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// The version is the digest of the content, so publishing it again is
	// publishing the same text. It must not overwrite who published it first.
	if err := r.Publish(ctx, published, "usr_bruno", "acme"); err != nil {
		t.Fatalf("Publish again: %v", err)
	}

	agents, err := r.List(ctx, domain.Scope{}, true)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("List = %d versions, want 1", len(agents))
	}
	if agents[0].PublishedBy != "usr_ana" {
		t.Errorf("publishedBy = %q, want the first publisher", agents[0].PublishedBy)
	}
}

func TestList_showsTheNewestVersionOfEachAgent(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()

	first := published(t, definition)
	second := published(t, definition+"\nAlso note the account tier.\n")

	for _, s := range []spec.Spec{first, second} {
		if err := r.Publish(ctx, s, "usr_ana", "acme"); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	agents, err := r.List(ctx, domain.Scope{}, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("List = %d agents, want one row per agent", len(agents))
	}
	if agents[0].VersionID != second.Version || !agents[0].Latest {
		t.Errorf("version = %s latest = %v, want the newest marked latest", agents[0].VersionID, agents[0].Latest)
	}

	all, err := r.List(ctx, domain.Scope{}, true)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List all = %d, want the history", len(all))
	}
}

func TestList_scopeNarrowsToOneArea(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()

	if err := r.Publish(ctx, published(t, definition), "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	agents, err := r.List(ctx, domain.Scope{Company: "acme", Area: "marketing"}, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("List = %v, want nothing outside the agent's area", agents)
	}
}

func TestPublishedVersion_cannotBeChangedInPlace(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	published := published(t, definition)

	if err := r.Publish(ctx, published, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// The database enforces it, not the code that happens to be in front of
	// it: a run pinned to a version has to mean something years later.
	pool, _ := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	defer pool.Close()
	if _, err := pool.Exec(ctx,
		`update agent_specs set instructions = 'rewritten' where agent_id = 'triage'`); err == nil {
		t.Fatal("a published version was rewritten in place")
	}
}

// The history is what makes a run explainable years later: the version it was
// pinned to still reads exactly as it did when it ran.

func withBody(body string) string {
	return strings.Replace(definition, "Read the ticket and classify it.", body, 1)
}

func TestVersions_afterRepublishing_readsNewestFirstAndMarksOnlyOneLatest(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()

	for _, body := range []string{"Classifique o chamado.", "Classifique e responda o chamado."} {
		if err := r.Publish(ctx, published(t, withBody(body)), "usr_ana", "acme"); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	versions, err := r.Versions(ctx, "triage")
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want both", len(versions))
	}
	if !versions[0].Latest || versions[1].Latest {
		t.Error("exactly the newest version must be marked latest")
	}
}

func TestInstructions_readsTheTextThatVersionWasPublishedWith(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()

	first := published(t, withBody("Classifique o chamado."))
	for _, s := range []spec.Spec{first, published(t, withBody("Não faça nada."))} {
		if err := r.Publish(ctx, s, "usr_ana", "acme"); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// A run pinned to the first version is explained by the first version,
	// never by whatever was published over it since.
	text, _, err := r.Instructions(ctx, "triage", first.Version)
	if err != nil {
		t.Fatalf("Instructions: %v", err)
	}
	if text != "Classifique o chamado." {
		t.Errorf("instructions = %q, want the text that version carried", text)
	}
}

// The console does not publish a different kind of artefact from the one on
// disk. It renders the file, and the file is what gets parsed and published.

func TestRender_roundTripsThroughParse(t *testing.T) {
	original := published(t, definition)

	rendered, err := spec.Render(original)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	back, err := spec.Parse("console", rendered)
	if err != nil {
		t.Fatalf("Parse what Render wrote: %v\n%s", err, rendered)
	}

	if back.ID != original.ID || back.Name != original.Name || back.Model != original.Model {
		t.Errorf("identity changed: %+v", back)
	}
	if back.Instructions != original.Instructions {
		t.Errorf("instructions = %q, want %q", back.Instructions, original.Instructions)
	}
	if len(back.Tools) != len(original.Tools) || back.Tools[0] != original.Tools[0] {
		t.Errorf("tools = %v, want %v", back.Tools, original.Tools)
	}
	if len(back.Triggers) != 1 || back.Triggers[0].Schedule != "*/15 * * * *" {
		t.Errorf("triggers = %+v, want the schedule back", back.Triggers)
	}
	if back.Budget != original.Budget {
		t.Errorf("budget = %+v, want %+v", back.Budget, original.Budget)
	}
}

func TestRender_theSameDefinitionTwice_isTheSameVersion(t *testing.T) {
	// The property the registry rests on: publishing is a no-op when the
	// content is identical. A console that produced different bytes for the
	// same definition would create a second version of the same text on every
	// save, and every run pinned to either would be pinned to a coin toss.
	original := published(t, definition)

	first, err := spec.Render(original)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	second, _ := spec.Render(original)

	a, _ := spec.Parse("console", first)
	b, _ := spec.Parse("console", second)
	if a.Version != b.Version {
		t.Errorf("two renders gave %s and %s", a.Version, b.Version)
	}
}
