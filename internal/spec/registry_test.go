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
	//
	// agent_state goes with it. Which version is current lives there, so a
	// test that chose one left the next test's fresh specs answering with a
	// version that no longer existed — the fixture outliving the rows it
	// pointed at.
	if _, err := pool.Exec(context.Background(),
		`truncate agent_specs; truncate agent_state`); err != nil {
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

func TestPublish_refusesADefinitionForAnotherCompany(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	published := published(t, strings.Replace(definition, "area: cx", "company: cora\narea: cx", 1))

	if err := r.Publish(ctx, published, "usr_ana", "acme"); err == nil {
		t.Fatal("Publish succeeded with a definition for another company")
	}
}

func TestPublish_emits_survivesTheRegistry(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	// The composition graph is derived from what the registry holds, so a
	// field the registry drops is an edge that silently disappears — and the
	// parse and render tests next door both pass while it does. Found by
	// declaring an emitter in the development stack and reading "nobody
	// publishes" on the screen.
	source := spec.Spec{
		ID: "triagem", Name: "Triagem", Area: "cx", Version: "v1",
		Provider: "openai", Model: "gpt-4o-mini",
		Tools:  []domain.ToolID{"crm.lookup"},
		Emits:  spec.Emits{{Event: "ticket.triado"}},
		Budget: domain.Budget{Micros: 100_000},
	}
	if err := registry.Publish(ctx, source, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	again, err := registry.Get(ctx, "triagem", "v1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(again.Emits) != 1 || again.Emits[0].Event != "ticket.triado" {
		t.Errorf("Emits = %v, want the declared event", again.Emits)
	}
}

func TestPublish_emitContext_survivesTheRegistry(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	source := spec.Spec{
		ID: "triagem", Name: "Triagem", Area: "cx", Version: "v1",
		Provider: "openai", Model: "gpt-4o-mini",
		Tools: []domain.ToolID{"crm.lookup"},
		Emits: spec.Emits{{
			Event: "incident.triaged", Context: "incident",
			Artifacts: []string{"triage_summary", "suspected_cause"},
		}},
		Budget: domain.Budget{Micros: 100_000},
	}
	if err := registry.Publish(ctx, source, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	again, err := registry.Get(ctx, "triagem", "v1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got := again.Emits[0]
	if got.Event != "incident.triaged" || got.Context != "incident" ||
		strings.Join(got.Artifacts, ",") != "triage_summary,suspected_cause" {
		t.Errorf("Emits[0] = %+v, want the context-carrying event", got)
	}
}

func TestList_currentVersion_isTheOneNamedRatherThanTheNewest(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	// The failure this prevents: an author edits a definition, the platform
	// publishes it, they revert the file. The withdrawn version stays newest
	// by timestamp for ever, every new run pins to a specification nobody
	// holds, and it parks with spec_unresolved seconds after opening.
	first := published(t, definition)
	if err := registry.Publish(ctx, first, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	second := first
	second.Version = "v-withdrawn"
	second.Instructions = first.Instructions + "\n\nUm parágrafo que alguém removeu depois."
	if err := registry.Publish(ctx, second, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// The worker holds the first one again, and says so.
	if err := registry.MakeCurrent(ctx, first.ID, first.Version); err != nil {
		t.Fatalf("MakeCurrent: %v", err)
	}

	listed, err := registry.List(ctx, domain.Scope{}, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List = %+v, want one agent", listed)
	}
	if listed[0].VersionID != first.Version {
		t.Errorf("VersionID = %q, want the version somebody chose (%q)",
			listed[0].VersionID, first.Version)
	}
}

func TestList_nobodyChose_fallsBackToTheNewest(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	// An installation upgrading into this has no choice recorded for any
	// agent. Answering with nothing would take every agent off every screen.
	first := published(t, definition)
	if err := registry.Publish(ctx, first, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	listed, err := registry.List(ctx, domain.Scope{}, false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].VersionID != first.Version {
		t.Errorf("List = %+v, want the newest by publication", listed)
	}
}

func TestVersions_currentFirst_soARunPinsToWhatIsHeld(t *testing.T) {
	registry := openRegistry(t)
	ctx := context.Background()

	// The opener reads the first row and pins the run to it. Ordering by
	// publication alone meant every new run pinned to a version somebody had
	// withdrawn, and parked with spec_unresolved seconds after opening.
	first := published(t, definition)
	if err := registry.Publish(ctx, first, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	withdrawn := first
	withdrawn.Version = "v-withdrawn"
	withdrawn.Instructions = first.Instructions + "\n\nRemovido depois."
	if err := registry.Publish(ctx, withdrawn, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := registry.MakeCurrent(ctx, first.ID, first.Version); err != nil {
		t.Fatalf("MakeCurrent: %v", err)
	}

	versions, err := registry.Versions(ctx, first.ID)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("Versions = %+v, want both", versions)
	}
	if versions[0].VersionID != first.Version || !versions[0].Latest {
		t.Errorf("first = %+v, want the current version marked latest", versions[0])
	}
}

/*
The stages survive publishing.

They were parsed, validated and rendered from the beginning and never stored:
the table had no column, so reading a version back gave a specification with
no steps — a different agent from the one somebody wrote. What makes it worth
a test rather than a migration alone is what `reaches` is for: the capability
pack is the ceiling and the step is the actual permission, so losing them
widens an agent quietly.
*/
func TestPublish_declaredSteps_areReadBack(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()

	withSteps := published(t, definition)
	withSteps.Steps = []spec.Step{
		{Name: "Encontrar o cliente", Reaches: []domain.ToolID{"crm.lookup"},
			StopsWhen: "não encontrar o cliente"},
		{Name: "Responder", Reaches: []domain.ToolID{"crm.reply"}, Model: "gpt-caro"},
	}

	if err := r.Publish(ctx, withSteps, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	got, err := r.Get(ctx, "triage", withSteps.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(got.Steps) != 2 {
		t.Fatalf("steps = %+v, want both", got.Steps)
	}
	if got.Steps[0].StopsWhen != "não encontrar o cliente" {
		t.Errorf("stops_when = %q, want the author's own words", got.Steps[0].StopsWhen)
	}
	// What the first step reaches is what the Gate narrows to while a run
	// sits in it, and it is the reason any of this is stored.
	if reach := got.Steps[0].Reaches; len(reach) != 1 || reach[0] != "crm.lookup" {
		t.Errorf("the first step reaches %v", reach)
	}
	if got.Steps[1].Model != "gpt-caro" {
		t.Errorf("model override = %q, want it kept", got.Steps[1].Model)
	}
}

// An agent that declares no steps has one envelope holding the whole pack,
// which is a different thing from a step that reaches nothing.
func TestPublish_noSteps_readsBackAsNoneRatherThanOne(t *testing.T) {
	r := openRegistry(t)
	ctx := context.Background()
	plain := published(t, definition)

	if err := r.Publish(ctx, plain, "usr_ana", "acme"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	got, err := r.Get(ctx, "triage", plain.Version)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Steps) != 0 {
		t.Errorf("steps = %+v, want none", got.Steps)
	}
	// With none declared the engine hands the Gate the whole pack, which is
	// how every agent behaved before steps existed.
	if len(got.Tools) == 0 {
		t.Error("the pack came back empty")
	}
}
