package spec_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/fuseone/agents/internal/spec"
)

const valid = `---
id: ticket-triage
name: Triagem de chamados
area: cx
provider: anthropic
model: claude-opus-5
effort: medium
tools: [crm.lookup, crm.note]
budget: {micros: 500000, steps: 60}
triggers:
  - { type: cron, schedule: "*/15 * * * *" }
---

Você faz a triagem dos chamados que chegam em suporte@.
`

func parse(t *testing.T, body string) spec.Spec {
	t.Helper()
	s, err := spec.Parse("test.agent.md", []byte(body))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return s
}

func TestParse_bodyBecomesTheInstructions(t *testing.T) {
	t.Parallel()

	s := parse(t, valid)

	// The text the author reviewed is the text the model receives — there is
	// no translation step in which intent can be lost.
	if !strings.HasPrefix(s.Instructions, "Você faz a triagem") {
		t.Errorf("Instructions = %q, want the file body", s.Instructions)
	}
	if strings.Contains(s.Instructions, "provider:") {
		t.Error("frontmatter leaked into the instructions")
	}
}

func TestParse_frontmatterFillsTheContract(t *testing.T) {
	t.Parallel()

	s := parse(t, valid)

	switch {
	case s.ID != "ticket-triage":
		t.Errorf("ID = %q", s.ID)
	case s.Area != "cx":
		t.Errorf("Area = %q — it is the unit of cost attribution", s.Area)
	case len(s.Tools) != 2:
		t.Errorf("Tools = %v, want the declared pack", s.Tools)
	case s.Budget.Micros != 500_000:
		t.Errorf("Budget.Micros = %d", s.Budget.Micros)
	case len(s.Triggers) != 1 || s.Triggers[0].Schedule == "":
		t.Errorf("Triggers = %+v, want the cron schedule", s.Triggers)
	}
}

func TestParse_versionIsTheContentDigest(t *testing.T) {
	t.Parallel()

	first := parse(t, valid)
	same := parse(t, valid)
	edited := parse(t, strings.Replace(valid, "effort: medium", "effort: high", 1))

	// Making the version *be* the content is what makes a published version
	// impossible to edit in place: different bytes are a different version,
	// so a run pinned to one is pinned to exact text.
	if first.Version != same.Version {
		t.Error("identical bytes produced different versions")
	}
	if first.Version == edited.Version {
		t.Error("an edited definition kept the same version")
	}
}

func TestParse_missingRequirements_areReportedTogether(t *testing.T) {
	t.Parallel()

	_, err := spec.Parse("bad.md", []byte("---\nid: x\n---\n\nfaça algo\n"))
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("Parse = %v, want %v", err, spec.ErrInvalid)
	}
	// One error per publish attempt wastes the author's time; list everything
	// that is wrong at once.
	for _, want := range []string{"area", "tools", "budget", "provider"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
}

func TestParse_agentWithNoBudget_doesNotPublish(t *testing.T) {
	t.Parallel()

	body := strings.Replace(valid, "budget: {micros: 500000, steps: 60}", "budget: {}", 1)

	// Without a ceiling a runaway agent bills until somebody notices. This is
	// the one default the platform must refuse to invent (PRD FO-02).
	if _, err := spec.Parse("x.md", []byte(body)); err == nil {
		t.Fatal("an agent with no budget was accepted")
	}
}

func TestParse_noFrontmatter_isRejected(t *testing.T) {
	t.Parallel()

	if _, err := spec.Parse("x.md", []byte("apenas texto")); !errors.Is(err, spec.ErrNoFrontmatter) {
		t.Errorf("Parse = %v, want %v", err, spec.ErrNoFrontmatter)
	}
}

func TestStore_publishingANewVersion_leavesTheOldOneResolvable(t *testing.T) {
	t.Parallel()

	store := spec.NewStore()
	v1 := parse(t, valid)
	v2 := parse(t, strings.Replace(valid, "effort: medium", "effort: high", 1))

	for _, s := range []spec.Spec{v1, v2} {
		if err := store.Publish(s); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// A run pinned to v1 keeps reading v1 for its whole life: publishing must
	// never change what an in-flight run is doing (PRD DE-09).
	got, err := store.Get("ticket-triage", v1.Version)
	if err != nil {
		t.Fatalf("Get(v1): %v", err)
	}
	if got.Effort != "medium" {
		t.Errorf("Effort = %q, want the pinned version's value", got.Effort)
	}

	current, err := store.Get("ticket-triage", "")
	if err != nil {
		t.Fatalf("Get(current): %v", err)
	}
	if current.Version != v2.Version {
		t.Error("a fresh trigger did not get the newest version")
	}
}

func TestStore_republishingIdenticalBytes_isANoOp(t *testing.T) {
	t.Parallel()

	store := spec.NewStore()
	s := parse(t, valid)
	for range 3 {
		if err := store.Publish(s); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// The version is the digest, so a redundant publish cannot produce a
	// different version and there is nothing to reconcile.
	if got := store.Versions("ticket-triage"); len(got) != 1 {
		t.Errorf("history = %v, want one entry", got)
	}
}

func TestStore_unknownVersion_isNotFound(t *testing.T) {
	t.Parallel()

	store := spec.NewStore()
	if err := store.Publish(parse(t, valid)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if _, err := store.Get("ticket-triage", "vdeadbeef"); !errors.Is(err, spec.ErrNotFound) {
		t.Errorf("Get = %v, want %v", err, spec.ErrNotFound)
	}
}

func TestLoadDir_oneBadDefinition_failsTheWholeLoad(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"agents/good.agent.md": {Data: []byte(valid)},
		"agents/bad.agent.md":  {Data: []byte("---\nid: broken\n---\n\ntexto\n")},
	}

	// Skipping the bad one would leave the author staring at a catalogue that
	// silently lacks their agent, with nothing to tell them why.
	if _, err := spec.NewStore().LoadDir(context.Background(), fsys, "agents"); err == nil {
		t.Fatal("LoadDir ignored a malformed definition")
	}
}

func TestLoadDir_publishesEveryDefinition(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"agents/a.agent.md": {Data: []byte(valid)},
		"agents/b.agent.md": {Data: []byte(strings.Replace(valid, "ticket-triage", "lead-qualifier", 1))},
		"agents/README.md2": {Data: []byte("not a definition")},
	}

	store := spec.NewStore()
	loaded, err := store.LoadDir(context.Background(), fsys, "agents")
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if loaded != 2 {
		t.Errorf("loaded = %d, want 2", loaded)
	}
	if got := store.Agents(); len(got) != 2 {
		t.Errorf("Agents = %v, want two", got)
	}
}
